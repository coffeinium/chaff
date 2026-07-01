package store

import (
	"database/sql"
	"time"
)

func (s *Store) PutSnoop(domain, ip string, ttl time.Duration) error {
	now := time.Now()
	_, err := s.db.Exec(`
		INSERT INTO snoop (domain, ip, seen_at, expires_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(domain, ip) DO UPDATE SET seen_at = excluded.seen_at, expires_at = excluded.expires_at`,
		domain, ip, now.Unix(), now.Add(ttl).Unix())
	return err
}

func (s *Store) ExpireSnoop() (int64, error) {
	res, err := s.db.Exec(`DELETE FROM snoop WHERE expires_at > 0 AND expires_at <= strftime('%s','now')`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) DomainForIP(ip string) (string, bool, error) {
	var d string
	err := s.db.QueryRow(`
		SELECT domain FROM snoop
		WHERE ip = ? AND (expires_at = 0 OR expires_at > strftime('%s','now'))
		ORDER BY seen_at DESC LIMIT 1`, ip).Scan(&d)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}
	return d, true, nil
}

func (s *Store) SnoopedIPsForBlockedDomains() ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT sn.ip
		FROM snoop sn
		JOIN indicators i ON i.value = sn.domain
		WHERE i.kind = 'domain' AND i.enabled = 1 AND i.action = 'block'
		  AND (sn.expires_at = 0 OR sn.expires_at > strftime('%s','now'))`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return nil, err
		}
		out = append(out, ip)
	}
	return out, rows.Err()
}
