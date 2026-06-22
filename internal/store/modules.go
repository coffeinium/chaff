package store

// IsModuleEnabled — должен ли модуль стартовать. По умолчанию включён; выключает
// только явная строка-override.
func (s *Store) IsModuleEnabled(name string) bool {
	var enabled int
	err := s.db.QueryRow(`SELECT enabled FROM modules WHERE name = ?`, name).Scan(&enabled)
	if err != nil {
		return true // нет строки -> включён
	}
	return enabled != 0
}

// SetModuleEnabled фиксирует явное решение вкл/выкл.
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
