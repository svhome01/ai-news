package repository

import (
	"context"
	"database/sql"
	"fmt"

	"ai-news/internal/domain"
)

// ScheduleRepo provides access to the schedules table.
type ScheduleRepo struct{ db *sql.DB }

func NewScheduleRepo(db *sql.DB) *ScheduleRepo { return &ScheduleRepo{db: db} }

// Create inserts a new schedule entry and returns its ID.
func (r *ScheduleRepo) Create(ctx context.Context, s *domain.Schedule) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO schedules (type, hour, minute, enabled) VALUES (?, ?, ?, ?)`,
		s.Type, s.Hour, s.Minute, boolToInt(s.Enabled),
	)
	if err != nil {
		return 0, fmt.Errorf("schedule create: %w", err)
	}
	return res.LastInsertId()
}

// GetByID returns a schedule or domain.ErrNotFound.
func (r *ScheduleRepo) GetByID(ctx context.Context, id int64) (*domain.Schedule, error) {
	var s domain.Schedule
	var enabledInt int
	err := r.db.QueryRowContext(ctx,
		`SELECT id, type, hour, minute, enabled FROM schedules WHERE id = ?`, id).
		Scan(&s.ID, &s.Type, &s.Hour, &s.Minute, &enabledInt)
	if err == sql.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("schedule get: %w", err)
	}
	s.Enabled = enabledInt == 1
	return &s, nil
}

// List returns all schedules ordered by type and hour.
func (r *ScheduleRepo) List(ctx context.Context) ([]*domain.Schedule, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, type, hour, minute, enabled FROM schedules ORDER BY type, hour, minute`)
	if err != nil {
		return nil, fmt.Errorf("schedule list: %w", err)
	}
	defer rows.Close()
	return scanSchedules(rows)
}

// ListByType returns schedules of the given type ("scrape" or "generate").
func (r *ScheduleRepo) ListByType(ctx context.Context, schedType string) ([]*domain.Schedule, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, type, hour, minute, enabled FROM schedules WHERE type = ? ORDER BY hour, minute`,
		schedType)
	if err != nil {
		return nil, fmt.Errorf("schedule list by type: %w", err)
	}
	defer rows.Close()
	return scanSchedules(rows)
}

// Update overwrites a schedule's hour, minute, and enabled flag.
func (r *ScheduleRepo) Update(ctx context.Context, s *domain.Schedule) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE schedules SET hour = ?, minute = ?, enabled = ? WHERE id = ?`,
		s.Hour, s.Minute, boolToInt(s.Enabled), s.ID)
	return err
}

// Delete removes a schedule by ID.
func (r *ScheduleRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM schedules WHERE id = ?`, id)
	return err
}

// ── scan helper ───────────────────────────────────────────────────────────────

func scanSchedules(rows *sql.Rows) ([]*domain.Schedule, error) {
	var list []*domain.Schedule
	for rows.Next() {
		var s domain.Schedule
		var enabledInt int
		if err := rows.Scan(&s.ID, &s.Type, &s.Hour, &s.Minute, &enabledInt); err != nil {
			return nil, fmt.Errorf("scan schedule row: %w", err)
		}
		s.Enabled = enabledInt == 1
		list = append(list, &s)
	}
	return list, rows.Err()
}
