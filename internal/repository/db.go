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

// runMigrations applies numbered migrations tracked by PRAGMA user_version.
// Each migration runs only when the current version is below its target version.
func runMigrations(db *sql.DB) error {
	var version int
	_ = db.QueryRow("PRAGMA user_version").Scan(&version)

	if version < 1 {
		if err := execSQL(db, migrations.SQL1); err != nil {
			return fmt.Errorf("migration 001: %w", err)
		}
		if _, err := db.Exec("PRAGMA user_version = 1"); err != nil {
			return fmt.Errorf("set user_version 1: %w", err)
		}
	}

	if version < 2 {
		// Temporarily disable FK enforcement to allow table rebuild (DROP + RENAME).
		if _, err := db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
			return fmt.Errorf("disable FK: %w", err)
		}
		migrErr := execSQL(db, migrations.SQL2)
		db.Exec("PRAGMA foreign_keys = ON") // always re-enable
		if migrErr != nil {
			return fmt.Errorf("migration 002: %w", migrErr)
		}
		if _, err := db.Exec("PRAGMA user_version = 2"); err != nil {
			return fmt.Errorf("set user_version 2: %w", err)
		}
	}

	if version < 3 {
		if err := execSQL(db, migrations.SQL3); err != nil {
			return fmt.Errorf("migration 003: %w", err)
		}
		if _, err := db.Exec("PRAGMA user_version = 3"); err != nil {
			return fmt.Errorf("set user_version 3: %w", err)
		}
	}

	return nil
}

// execSQL executes a multi-statement SQL string by splitting on ";" and
// skipping empty or comment-only chunks.
func execSQL(db *sql.DB, sql string) error {
	for _, stmt := range strings.Split(sql, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if isAllComments(stmt) {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("%w\n  stmt: %.120s", err, stmt)
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
