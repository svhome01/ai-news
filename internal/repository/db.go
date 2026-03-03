package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"ai-news/migrations"
	_ "modernc.org/sqlite"
)

// OpenDB opens the SQLite database at the given path, applies WAL mode,
// enables foreign key enforcement, and runs the schema migrations.
// SetMaxOpenConns(1) is required because SQLite only supports one writer.
func OpenDB(path string) (*sql.DB, error) {
	// _busy_timeout gives concurrent readers a grace period instead of immediately failing.
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// WAL must be set outside a transaction via a direct Exec.
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL: %w", err)
	}

	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// runMigrations executes the embedded SQL file statement by statement.
// SQLite's database/sql driver does not support multi-statement Exec,
// so we split on ";" and skip empty/comment-only lines.
func runMigrations(db *sql.DB) error {
	for _, stmt := range strings.Split(migrations.SQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		// Skip leading comment blocks (lines starting with --)
		if isAllComments(stmt) {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migration: %w\n  stmt: %.120s", err, stmt)
		}
	}
	return nil
}

// isAllComments returns true if every non-empty line in s starts with "--".
func isAllComments(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "--") {
			return false
		}
	}
	return true
}
