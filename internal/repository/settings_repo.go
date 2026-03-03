package repository

import (
	"context"
	"database/sql"
	"fmt"

	"ai-news/internal/domain"
)

// SettingsRepo provides access to the app_settings table (single row, id = 1).
type SettingsRepo struct{ db *sql.DB }

func NewSettingsRepo(db *sql.DB) *SettingsRepo { return &SettingsRepo{db: db} }

// Get returns the single settings row (id = 1).
func (r *SettingsRepo) Get(ctx context.Context) (*domain.AppSettings, error) {
	var s domain.AppSettings
	err := r.db.QueryRowContext(ctx, `
		SELECT id, voicevox_speed_scale, gemini_model, retention_days, updated_at
		FROM app_settings WHERE id = 1`).Scan(
		&s.ID, &s.VoicevoxSpeedScale, &s.GeminiModel, &s.RetentionDays, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("settings get: %w", err)
	}
	return &s, nil
}

// Update overwrites all mutable fields.
// updated_at is set explicitly because SQLite has no ON UPDATE trigger.
func (r *SettingsRepo) Update(ctx context.Context, s *domain.AppSettings) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE app_settings
		SET voicevox_speed_scale = ?, gemini_model = ?, retention_days = ?,
		    updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id = 1`,
		s.VoicevoxSpeedScale, s.GeminiModel, s.RetentionDays,
	)
	return err
}
