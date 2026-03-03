package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"ai-news/internal/domain"
)

// BroadcastRepo provides access to the broadcasts table.
type BroadcastRepo struct{ db *sql.DB }

func NewBroadcastRepo(db *sql.DB) *BroadcastRepo { return &BroadcastRepo{db: db} }

// Create inserts a new broadcast and returns its ID.
func (r *BroadcastRepo) Create(ctx context.Context, b *domain.Broadcast) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO broadcasts
			(pipeline_run_id, category, broadcast_type, title, script, file_path, file_url, duration_sec)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		b.PipelineRunID, b.Category, string(b.BroadcastType),
		b.Title, b.Script, b.FilePath, b.FileURL, b.DurationSec,
	)
	if err != nil {
		return 0, fmt.Errorf("broadcast create: %w", err)
	}
	return res.LastInsertId()
}

// GetByID returns a broadcast or domain.ErrNotFound.
func (r *BroadcastRepo) GetByID(ctx context.Context, id int64) (*domain.Broadcast, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, pipeline_run_id, category, broadcast_type, title, script,
		       file_path, file_url, duration_sec, created_at
		FROM broadcasts WHERE id = ?`, id)
	return scanBroadcast(row)
}

// GetLatest returns the most recently created broadcast for a category.
func (r *BroadcastRepo) GetLatest(ctx context.Context, category string) (*domain.Broadcast, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, pipeline_run_id, category, broadcast_type, title, script,
		       file_path, file_url, duration_sec, created_at
		FROM broadcasts WHERE category = ?
		ORDER BY created_at DESC LIMIT 1`, category)
	return scanBroadcast(row)
}

// ListByCategory returns recent broadcasts for a category (newest first).
func (r *BroadcastRepo) ListByCategory(ctx context.Context, category string, limit int) ([]*domain.Broadcast, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, pipeline_run_id, category, broadcast_type, title, script,
		       file_path, file_url, duration_sec, created_at
		FROM broadcasts WHERE category = ?
		ORDER BY created_at DESC LIMIT ?`, category, limit)
	if err != nil {
		return nil, fmt.Errorf("broadcast list: %w", err)
	}
	defer rows.Close()
	return scanBroadcasts(rows)
}

// SetFileURL updates the HTTP URL for a broadcast after it has been stored.
func (r *BroadcastRepo) SetFileURL(ctx context.Context, id int64, fileURL string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE broadcasts SET file_url = ? WHERE id = ?`, fileURL, id)
	return err
}

// SetDuration records the audio duration obtained from ffprobe.
func (r *BroadcastRepo) SetDuration(ctx context.Context, id int64, secs int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE broadcasts SET duration_sec = ? WHERE id = ?`, secs, id)
	return err
}

// ListOlderThan returns broadcasts created more than the given number of days ago.
// The caller should use the returned file_paths to delete SMB MP3 files before
// calling DeleteByIDs.
func (r *BroadcastRepo) ListOlderThan(ctx context.Context, days int) ([]*domain.Broadcast, error) {
	cutoff := cutoffTime(days)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, pipeline_run_id, category, broadcast_type, title, script,
		       file_path, file_url, duration_sec, created_at
		FROM broadcasts WHERE created_at < ?
		ORDER BY created_at`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("broadcast list older: %w", err)
	}
	defer rows.Close()
	return scanBroadcasts(rows)
}

// DeleteByIDs removes broadcasts by their IDs.
// ON DELETE SET NULL automatically clears articles.broadcast_id for linked articles.
func (r *BroadcastRepo) DeleteByIDs(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM broadcasts WHERE id IN (`+placeholders+`)`, args...)
	return err
}

// ── scan helpers ──────────────────────────────────────────────────────────────

func scanBroadcast(row *sql.Row) (*domain.Broadcast, error) {
	var b domain.Broadcast
	var bt string
	err := row.Scan(
		&b.ID, &b.PipelineRunID, &b.Category, &bt,
		&b.Title, &b.Script, &b.FilePath, &b.FileURL, &b.DurationSec, &b.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan broadcast: %w", err)
	}
	b.BroadcastType = domain.BroadcastType(bt)
	return &b, nil
}

func scanBroadcasts(rows *sql.Rows) ([]*domain.Broadcast, error) {
	var list []*domain.Broadcast
	for rows.Next() {
		var b domain.Broadcast
		var bt string
		if err := rows.Scan(
			&b.ID, &b.PipelineRunID, &b.Category, &bt,
			&b.Title, &b.Script, &b.FilePath, &b.FileURL, &b.DurationSec, &b.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan broadcast row: %w", err)
		}
		b.BroadcastType = domain.BroadcastType(bt)
		list = append(list, &b)
	}
	return list, rows.Err()
}
