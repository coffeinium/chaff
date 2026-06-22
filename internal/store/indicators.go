package store

import (
	"net/netip"
	"time"

	"github.com/coffeinium/chaff/internal/model"
)

func (s *Store) UpsertIndicators(sourceID int64, inds []model.Indicator) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	stmt, err := tx.Prepare(`
		INSERT INTO indicators
			(value, kind, action, scope, threat, note, source_id, first_seen, last_seen, expires_at, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT(value, kind, source_id) DO UPDATE SET
			action     = excluded.action,
			scope      = excluded.scope,
			threat     = excluded.threat,
			note       = excluded.note,
			last_seen  = excluded.last_seen,
			expires_at = excluded.expires_at,
			enabled    = 1
	`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

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
		if _, err := stmt.Exec(
			in.Value, string(in.Kind), string(in.Action), string(in.Scope),
			in.Threat, in.Note, sourceID, now, now, in.ExpiresAt,
		); err != nil {
			return n, err
		}
		n++
	}
	return n, tx.Commit()
}

func (s *Store) ListByKind(kind model.Kind) ([]model.Indicator, error) {
	rows, err := s.db.Query(`
		SELECT id, value, kind, action, scope, threat, note, source_id,
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

	rows, err := s.db.Query(`
		SELECT value, kind, action, scope, threat
		FROM indicators
		WHERE enabled = 1
		  AND kind IN ('ip','cidr','domain','url')
		  AND (expires_at = 0 OR expires_at > strftime('%s','now'))`)
	if err != nil {
		return rs, err
	}
	defer rows.Close()

	for rows.Next() {
		var value, kind, action, scope, threat string
		if err := rows.Scan(&value, &kind, &action, &scope, &threat); err != nil {
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
		case model.KindDomain:
			if model.Action(action) == model.ActionAllow {
				rs.Allow.Domains[value] = true
				continue
			}
			rs.Domains = append(rs.Domains, model.DomainRule{
				Domain: value, Scope: model.Scope(scope),
				Action: model.Action(action), Threat: threat,
			})
		case model.KindURL:
			if model.Action(action) == model.ActionAllow {
				continue
			}
			rs.URLs = append(rs.URLs, model.URLRule{
				URL: value, Action: model.Action(action), Threat: threat,
			})
		}
	}
	return rs, rows.Err()
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
			&in.ID, &in.Value, &kind, &action, &scope, &in.Threat, &in.Note,
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
