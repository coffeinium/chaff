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

// PolicyGroup — именованная группа машин со своими правилами (ip/cidr/домен/url),
// которые применяются только к участникам, когда группа включена. Глобальные
// правила (ручные и фиды) всегда приоритетнее групповых.
type PolicyGroup struct {
	ID        int64          `json:"id"`
	Name      string         `json:"name"`
	Enabled   bool           `json:"enabled"`
	Note      string         `json:"note,omitempty"`
	CreatedAt int64          `json:"created_at"`
	Members   []PolicyMember `json:"members"`
	Rules     []GroupRule    `json:"rules"`
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

// GroupRule — правило группы, тот же вид, что глобальные индикаторы,
// но действует только на участников группы.
type GroupRule struct {
	ID        int64        `json:"id"`
	Value     string       `json:"value"`
	Kind      model.Kind   `json:"kind"`
	Action    model.Action `json:"action"`
	Note      string       `json:"note,omitempty"`
	CreatedAt int64        `json:"created_at"`
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

// MemberMachineIndex: MAC -> имя группы, по всем группам (для пометки кандидатов).
func (s *Store) MemberMachineIndex() (map[string]string, error) {
	hostMACs, err := s.hostToMACs()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
		SELECT g.name, m.kind, m.value
		FROM policy_members m JOIN policy_groups g ON g.id = m.group_id`)
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

func (s *Store) GetGroup(ref string) (PolicyGroup, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return PolicyGroup{}, errors.New("нужно имя или id группы")
	}
	q := `SELECT id, name, enabled, note, created_at FROM policy_groups WHERE name = ?`
	arg := any(ref)
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		q = `SELECT id, name, enabled, note, created_at FROM policy_groups WHERE id = ?`
		arg = id
	}
	var g PolicyGroup
	var enabled int
	err := s.db.QueryRow(q, arg).Scan(&g.ID, &g.Name, &enabled, &g.Note, &g.CreatedAt)
	if err == sql.ErrNoRows {
		return PolicyGroup{}, fmt.Errorf("группа %q не найдена", ref)
	}
	if err != nil {
		return PolicyGroup{}, err
	}
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

func (s *Store) rulesOf(groupID int64) ([]GroupRule, error) {
	rows, err := s.db.Query(`
		SELECT id, value, kind, action, note, created_at
		FROM policy_rules WHERE group_id = ? ORDER BY kind, value`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GroupRule
	for rows.Next() {
		var r GroupRule
		var kind, action string
		if err := rows.Scan(&r.ID, &r.Value, &kind, &action, &r.Note, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Kind = model.Kind(kind)
		r.Action = model.Action(action)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListGroups() ([]PolicyGroup, error) {
	rows, err := s.db.Query(
		`SELECT id, name, enabled, note, created_at FROM policy_groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	var groups []PolicyGroup
	for rows.Next() {
		var g PolicyGroup
		var enabled int
		if err := rows.Scan(&g.ID, &g.Name, &enabled, &g.Note, &g.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
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
		rules, err := s.rulesOf(groups[i].ID)
		if err != nil {
			return nil, err
		}
		groups[i].Rules = rules
	}
	return groups, nil
}

func (s *Store) CreateGroup(name, note string) (PolicyGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return PolicyGroup{}, errors.New("нужно имя группы")
	}
	res, err := s.db.Exec(
		`INSERT INTO policy_groups (name, enabled, note, created_at) VALUES (?, 0, ?, ?)`,
		name, strings.TrimSpace(note), time.Now().Unix())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return PolicyGroup{}, fmt.Errorf("группа %q уже существует", name)
		}
		return PolicyGroup{}, err
	}
	id, _ := res.LastInsertId()
	return PolicyGroup{ID: id, Name: name, Note: strings.TrimSpace(note)}, nil
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

func normRuleAction(action string) (model.Action, error) {
	switch model.Action(strings.ToLower(strings.TrimSpace(action))) {
	case model.ActionBlock, "":
		return model.ActionBlock, nil
	case model.ActionAllow:
		return model.ActionAllow, nil
	}
	return "", fmt.Errorf("действие должно быть block или allow, а не %q", action)
}

func (s *Store) AddGroupRule(ref, value, action, note string) (PolicyGroup, string, error) {
	g, err := s.GetGroup(ref)
	if err != nil {
		return PolicyGroup{}, "", err
	}
	value = strings.TrimSpace(value)
	kind := model.Classify(value)
	switch kind {
	case model.KindUnknown:
		return PolicyGroup{}, "", errors.New("не распознан вид значения (ip/cidr/домен/url)")
	case model.KindMAC:
		return PolicyGroup{}, "", errors.New("MAC блокируется целиком глобальным правилом (chaff block add MAC), в группе поддерживаются ip/cidr/домен/url")
	}
	act, err := normRuleAction(action)
	if err != nil {
		return PolicyGroup{}, "", err
	}
	now := time.Now().Unix()
	if _, err := s.db.Exec(`
		INSERT INTO policy_rules (group_id, value, kind, action, scope, note, created_at)
		VALUES (?, ?, ?, ?, 'domain', ?, ?)
		ON CONFLICT(group_id, value, kind) DO UPDATE SET
			action = excluded.action,
			note   = CASE WHEN excluded.note <> '' THEN excluded.note ELSE note END`,
		g.ID, value, string(kind), string(act), strings.TrimSpace(note), now); err != nil {
		return PolicyGroup{}, "", err
	}
	return g, fmt.Sprintf("в группу %q добавлено правило %s %q", g.Name, act, value), nil
}

func (s *Store) RemoveGroupRule(ref, value string) (PolicyGroup, error) {
	g, err := s.GetGroup(ref)
	if err != nil {
		return PolicyGroup{}, err
	}
	res, err := s.db.Exec(
		`DELETE FROM policy_rules WHERE group_id = ? AND value = ?`, g.ID, strings.TrimSpace(value))
	if err != nil {
		return PolicyGroup{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return PolicyGroup{}, fmt.Errorf("правило %q не найдено в группе %q", value, g.Name)
	}
	return g, nil
}

// GroupPolicies собирает включённые группы с правилами для Ruleset.
// Пусто, если модуль grouppolicy выключен — тогда поведение как без групп.
func (s *Store) GroupPolicies() ([]model.GroupPolicy, error) {
	if !s.IsModuleEnabled("grouppolicy") {
		return nil, nil
	}
	hostMACs, err := s.hostToMACs()
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`SELECT id, name FROM policy_groups WHERE enabled = 1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	type grp struct {
		id   int64
		name string
	}
	var enabled []grp
	for rows.Next() {
		var g grp
		if err := rows.Scan(&g.id, &g.name); err != nil {
			rows.Close()
			return nil, err
		}
		enabled = append(enabled, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []model.GroupPolicy
	for _, g := range enabled {
		gp := model.GroupPolicy{ID: g.id, Name: g.name, Allow: model.AllowSet{Domains: map[string]bool{}}}

		mrows, err := s.db.Query(`SELECT kind, value FROM policy_members WHERE group_id = ?`, g.id)
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for mrows.Next() {
			var kind, value string
			if err := mrows.Scan(&kind, &value); err != nil {
				mrows.Close()
				return nil, err
			}
			for _, mc := range resolveMemberMACs(kind, value, hostMACs) {
				if !seen[mc] {
					seen[mc] = true
					gp.MACs = append(gp.MACs, mc)
				}
			}
		}
		mrows.Close()
		if err := mrows.Err(); err != nil {
			return nil, err
		}

		rules, err := s.rulesOf(g.id)
		if err != nil {
			return nil, err
		}
		for _, r := range rules {
			switch r.Kind {
			case model.KindIP, model.KindCIDR:
				p, ok := parsePrefix(r.Value)
				if !ok {
					continue
				}
				if r.Action == model.ActionAllow {
					gp.Allow.IPs = append(gp.Allow.IPs, p)
				} else if p.Addr().Is4() {
					gp.IPv4 = append(gp.IPv4, p)
				}
			case model.KindDomain:
				if r.Action == model.ActionAllow {
					gp.Allow.Domains[r.Value] = true
					continue
				}
				gp.Domains = append(gp.Domains, model.DomainRule{
					Domain: r.Value, Scope: model.ScopeDomain, Action: r.Action,
				})
			case model.KindURL:
				if r.Action == model.ActionAllow {
					continue
				}
				gp.URLs = append(gp.URLs, model.URLRule{URL: r.Value, Action: r.Action})
			}
		}

		if len(gp.MACs) == 0 {
			continue // участники ещё не выучены — применять не к кому
		}
		out = append(out, gp)
	}
	return out, nil
}
