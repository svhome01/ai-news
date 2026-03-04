package repository

import (
	"context"
	"database/sql"
	"fmt"

	"ai-news/internal/domain"
)

// CategoryRepo provides access to the category_settings table.
type CategoryRepo struct{ db *sql.DB }

func NewCategoryRepo(db *sql.DB) *CategoryRepo { return &CategoryRepo{db: db} }

// Create inserts a new category.
func (r *CategoryRepo) Create(ctx context.Context, c *domain.CategorySettings) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO category_settings
			(category, display_name, articles_per_episode, summary_chars_per_article,
			 language, tts_engine, voicevox_speaker_id, tts_voice, speed_scale, enabled, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Category, c.DisplayName, c.ArticlesPerEpisode, c.SummaryCharsPerArticle,
		c.Language, c.TTSEngine, c.VoicevoxSpeakerID, c.TTSVoice, c.SpeedScale,
		boolToInt(c.Enabled), c.SortOrder,
	)
	if err != nil {
		return fmt.Errorf("category create: %w", err)
	}
	return nil
}

// GetByName returns a category or domain.ErrNotFound.
func (r *CategoryRepo) GetByName(ctx context.Context, name string) (*domain.CategorySettings, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT category, display_name, articles_per_episode, summary_chars_per_article,
		       language, tts_engine, voicevox_speaker_id, tts_voice, speed_scale, enabled, sort_order, created_at
		FROM category_settings WHERE category = ?`, name)
	return scanCategory(row)
}

// List returns all categories ordered by sort_order.
func (r *CategoryRepo) List(ctx context.Context) ([]*domain.CategorySettings, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT category, display_name, articles_per_episode, summary_chars_per_article,
		       language, tts_engine, voicevox_speaker_id, tts_voice, speed_scale, enabled, sort_order, created_at
		FROM category_settings ORDER BY sort_order, category`)
	if err != nil {
		return nil, fmt.Errorf("category list: %w", err)
	}
	defer rows.Close()
	return scanCategories(rows)
}

// ListEnabled returns only enabled categories, ordered by sort_order.
func (r *CategoryRepo) ListEnabled(ctx context.Context) ([]*domain.CategorySettings, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT category, display_name, articles_per_episode, summary_chars_per_article,
		       language, tts_engine, voicevox_speaker_id, tts_voice, speed_scale, enabled, sort_order, created_at
		FROM category_settings WHERE enabled = 1 ORDER BY sort_order, category`)
	if err != nil {
		return nil, fmt.Errorf("category list enabled: %w", err)
	}
	defer rows.Close()
	return scanCategories(rows)
}

// Update overwrites all mutable fields of a category.
func (r *CategoryRepo) Update(ctx context.Context, c *domain.CategorySettings) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE category_settings
		SET display_name = ?, articles_per_episode = ?, summary_chars_per_article = ?,
		    language = ?, tts_engine = ?, voicevox_speaker_id = ?, tts_voice = ?,
		    speed_scale = ?, enabled = ?, sort_order = ?
		WHERE category = ?`,
		c.DisplayName, c.ArticlesPerEpisode, c.SummaryCharsPerArticle,
		c.Language, c.TTSEngine, c.VoicevoxSpeakerID, c.TTSVoice,
		c.SpeedScale, boolToInt(c.Enabled), c.SortOrder, c.Category,
	)
	return err
}

// Delete removes a category by name.
func (r *CategoryRepo) Delete(ctx context.Context, name string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM category_settings WHERE category = ?`, name)
	return err
}

// ── scan helpers ──────────────────────────────────────────────────────────────

func scanCategory(row *sql.Row) (*domain.CategorySettings, error) {
	var c domain.CategorySettings
	var enabledInt int
	err := row.Scan(
		&c.Category, &c.DisplayName, &c.ArticlesPerEpisode, &c.SummaryCharsPerArticle,
		&c.Language, &c.TTSEngine, &c.VoicevoxSpeakerID, &c.TTSVoice,
		&c.SpeedScale, &enabledInt, &c.SortOrder, &c.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan category: %w", err)
	}
	c.Enabled = enabledInt == 1
	return &c, nil
}

func scanCategories(rows *sql.Rows) ([]*domain.CategorySettings, error) {
	var list []*domain.CategorySettings
	for rows.Next() {
		var c domain.CategorySettings
		var enabledInt int
		if err := rows.Scan(
			&c.Category, &c.DisplayName, &c.ArticlesPerEpisode, &c.SummaryCharsPerArticle,
			&c.Language, &c.TTSEngine, &c.VoicevoxSpeakerID, &c.TTSVoice,
			&c.SpeedScale, &enabledInt, &c.SortOrder, &c.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan category row: %w", err)
		}
		c.Enabled = enabledInt == 1
		list = append(list, &c)
	}
	return list, rows.Err()
}
