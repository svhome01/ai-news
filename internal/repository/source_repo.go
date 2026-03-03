package repository

import (
	"context"
	"database/sql"
	"fmt"

	"ai-news/internal/domain"
)

// SourceRepo provides access to the sources table.
type SourceRepo struct{ db *sql.DB }

func NewSourceRepo(db *sql.DB) *SourceRepo { return &SourceRepo{db: db} }

// Create inserts a new source and returns its ID.
func (r *SourceRepo) Create(ctx context.Context, s *domain.Source) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO sources (name, url, category, fetch_method, css_selector, enabled)
		VALUES (?, ?, ?, ?, ?, ?)`,
		s.Name, s.URL, s.Category, string(s.FetchMethod), s.CSSSelector, boolToInt(s.Enabled),
	)
	if err != nil {
		return 0, fmt.Errorf("source create: %w", err)
	}
	return res.LastInsertId()
}

// GetByID returns a source or domain.ErrNotFound.
func (r *SourceRepo) GetByID(ctx context.Context, id int64) (*domain.Source, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, url, category, fetch_method, css_selector, enabled, created_at, updated_at
		FROM sources WHERE id = ?`, id)
	return scanSource(row)
}

// List returns all sources ordered by id.
func (r *SourceRepo) List(ctx context.Context) ([]*domain.Source, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, url, category, fetch_method, css_selector, enabled, created_at, updated_at
		FROM sources ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("source list: %w", err)
	}
	defer rows.Close()
	return scanSources(rows)
}

// ListEnabled returns only enabled sources.
func (r *SourceRepo) ListEnabled(ctx context.Context) ([]*domain.Source, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, url, category, fetch_method, css_selector, enabled, created_at, updated_at
		FROM sources WHERE enabled = 1 ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("source list enabled: %w", err)
	}
	defer rows.Close()
	return scanSources(rows)
}

// Update overwrites all mutable fields.
// updated_at is set explicitly because SQLite has no ON UPDATE trigger.
func (r *SourceRepo) Update(ctx context.Context, s *domain.Source) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE sources
		SET name = ?, url = ?, category = ?, fetch_method = ?, css_selector = ?, enabled = ?,
		    updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id = ?`,
		s.Name, s.URL, s.Category, string(s.FetchMethod), s.CSSSelector,
		boolToInt(s.Enabled), s.ID,
	)
	return err
}

// Delete removes a source by ID (cascades to articles via ON DELETE CASCADE).
func (r *SourceRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sources WHERE id = ?`, id)
	return err
}

// ── scan helpers ──────────────────────────────────────────────────────────────

func scanSource(row *sql.Row) (*domain.Source, error) {
	var s domain.Source
	var enabledInt int
	var fetchMethod string
	err := row.Scan(
		&s.ID, &s.Name, &s.URL, &s.Category,
		&fetchMethod, &s.CSSSelector, &enabledInt,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan source: %w", err)
	}
	s.FetchMethod = domain.FetchMethod(fetchMethod)
	s.Enabled = enabledInt == 1
	return &s, nil
}

func scanSources(rows *sql.Rows) ([]*domain.Source, error) {
	var list []*domain.Source
	for rows.Next() {
		var s domain.Source
		var enabledInt int
		var fetchMethod string
		if err := rows.Scan(
			&s.ID, &s.Name, &s.URL, &s.Category,
			&fetchMethod, &s.CSSSelector, &enabledInt,
			&s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan source row: %w", err)
		}
		s.FetchMethod = domain.FetchMethod(fetchMethod)
		s.Enabled = enabledInt == 1
		list = append(list, &s)
	}
	return list, rows.Err()
}

// boolToInt converts a bool to SQLite's integer representation (0/1).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
