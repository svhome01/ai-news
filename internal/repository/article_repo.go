package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"ai-news/internal/domain"
)

// ThumbnailRef holds the article ID and its local thumbnail serving path.
// Used by cleanup_usecase to delete thumbnail files before removing article rows.
type ThumbnailRef struct {
	ArticleID    int64
	ThumbnailURL string
}

// ArticleRepo provides access to the articles table.
type ArticleRepo struct{ db *sql.DB }

func NewArticleRepo(db *sql.DB) *ArticleRepo { return &ArticleRepo{db: db} }

// Save inserts an article, silently ignoring duplicate URLs (ON CONFLICT IGNORE).
// Returns the new row's ID, or 0 if the URL already existed.
func (r *ArticleRepo) Save(ctx context.Context, a *domain.Article) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO articles
			(source_id, pipeline_run_id, title, url, thumbnail_url, raw_content, category)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.SourceID, a.PipelineRunID, a.Title, a.URL,
		a.ThumbnailURL, a.RawContent, a.Category,
	)
	if err != nil {
		return 0, fmt.Errorf("article save: %w", err)
	}
	return res.LastInsertId()
}

// GetByID returns a single article or domain.ErrNotFound.
func (r *ArticleRepo) GetByID(ctx context.Context, id int64) (*domain.Article, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, source_id, pipeline_run_id, broadcast_id,
		        title, url, thumbnail_url, raw_content, category,
		        is_selected, select_rank, summary, created_at, processed_at
		 FROM articles WHERE id = ?`, id)
	return scanArticle(row)
}

// ListByCategory returns the most recent articles for a category (newest first).
func (r *ArticleRepo) ListByCategory(ctx context.Context, category string, limit int) ([]*domain.Article, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, source_id, pipeline_run_id, broadcast_id,
		       title, url, thumbnail_url, raw_content, category,
		       is_selected, select_rank, summary, created_at, processed_at
		FROM articles
		WHERE category = ?
		ORDER BY created_at DESC
		LIMIT ?`, category, limit)
	if err != nil {
		return nil, fmt.Errorf("article list: %w", err)
	}
	defer rows.Close()
	return scanArticles(rows)
}

// ListUnprocessed returns articles for a category that have not yet been assigned
// to a broadcast (broadcast_id IS NULL and is_selected = 0), newest first.
func (r *ArticleRepo) ListUnprocessed(ctx context.Context, category string, limit int) ([]*domain.Article, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, source_id, pipeline_run_id, broadcast_id,
		       title, url, thumbnail_url, raw_content, category,
		       is_selected, select_rank, summary, created_at, processed_at
		FROM articles
		WHERE category = ? AND broadcast_id IS NULL AND is_selected = 0
		ORDER BY created_at DESC
		LIMIT ?`, category, limit)
	if err != nil {
		return nil, fmt.Errorf("article list unprocessed: %w", err)
	}
	defer rows.Close()
	return scanArticles(rows)
}

// CountUnprocessed returns the number of unprocessed articles for a category.
func (r *ArticleRepo) CountUnprocessed(ctx context.Context, category string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM articles WHERE category = ? AND broadcast_id IS NULL AND is_selected = 0`,
		category).Scan(&n)
	return n, err
}

// MarkSelected sets is_selected, select_rank, and summary for an article chosen by Gemini.
func (r *ArticleRepo) MarkSelected(ctx context.Context, id int64, rank int, summary string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE articles SET is_selected = 1, select_rank = ?, summary = ? WHERE id = ?`,
		rank, summary, id)
	return err
}

// SetThumbnailURL updates the local thumbnail serving path after the image has been saved.
func (r *ArticleRepo) SetThumbnailURL(ctx context.Context, id int64, url string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE articles SET thumbnail_url = ? WHERE id = ?`, url, id)
	return err
}

// SetBroadcastID links an article to a broadcast and records the processing timestamp.
func (r *ArticleRepo) SetBroadcastID(ctx context.Context, id, broadcastID int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE articles
		SET broadcast_id = ?, processed_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id = ?`, broadcastID, id)
	return err
}

// NullifyRawContent clears raw_content on articles older than the given number of days
// to keep the database compact. thumbnail_url is intentionally preserved.
func (r *ArticleRepo) NullifyRawContent(ctx context.Context, olderThanDays int) error {
	cutoff := cutoffTime(olderThanDays)
	_, err := r.db.ExecContext(ctx, `
		UPDATE articles SET raw_content = NULL
		WHERE raw_content IS NOT NULL AND created_at < ?`, cutoff)
	return err
}

// FindThumbnailsOlderThan returns (id, thumbnail_url) for articles older than
// the given number of days that have a thumbnail. Call this before DeleteOlderThan
// so the caller can delete the local image files first.
func (r *ArticleRepo) FindThumbnailsOlderThan(ctx context.Context, days int) ([]ThumbnailRef, error) {
	cutoff := cutoffTime(days)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, thumbnail_url FROM articles
		WHERE thumbnail_url IS NOT NULL AND created_at < ?`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("find thumbnails: %w", err)
	}
	defer rows.Close()

	var refs []ThumbnailRef
	for rows.Next() {
		var ref ThumbnailRef
		if err := rows.Scan(&ref.ArticleID, &ref.ThumbnailURL); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

// DeleteOlderThan removes article rows older than the given number of days.
// Always call FindThumbnailsOlderThan → delete local files → then this method.
// broadcast_id foreign keys are cleared automatically via ON DELETE SET NULL.
func (r *ArticleRepo) DeleteOlderThan(ctx context.Context, days int) error {
	cutoff := cutoffTime(days)
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM articles WHERE created_at < ?`, cutoff)
	return err
}

// cutoffTime computes the ISO-8601 UTC timestamp for "now minus N days".
func cutoffTime(days int) string {
	return time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02T15:04:05Z")
}

// ── scan helpers ──────────────────────────────────────────────────────────────

func scanArticle(row *sql.Row) (*domain.Article, error) {
	var a domain.Article
	var isSelectedInt int
	err := row.Scan(
		&a.ID, &a.SourceID, &a.PipelineRunID, &a.BroadcastID,
		&a.Title, &a.URL, &a.ThumbnailURL, &a.RawContent, &a.Category,
		&isSelectedInt, &a.SelectRank, &a.Summary, &a.CreatedAt, &a.ProcessedAt,
	)
	if err == sql.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan article: %w", err)
	}
	a.IsSelected = isSelectedInt == 1
	return &a, nil
}

func scanArticles(rows *sql.Rows) ([]*domain.Article, error) {
	var list []*domain.Article
	for rows.Next() {
		var a domain.Article
		var isSelectedInt int
		if err := rows.Scan(
			&a.ID, &a.SourceID, &a.PipelineRunID, &a.BroadcastID,
			&a.Title, &a.URL, &a.ThumbnailURL, &a.RawContent, &a.Category,
			&isSelectedInt, &a.SelectRank, &a.Summary, &a.CreatedAt, &a.ProcessedAt,
		); err != nil {
			return nil, fmt.Errorf("scan article row: %w", err)
		}
		a.IsSelected = isSelectedInt == 1
		list = append(list, &a)
	}
	return list, rows.Err()
}
