package store

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/coffeinium/chaff/migrations"
)

func (s *Store) migrate() error {
	if _, err := s.db.Exec(
		`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at INTEGER)`,
	); err != nil {
		return fmt.Errorf("создать schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var seen int
		if err := s.db.QueryRow(
			`SELECT COUNT(1) FROM schema_migrations WHERE name = ?`, name,
		).Scan(&seen); err != nil {
			return err
		}
		if seen > 0 {
			continue
		}
		body, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			return err
		}
		if err := s.applyMigration(name, string(body)); err != nil {
			return fmt.Errorf("применить %s: %w", name, err)
		}
	}
	return nil
}

func (s *Store) applyMigration(name, body string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, stmt := range splitStatements(body) {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("statement %q: %w", firstLine(stmt), err)
		}
	}
	if _, err := tx.Exec(
		`INSERT INTO schema_migrations(name, applied_at) VALUES(?, strftime('%s','now'))`, name,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// splitStatements выкидывает строки-комментарии "--" (в них бывает ';') и режет
// остаток на отдельные стейтменты. В наших миграциях точки с запятой внутри
// строковых литералов нет, так что это безопасно.
func splitStatements(body string) []string {
	var clean strings.Builder
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		clean.WriteString(line)
		clean.WriteByte('\n')
	}
	var out []string
	for _, stmt := range strings.Split(clean.String(), ";") {
		if stmt = strings.TrimSpace(stmt); stmt != "" {
			out = append(out, stmt)
		}
	}
	return out
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
