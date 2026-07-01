package store

import (
	"database/sql"
	"errors"
)

type WebToken struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Hash      string `json:"-"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
	LastUsed  int64  `json:"last_used"`
}

func (s *Store) AddToken(name, hash string, createdAt, expiresAt int64) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO web_tokens (name, hash, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		name, hash, createdAt, expiresAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) TokenByHash(hash string) (WebToken, bool, error) {
	var t WebToken
	err := s.db.QueryRow(
		`SELECT id, name, hash, created_at, expires_at, last_used FROM web_tokens WHERE hash = ?`, hash,
	).Scan(&t.ID, &t.Name, &t.Hash, &t.CreatedAt, &t.ExpiresAt, &t.LastUsed)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WebToken{}, false, nil
		}
		return WebToken{}, false, err
	}
	return t, true, nil
}

func (s *Store) TokenByID(id int64) (WebToken, bool, error) {
	var t WebToken
	err := s.db.QueryRow(
		`SELECT id, name, hash, created_at, expires_at, last_used FROM web_tokens WHERE id = ?`, id,
	).Scan(&t.ID, &t.Name, &t.Hash, &t.CreatedAt, &t.ExpiresAt, &t.LastUsed)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WebToken{}, false, nil
		}
		return WebToken{}, false, err
	}
	return t, true, nil
}

func (s *Store) ListTokens() ([]WebToken, error) {
	rows, err := s.db.Query(
		`SELECT id, name, hash, created_at, expires_at, last_used FROM web_tokens ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WebToken
	for rows.Next() {
		var t WebToken
		if err := rows.Scan(&t.ID, &t.Name, &t.Hash, &t.CreatedAt, &t.ExpiresAt, &t.LastUsed); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) TouchToken(id, ts int64) error {
	_, err := s.db.Exec(`UPDATE web_tokens SET last_used = ? WHERE id = ?`, ts, id)
	return err
}

func (s *Store) RemoveTokenByID(id int64) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM web_tokens WHERE id = ?`, id)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) RemoveTokenByName(name string) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM web_tokens WHERE name = ?`, name)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
