package store

import (
	"time"

	"github.com/coffeinium/chaff/internal/model"
)

// ManualSourceID помечает индикаторы, заведённые руками (allowlist, ручные
// блоки), в отличие от пришедших из фидов.
const ManualSourceID = 0

// AddManual добавляет/обновляет индикатор, заведённый оператором (например,
// allow-исключение для легит-инфры). Пустой Kind определяется автоматически.
func (s *Store) AddManual(value string, kind model.Kind, action model.Action) error {
	if kind == model.KindUnknown {
		kind = model.Classify(value)
	}
	now := time.Now().Unix()
	_, err := s.db.Exec(`
		INSERT INTO indicators (value, kind, action, scope, source_id, first_seen, last_seen, enabled)
		VALUES (?, ?, ?, 'domain', ?, ?, ?, 1)
		ON CONFLICT(value, kind, source_id) DO UPDATE SET action = excluded.action, last_seen = excluded.last_seen, enabled = 1`,
		value, string(kind), string(action), ManualSourceID, now, now)
	return err
}

// RemoveManual удаляет ручной индикатор по значению.
func (s *Store) RemoveManual(value string) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM indicators WHERE value = ? AND source_id = ?`, value, ManualSourceID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ListManual — ручные индикаторы с заданным действием.
func (s *Store) ListManual(action model.Action) ([]model.Indicator, error) {
	rows, err := s.db.Query(`
		SELECT id, value, kind, action, scope, threat, note, source_id,
		       first_seen, last_seen, expires_at, enabled
		FROM indicators WHERE source_id = ? AND action = ? ORDER BY value`,
		ManualSourceID, string(action))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIndicators(rows)
}
