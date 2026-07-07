package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/coffeinium/chaff/internal/model"
)

// PolicyGroup — именованная группа машин с общей политикой (block/allow),
// применяемой к её участникам, когда группа включена.
type PolicyGroup struct {
	ID        int64          `json:"id"`
	Name      string         `json:"name"`
	Action    model.Action   `json:"action"`
	Enabled   bool           `json:"enabled"`
	Note      string         `json:"note,omitempty"`
	CreatedAt int64          `json:"created_at"`
	Members   []PolicyMember `json:"members"`
}

// PolicyMember — участник группы: MAC-адрес или имя хоста (kind: mac|host).
// Для host в MACs подставляются выученные MAC-адреса (может быть пусто — тогда
// участник «ждёт», пока сеть не выучит его MAC).
type PolicyMember struct {
	Kind     string   `json:"kind"`
	Value    string   `json:"value"`
	MACs     []string `json:"macs,omitempty"`
	Hostname string   `json:"hostname,omitempty"`
	Resolved bool     `json:"resolved"`
}

func normMemberKind(value string) (kind, norm string) {
	value = strings.TrimSpace(value)
	if model.Classify(value) == model.KindMAC {
		return "mac", model.NormalizeMAC(value)
	}
	return "host", strings.ToLower(value)
}

// hostToMACs строит обратный индекс: имя хоста -> выученные MAC-адреса.
func (s *Store) hostToMACs() (map[string][]string, error) {
	rows, err := s.db.Query(`SELECT key, hostname FROM hostnames WHERE kind = 'mac' AND hostname <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var key, hostname string
		if err := rows.Scan(&key, &hostname); err != nil {
			return nil, err
		}
		h := strings.ToLower(hostname)
		out[h] = append(out[h], model.NormalizeMAC(key))
	}
	return out, rows.Err()
}

func resolveMemberMACs(kind, value string, hostMACs map[string][]string) []string {
	if kind == "mac" {
		return []string{model.NormalizeMAC(value)}
	}
	return hostMACs[strings.ToLower(value)]
}

// manualMACRules возвращает ручные (source_id=0) MAC-правила — «фундаментальные»
// правила, с которыми групповым запрещено конфликтовать.
func (s *Store) manualMACRules() (map[string]model.Action, error) {
	rows, err := s.db.Query(
		`SELECT value, action FROM indicators WHERE source_id = ? AND kind = 'mac' AND enabled = 1`,
		ManualSourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]model.Action{}
	for rows.Next() {
		var value, action string
		if err := rows.Scan(&value, &action); err != nil {
			return nil, err
		}
		out[model.NormalizeMAC(value)] = model.Action(action)
	}
	return out, rows.Err()
}

func (s *Store) checkFundamentalConflict(macs []string, action model.Action) error {
	if len(macs) == 0 {
		return nil
	}
	rules, err := s.manualMACRules()
	if err != nil {
		return err
	}
	for _, mc := range macs {
		a, ok := rules[mc]
		if !ok {
			continue
		}
		if (action == model.ActionBlock && a == model.ActionAllow) ||
			(action == model.ActionAllow && a == model.ActionBlock) {
			return fmt.Errorf(
				"конфликт с фундаментальным правилом: MAC %s вручную помечен как %s, групповое правило %s запрещено",
				mc, a, action)
		}
	}
	return nil
}

// macsClaimedByOthers: MAC -> имя чужой группы (кроме exceptID), с учётом
// разрешения host-участников в MAC-адреса.
func (s *Store) macsClaimedByOthers(exceptID int64) (map[string]string, error) {
	hostMACs, err := s.hostToMACs()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
		SELECT g.name, m.kind, m.value
		FROM policy_members m JOIN policy_groups g ON g.id = m.group_id
		WHERE m.group_id <> ?`, exceptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, kind, value string
		if err := rows.Scan(&name, &kind, &value); err != nil {
			return nil, err
		}
		for _, mc := range resolveMemberMACs(kind, value, hostMACs) {
			out[mc] = name
		}
	}
	return out, rows.Err()
}

// MemberMachineIndex: MAC -> имя группы, по всем группам (для пометки кандидатов).
func (s *Store) MemberMachineIndex() (map[string]string, error) {
	return s.macsClaimedByOthers(0)
}

func (s *Store) GetGroup(ref string) (PolicyGroup, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return PolicyGroup{}, errors.New("нужно имя или id группы")
	}
	q := `SELECT id, name, action, enabled, note, created_at FROM policy_groups WHERE name = ?`
	arg := any(ref)
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		q = `SELECT id, name, action, enabled, note, created_at FROM policy_groups WHERE id = ?`
		arg = id
	}
	var g PolicyGroup
	var action string
	var enabled int
	err := s.db.QueryRow(q, arg).Scan(&g.ID, &g.Name, &action, &enabled, &g.Note, &g.CreatedAt)
	if err == sql.ErrNoRows {
		return PolicyGroup{}, fmt.Errorf("группа %q не найдена", ref)
	}
	if err != nil {
		return PolicyGroup{}, err
	}
	g.Action = model.Action(action)
	g.Enabled = enabled != 0
	return g, nil
}

func (s *Store) membersOf(groupID int64, hostMACs map[string][]string, byMAC map[string]string) ([]PolicyMember, error) {
	rows, err := s.db.Query(`SELECT kind, value FROM policy_members WHERE group_id = ? ORDER BY kind, value`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PolicyMember
	for rows.Next() {
		var m PolicyMember
		if err := rows.Scan(&m.Kind, &m.Value); err != nil {
			return nil, err
		}
		if m.Kind == "mac" {
			m.MACs = []string{m.Value}
			m.Hostname = byMAC[m.Value]
			m.Resolved = true
		} else {
			m.MACs = hostMACs[m.Value]
			m.Resolved = len(m.MACs) > 0
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) ListGroups() ([]PolicyGroup, error) {
	rows, err := s.db.Query(
		`SELECT id, name, action, enabled, note, created_at FROM policy_groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	var groups []PolicyGroup
	for rows.Next() {
		var g PolicyGroup
		var action string
		var enabled int
		if err := rows.Scan(&g.ID, &g.Name, &action, &enabled, &g.Note, &g.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		g.Action = model.Action(action)
		g.Enabled = enabled != 0
		groups = append(groups, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	hostMACs, err := s.hostToMACs()
	if err != nil {
		return nil, err
	}
	byMAC, _, err := s.Hostnames()
	if err != nil {
		return nil, err
	}
	for i := range groups {
		mem, err := s.membersOf(groups[i].ID, hostMACs, byMAC)
		if err != nil {
			return nil, err
		}
		groups[i].Members = mem
	}
	return groups, nil
}

func (s *Store) CreateGroup(name, action, note string) (PolicyGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return PolicyGroup{}, errors.New("нужно имя группы")
	}
	act, err := normAction(action)
	if err != nil {
		return PolicyGroup{}, err
	}
	res, err := s.db.Exec(
		`INSERT INTO policy_groups (name, action, enabled, note, created_at) VALUES (?, ?, 0, ?, ?)`,
		name, string(act), strings.TrimSpace(note), time.Now().Unix())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return PolicyGroup{}, fmt.Errorf("группа %q уже существует", name)
		}
		return PolicyGroup{}, err
	}
	id, _ := res.LastInsertId()
	return PolicyGroup{ID: id, Name: name, Action: act, Note: strings.TrimSpace(note)}, nil
}

func normAction(action string) (model.Action, error) {
	switch model.Action(strings.ToLower(strings.TrimSpace(action))) {
	case model.ActionBlock, "":
		return model.ActionBlock, nil
	case model.ActionAllow:
		return model.ActionAllow, nil
	}
	return "", fmt.Errorf("действие должно быть block или allow, а не %q", action)
}

func (s *Store) DeleteGroup(ref string) (string, error) {
	g, err := s.GetGroup(ref)
	if err != nil {
		return "", err
	}
	if _, err := s.db.Exec(`DELETE FROM policy_groups WHERE id = ?`, g.ID); err != nil {
		return "", err
	}
	return g.Name, nil
}

func (s *Store) SetGroupEnabled(ref string, enabled bool) (PolicyGroup, error) {
	g, err := s.GetGroup(ref)
	if err != nil {
		return PolicyGroup{}, err
	}
	v := 0
	if enabled {
		v = 1
	}
	if _, err := s.db.Exec(`UPDATE policy_groups SET enabled = ? WHERE id = ?`, v, g.ID); err != nil {
		return PolicyGroup{}, err
	}
	g.Enabled = enabled
	return g, nil
}

func (s *Store) SetGroupNote(ref, note string) (PolicyGroup, error) {
	g, err := s.GetGroup(ref)
	if err != nil {
		return PolicyGroup{}, err
	}
	if _, err := s.db.Exec(`UPDATE policy_groups SET note = ? WHERE id = ?`, strings.TrimSpace(note), g.ID); err != nil {
		return PolicyGroup{}, err
	}
	return g, nil
}

func (s *Store) SetGroupAction(ref, action string) (PolicyGroup, error) {
	g, err := s.GetGroup(ref)
	if err != nil {
		return PolicyGroup{}, err
	}
	act, err := normAction(action)
	if err != nil {
		return PolicyGroup{}, err
	}
	// Смена действия не должна порождать конфликт с фундаментальными правилами.
	hostMACs, err := s.hostToMACs()
	if err != nil {
		return PolicyGroup{}, err
	}
	rows, err := s.db.Query(`SELECT kind, value FROM policy_members WHERE group_id = ?`, g.ID)
	if err != nil {
		return PolicyGroup{}, err
	}
	var macs []string
	for rows.Next() {
		var kind, value string
		if err := rows.Scan(&kind, &value); err != nil {
			rows.Close()
			return PolicyGroup{}, err
		}
		macs = append(macs, resolveMemberMACs(kind, value, hostMACs)...)
	}
	rows.Close()
	if err := s.checkFundamentalConflict(macs, act); err != nil {
		return PolicyGroup{}, err
	}
	if _, err := s.db.Exec(`UPDATE policy_groups SET action = ? WHERE id = ?`, string(act), g.ID); err != nil {
		return PolicyGroup{}, err
	}
	g.Action = act
	return g, nil
}

func (s *Store) AddGroupMember(ref, value string) (PolicyGroup, string, error) {
	g, err := s.GetGroup(ref)
	if err != nil {
		return PolicyGroup{}, "", err
	}
	kind, norm := normMemberKind(value)
	if norm == "" {
		return PolicyGroup{}, "", errors.New("нужно значение: MAC-адрес или имя хоста")
	}

	var owner string
	err = s.db.QueryRow(
		`SELECT g.name FROM policy_members m JOIN policy_groups g ON g.id = m.group_id WHERE m.kind = ? AND m.value = ?`,
		kind, norm).Scan(&owner)
	if err == nil {
		return PolicyGroup{}, "", fmt.Errorf("%q уже состоит в группе %q", norm, owner)
	}
	if err != sql.ErrNoRows {
		return PolicyGroup{}, "", err
	}

	hostMACs, err := s.hostToMACs()
	if err != nil {
		return PolicyGroup{}, "", err
	}
	macs := resolveMemberMACs(kind, norm, hostMACs)

	if err := s.checkFundamentalConflict(macs, g.Action); err != nil {
		return PolicyGroup{}, "", err
	}
	claimed, err := s.macsClaimedByOthers(g.ID)
	if err != nil {
		return PolicyGroup{}, "", err
	}
	for _, mc := range macs {
		if other, ok := claimed[mc]; ok {
			return PolicyGroup{}, "", fmt.Errorf("машина %s уже состоит в группе %q", mc, other)
		}
	}

	if _, err := s.db.Exec(
		`INSERT INTO policy_members (group_id, kind, value) VALUES (?, ?, ?)`, g.ID, kind, norm); err != nil {
		return PolicyGroup{}, "", err
	}
	return g, fmt.Sprintf("в группу %q добавлен %s %s", g.Name, kind, norm), nil
}

func (s *Store) RemoveGroupMember(ref, value string) (PolicyGroup, error) {
	g, err := s.GetGroup(ref)
	if err != nil {
		return PolicyGroup{}, err
	}
	kind, norm := normMemberKind(value)
	res, err := s.db.Exec(
		`DELETE FROM policy_members WHERE group_id = ? AND kind = ? AND value = ?`, g.ID, kind, norm)
	if err != nil {
		return PolicyGroup{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return PolicyGroup{}, fmt.Errorf("%q не найден в группе %q", norm, g.Name)
	}
	return g, nil
}

// GroupMACDecisions возвращает MAC-решения от включённых групп: множества block и
// allow. active=false, если модуль выключен или включённых групповых правил нет —
// в этом случае BuildRuleset сохраняет прежнее поведение.
func (s *Store) GroupMACDecisions() (block, allow map[string]bool, active bool, err error) {
	if !s.IsModuleEnabled("grouppolicy") {
		return nil, nil, false, nil
	}
	hostMACs, err := s.hostToMACs()
	if err != nil {
		return nil, nil, false, err
	}
	rows, err := s.db.Query(`
		SELECT g.action, m.kind, m.value
		FROM policy_members m JOIN policy_groups g ON g.id = m.group_id
		WHERE g.enabled = 1`)
	if err != nil {
		return nil, nil, false, err
	}
	defer rows.Close()
	block, allow = map[string]bool{}, map[string]bool{}
	has := false
	for rows.Next() {
		var action, kind, value string
		if err := rows.Scan(&action, &kind, &value); err != nil {
			return nil, nil, false, err
		}
		dst := block
		if model.Action(action) == model.ActionAllow {
			dst = allow
		}
		for _, mc := range resolveMemberMACs(kind, value, hostMACs) {
			dst[mc] = true
			has = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, false, err
	}
	return block, allow, has, nil
}
