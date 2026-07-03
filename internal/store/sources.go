package store

import (
	"encoding/json"
	"time"

	"github.com/coffeinium/chaff/internal/model"
)

func (s *Store) AddSource(spec model.SourceSpec) (int64, error) {
	cm, err := json.Marshal(spec.ColumnMap)
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`
		INSERT INTO sources (name, adapter, uri, column_map, enabled)
		VALUES (?, ?, ?, ?, 1)`,
		spec.Name, spec.Adapter, spec.URI, string(cm))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListSources() ([]model.SourceSpec, error) {
	rows, err := s.db.Query(`
		SELECT id, name, adapter, uri, column_map, enabled, last_sync, last_status, last_count
		FROM sources ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSources(rows)
}

func (s *Store) EnabledSources() ([]model.SourceSpec, error) {
	rows, err := s.db.Query(`
		SELECT id, name, adapter, uri, column_map, enabled, last_sync, last_status, last_count
		FROM sources WHERE enabled = 1 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSources(rows)
}

func (s *Store) RemoveSource(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM indicators WHERE source_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM sources WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UpdateSourceURI(id int64, uri string) error {
	_, err := s.db.Exec(`UPDATE sources SET uri = ? WHERE id = ?`, uri, id)
	return err
}

func (s *Store) SetSourceEnabled(id int64, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE sources SET enabled = ? WHERE id = ?`, v, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE indicators SET enabled = ? WHERE source_id = ?`, v, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UpdateSourceStatus(id int64, status string, count int, hash string) error {
	_, err := s.db.Exec(`
		UPDATE sources SET last_sync = ?, last_status = ?, last_count = ?, content_hash = ?
		WHERE id = ?`,
		time.Now().Unix(), status, count, hash, id)
	return err
}

func scanSources(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]model.SourceSpec, error) {
	var out []model.SourceSpec
	for rows.Next() {
		var spec model.SourceSpec
		var cm string
		var enabled int
		if err := rows.Scan(
			&spec.ID, &spec.Name, &spec.Adapter, &spec.URI, &cm, &enabled,
			&spec.LastSync, &spec.LastStatus, &spec.LastCount,
		); err != nil {
			return nil, err
		}
		spec.Enabled = enabled != 0
		spec.ColumnMap = map[string]int{}
		_ = json.Unmarshal([]byte(cm), &spec.ColumnMap)
		out = append(out, spec)
	}
	return out, rows.Err()
}
