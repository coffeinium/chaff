package store

import (
	"net/netip"
	"time"

	"github.com/coffeinium/chaff/internal/model"
)

func (s *Store) ReplaceIndicators(sourceID int64, inds []model.Indicator) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	stmt, err := tx.Prepare(`
		INSERT INTO indicators
			(value, kind, action, scope, note, source_id, first_seen, last_seen, expires_at, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT(value, kind, source_id) DO UPDATE SET
			action     = excluded.action,
			scope      = excluded.scope,
			note       = excluded.note,
			last_seen  = excluded.last_seen,
			expires_at = excluded.expires_at,
			enabled    = 1
	`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	keep := make(map[[2]string]bool, len(inds))
	n := 0
	for _, in := range inds {
		if in.Kind == model.KindUnknown || in.Value == "" {
			continue
		}
		if in.Action == "" {
			in.Action = model.ActionBlock
		}
		if in.Scope == "" {
			in.Scope = model.ScopeDomain
		}
		if in.Kind == model.KindMAC {
			in.Value = model.NormalizeMAC(in.Value)
		}
		if _, err := stmt.Exec(
			in.Value, string(in.Kind), string(in.Action), string(in.Scope),
			in.Note, sourceID, now, now, in.ExpiresAt,
		); err != nil {
			return n, err
		}
		keep[[2]string{in.Value, string(in.Kind)}] = true
		n++
	}

	rows, err := tx.Query(`SELECT value, kind FROM indicators WHERE source_id = ?`, sourceID)
	if err != nil {
		return n, err
	}
	var stale [][2]string
	for rows.Next() {
		var v, k string
		if err := rows.Scan(&v, &k); err != nil {
			rows.Close()
			return n, err
		}
		if !keep[[2]string{v, k}] {
			stale = append(stale, [2]string{v, k})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return n, err
	}
	for _, sk := range stale {
		if _, err := tx.Exec(
			`DELETE FROM indicators WHERE source_id = ? AND value = ? AND kind = ?`,
			sourceID, sk[0], sk[1],
		); err != nil {
			return n, err
		}
	}
	return n, tx.Commit()
}

func (s *Store) ListByKind(kind model.Kind) ([]model.Indicator, error) {
	rows, err := s.db.Query(`
		SELECT id, value, kind, action, scope, note, source_id,
		       first_seen, last_seen, expires_at, enabled
		FROM indicators
		WHERE kind = ? AND enabled = 1 AND (expires_at = 0 OR expires_at > strftime('%s','now'))
		ORDER BY value`, string(kind))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIndicators(rows)
}

func (s *Store) ListAll() ([]model.Indicator, error) {
	rows, err := s.db.Query(`
		SELECT id, value, kind, action, scope, note, source_id,
		       first_seen, last_seen, expires_at, enabled
		FROM indicators
		WHERE enabled = 1 AND kind IN ('ip','cidr','domain','url','mac')
		  AND (expires_at = 0 OR expires_at > strftime('%s','now'))
		ORDER BY kind, value`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIndicators(rows)
}

func (s *Store) ListBySource(sourceID int64, limit int) ([]model.Indicator, int, error) {
	var total int
	if err := s.db.QueryRow(
		`SELECT COUNT(1) FROM indicators WHERE source_id = ?`, sourceID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`
		SELECT id, value, kind, action, scope, note, source_id,
		       first_seen, last_seen, expires_at, enabled
		FROM indicators
		WHERE source_id = ?
		ORDER BY kind, value LIMIT ?`, sourceID, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	inds, err := scanIndicators(rows)
	return inds, total, err
}

func (s *Store) UpdateNote(id int64, note string) (int64, error) {
	res, err := s.db.Exec(`UPDATE indicators SET note = ? WHERE id = ?`, note, id)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) CountByKind() (map[model.Kind]int, error) {
	rows, err := s.db.Query(`SELECT kind, COUNT(1) FROM indicators WHERE enabled = 1 GROUP BY kind`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[model.Kind]int)
	for rows.Next() {
		var k string
		var c int
		if err := rows.Scan(&k, &c); err != nil {
			return nil, err
		}
		out[model.Kind(k)] = c
	}
	return out, rows.Err()
}

func (s *Store) BuildRuleset() (model.Ruleset, error) {
	rs := model.Ruleset{Allow: model.AllowSet{Domains: map[string]bool{}}}
	// MAC-решения с учётом источника: ручные (source_id=0) — «фундаментальные».
	manualBlock := map[string]bool{}
	manualAllow := map[string]bool{}
	feedBlock := map[string]bool{}
	feedAllow := map[string]bool{}

	rows, err := s.db.Query(`
		SELECT value, kind, action, scope, source_id
		FROM indicators
		WHERE enabled = 1
		  AND kind IN ('ip','cidr','domain','url','mac')
		  AND (expires_at = 0 OR expires_at > strftime('%s','now'))`)
	if err != nil {
		return rs, err
	}

	for rows.Next() {
		var value, kind, action, scope string
		var sourceID int64
		if err := rows.Scan(&value, &kind, &action, &scope, &sourceID); err != nil {
			rows.Close()
			return rs, err
		}
		switch model.Kind(kind) {
		case model.KindIP, model.KindCIDR:
			p, ok := parsePrefix(value)
			if !ok {
				continue
			}
			if model.Action(action) == model.ActionAllow {
				rs.Allow.IPs = append(rs.Allow.IPs, p)
			} else if p.Addr().Is4() {
				rs.IPv4 = append(rs.IPv4, p)
			} else {
				rs.IPv6 = append(rs.IPv6, p)
			}
		case model.KindMAC:
			v := model.NormalizeMAC(value)
			manual := sourceID == ManualSourceID
			switch model.Action(action) {
			case model.ActionAllow:
				if manual {
					manualAllow[v] = true
				} else {
					feedAllow[v] = true
				}
			case model.ActionBlock:
				if manual {
					manualBlock[v] = true
				} else {
					feedBlock[v] = true
				}
			}
		case model.KindDomain:
			if model.Action(action) == model.ActionAllow {
				rs.Allow.Domains[value] = true
				continue
			}
			rs.Domains = append(rs.Domains, model.DomainRule{
				Domain: value, Scope: model.Scope(scope),
				Action: model.Action(action),
			})
		case model.KindURL:
			if model.Action(action) == model.ActionAllow {
				continue
			}
			rs.URLs = append(rs.URLs, model.URLRule{
				URL: value, Action: model.Action(action),
			})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return rs, err
	}
	// Закрываем rows явно: соединение с БД одно, а дальше идут новые запросы.
	rows.Close()

	gBlock, gAllow, active, err := s.GroupMACDecisions()
	if err != nil {
		return rs, err
	}
	rs.MACs = resolveMACBlocks(manualBlock, manualAllow, feedBlock, feedAllow, gBlock, gAllow, active)
	return rs, nil
}

// resolveMACBlocks сводит MAC-решения по приоритету. Без активных групп сохраняется
// прежняя семантика (любой allow перекрывает любой block). С активными группами
// приоритет строгий: ручное правило > групповое > фид.
func resolveMACBlocks(manualBlock, manualAllow, feedBlock, feedAllow, gBlock, gAllow map[string]bool, active bool) []string {
	var out []string
	if !active {
		for v := range manualBlock {
			if !manualAllow[v] && !feedAllow[v] {
				out = append(out, v)
			}
		}
		for v := range feedBlock {
			if manualBlock[v] {
				continue
			}
			if !manualAllow[v] && !feedAllow[v] {
				out = append(out, v)
			}
		}
		return out
	}

	keys := map[string]bool{}
	for _, m := range []map[string]bool{manualBlock, manualAllow, feedBlock, feedAllow, gBlock, gAllow} {
		for v := range m {
			keys[v] = true
		}
	}
	for v := range keys {
		blocked := false
		switch {
		case manualBlock[v]:
			blocked = true
		case manualAllow[v]:
			blocked = false
		case gBlock[v]:
			blocked = true
		case gAllow[v]:
			blocked = false
		case feedBlock[v]:
			blocked = true
		}
		if blocked {
			out = append(out, v)
		}
	}
	return out
}

func parsePrefix(v string) (netip.Prefix, bool) {
	if p, err := netip.ParsePrefix(v); err == nil {
		return p, true
	}
	if a, err := netip.ParseAddr(v); err == nil {
		return netip.PrefixFrom(a, a.BitLen()), true
	}
	return netip.Prefix{}, false
}

func scanIndicators(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]model.Indicator, error) {
	var out []model.Indicator
	for rows.Next() {
		var in model.Indicator
		var kind, action, scope string
		var enabled int
		if err := rows.Scan(
			&in.ID, &in.Value, &kind, &action, &scope, &in.Note,
			&in.SourceID, &in.FirstSeen, &in.LastSeen, &in.ExpiresAt, &enabled,
		); err != nil {
			return nil, err
		}
		in.Kind = model.Kind(kind)
		in.Action = model.Action(action)
		in.Scope = model.Scope(scope)
		in.Enabled = enabled != 0
		out = append(out, in)
	}
	return out, rows.Err()
}
