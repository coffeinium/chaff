package store

import "time"

type Hit struct {
	TS        int64  `json:"ts"`
	Layer     string `json:"layer"`
	Indicator string `json:"indicator"`
	SrcIP     string `json:"src_ip,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

func (s *Store) AddHit(h Hit) error {
	if h.TS == 0 {
		h.TS = time.Now().Unix()
	}
	_, err := s.db.Exec(`
		INSERT INTO hits (ts, layer, indicator, src_ip, detail) VALUES (?, ?, ?, ?, ?)`,
		h.TS, h.Layer, h.Indicator, h.SrcIP, h.Detail)
	return err
}

func (s *Store) RecentHits(limit int) ([]Hit, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`
		SELECT ts, layer, indicator, src_ip, detail FROM hits ORDER BY ts DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.TS, &h.Layer, &h.Indicator, &h.SrcIP, &h.Detail); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
