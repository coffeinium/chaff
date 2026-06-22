// Пакет feed — помощники парсинга, общие для адаптеров-источников. FSTEC CSV —
// лишь один из примеров формата; здесь всё format-agnostic и рулится конфигом.
package feed

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ReadURI тянет сырые байты источника. http(s):// — по сети, всё остальное —
// локальный путь.
func ReadURI(ctx context.Context, uri string) ([]byte, error) {
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
		if err != nil {
			return nil, err
		}
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetch %s: статус %d", uri, resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	}
	return os.ReadFile(strings.TrimPrefix(uri, "file://"))
}

// Hash — стабильный отпечаток содержимого для детекта изменений.
func Hash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ParseCSV разбирает CSV, срезая UTF-8 BOM и, при skipHeader, строку заголовка.
// Записи с разным числом полей не считаются ошибкой.
func ParseCSV(b []byte, skipHeader bool) ([][]string, error) {
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	r := csv.NewReader(bytes.NewReader(b))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if skipHeader && len(rows) > 0 {
		rows = rows[1:]
	}
	return rows, nil
}

// Lines режет текст на обрезанные строки без комментариев и пустых.
func Lines(b []byte) []string {
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	var out []string
	for _, ln := range strings.Split(string(b), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		out = append(out, ln)
	}
	return out
}

// Col безопасно достаёт колонку i из записи, или "" если её нет.
func Col(rec []string, i int) string {
	if i >= 0 && i < len(rec) {
		return strings.TrimSpace(rec[i])
	}
	return ""
}
