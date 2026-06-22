package store

import "database/sql"

func (s *Store) IsModuleEnabled(name string) bool {
	var enabled int
	err := s.db.QueryRow(`SELECT enabled FROM modules WHERE name = ?`, name).Scan(&enabled)
	if err != nil {
		return true
	}
	return enabled != 0
}

func (s *Store) SetModuleEnabled(name string, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO modules (name, enabled) VALUES (?, ?)
		ON CONFLICT(name) DO UPDATE SET enabled = excluded.enabled`, name, v)
	return err
}

func (s *Store) GetModuleConfig(name string) (string, error) {
	var cfg string
	err := s.db.QueryRow(`SELECT config FROM modules WHERE name = ?`, name).Scan(&cfg)
	if err == sql.ErrNoRows || cfg == "" {
		return "{}", nil
	}
	if err != nil {
		return "", err
	}
	return cfg, nil
}

func (s *Store) SetModuleConfig(name, cfg string) error {
	_, err := s.db.Exec(`
		INSERT INTO modules (name, config) VALUES (?, ?)
		ON CONFLICT(name) DO UPDATE SET config = excluded.config`, name, cfg)
	return err
}
