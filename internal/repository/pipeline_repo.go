package repository

import (
	"context"
	"database/sql"
	"fmt"

	"ai-news/internal/domain"
)

// PipelineRepo provides access to the pipeline_runs table.
type PipelineRepo struct{ db *sql.DB }

func NewPipelineRepo(db *sql.DB) *PipelineRepo { return &PipelineRepo{db: db} }

// Create inserts a new pipeline run and returns its ID.
func (r *PipelineRepo) Create(ctx context.Context, p *domain.PipelineRun) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO pipeline_runs (job_type, status, triggered_by)
		VALUES (?, ?, ?)`,
		string(p.JobType), string(p.Status), string(p.TriggeredBy),
	)
	if err != nil {
		return 0, fmt.Errorf("pipeline create: %w", err)
	}
	return res.LastInsertId()
}

// GetByID returns a pipeline run or domain.ErrNotFound.
func (r *PipelineRepo) GetByID(ctx context.Context, id int64) (*domain.PipelineRun, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, job_type, status, triggered_by, current_step,
		       articles_collected, articles_selected, error_message, started_at, finished_at
		FROM pipeline_runs WHERE id = ?`, id)
	return scanPipelineRun(row)
}

// UpdateStep sets the current_step progress label for HTMX polling.
func (r *PipelineRepo) UpdateStep(ctx context.Context, id int64, step string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE pipeline_runs SET current_step = ? WHERE id = ?`, step, id)
	return err
}

// UpdateCollected records the number of articles collected by a ScrapeJob.
func (r *PipelineRepo) UpdateCollected(ctx context.Context, id int64, count int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE pipeline_runs SET articles_collected = ? WHERE id = ?`, count, id)
	return err
}

// UpdateSelected records the number of articles selected by Gemini in a GenerateJob.
func (r *PipelineRepo) UpdateSelected(ctx context.Context, id int64, count int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE pipeline_runs SET articles_selected = ? WHERE id = ?`, count, id)
	return err
}

// Finish marks the run as completed or failed and records the end time.
func (r *PipelineRepo) Finish(ctx context.Context, id int64, status domain.RunStatus, errMsg *string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE pipeline_runs
		SET status = ?, error_message = ?, finished_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		WHERE id = ?`, string(status), errMsg, id)
	return err
}

// List returns recent pipeline runs (newest first).
func (r *PipelineRepo) List(ctx context.Context, limit int) ([]*domain.PipelineRun, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, job_type, status, triggered_by, current_step,
		       articles_collected, articles_selected, error_message, started_at, finished_at
		FROM pipeline_runs ORDER BY started_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("pipeline list: %w", err)
	}
	defer rows.Close()

	var list []*domain.PipelineRun
	for rows.Next() {
		var p domain.PipelineRun
		var jt, st, tb string
		if err := rows.Scan(
			&p.ID, &jt, &st, &tb, &p.CurrentStep,
			&p.ArticlesCollected, &p.ArticlesSelected, &p.ErrorMessage,
			&p.StartedAt, &p.FinishedAt,
		); err != nil {
			return nil, fmt.Errorf("scan pipeline row: %w", err)
		}
		p.JobType = domain.JobType(jt)
		p.Status = domain.RunStatus(st)
		p.TriggeredBy = domain.TriggeredBy(tb)
		list = append(list, &p)
	}
	return list, rows.Err()
}

// ── scan helper ───────────────────────────────────────────────────────────────

func scanPipelineRun(row *sql.Row) (*domain.PipelineRun, error) {
	var p domain.PipelineRun
	var jt, st, tb string
	err := row.Scan(
		&p.ID, &jt, &st, &tb, &p.CurrentStep,
		&p.ArticlesCollected, &p.ArticlesSelected, &p.ErrorMessage,
		&p.StartedAt, &p.FinishedAt,
	)
	if err == sql.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan pipeline run: %w", err)
	}
	p.JobType = domain.JobType(jt)
	p.Status = domain.RunStatus(st)
	p.TriggeredBy = domain.TriggeredBy(tb)
	return &p, nil
}
