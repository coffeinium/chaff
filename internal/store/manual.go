package store

import (
	"time"

	"github.com/coffeinium/chaff/internal/model"
)

const ManualSourceID = 0

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

func (s *Store) RemoveManual(value string) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM indicators WHERE value = ? AND source_id = ?`, value, ManualSourceID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

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
