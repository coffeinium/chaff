package store

import "github.com/coffeinium/chaff/internal/model"

// Lookup — включённые индикаторы с точным совпадением значения. Нужен команде
// test, чтобы объяснить, сработает ли значение и на каком слое.
func (s *Store) Lookup(value string) ([]model.Indicator, error) {
	rows, err := s.db.Query(`
		SELECT id, value, kind, action, scope, threat, note, source_id,
		       first_seen, last_seen, expires_at, enabled
		FROM indicators WHERE value = ? AND enabled = 1`, value)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIndicators(rows)
}
