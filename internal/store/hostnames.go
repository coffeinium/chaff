package store

import "time"

type HostEntry struct {
	Kind     string `json:"kind"`
	Key      string `json:"key"`
	Hostname string `json:"hostname"`
	Via      string `json:"via"`
	SeenAt   int64  `json:"seen_at"`
}

func (s *Store) PutHostname(kind, key, hostname, via string) error {
	_, err := s.db.Exec(`
		INSERT INTO hostnames (kind, key, hostname, via, seen_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(kind, key) DO UPDATE SET
			hostname = excluded.hostname, via = excluded.via, seen_at = excluded.seen_at`,
		kind, key, hostname, via, time.Now().Unix())
	return err
}

func (s *Store) Hostnames() (byMAC, byIP map[string]string, err error) {
	rows, err := s.db.Query(`SELECT kind, key, hostname FROM hostnames`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	byMAC, byIP = map[string]string{}, map[string]string{}
	for rows.Next() {
		var kind, key, hostname string
		if err := rows.Scan(&kind, &key, &hostname); err != nil {
			return nil, nil, err
		}
		if kind == "mac" {
			byMAC[key] = hostname
		} else {
			byIP[key] = hostname
		}
	}
	return byMAC, byIP, rows.Err()
}

func (s *Store) ListHostnames() ([]HostEntry, error) {
	rows, err := s.db.Query(`SELECT kind, key, hostname, via, seen_at FROM hostnames ORDER BY hostname, kind`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HostEntry
	for rows.Next() {
		var e HostEntry
		if err := rows.Scan(&e.Kind, &e.Key, &e.Hostname, &e.Via, &e.SeenAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
