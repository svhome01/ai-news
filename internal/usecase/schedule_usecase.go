package usecase

import (
	"context"
	"fmt"

	"ai-news/internal/domain"
	"ai-news/internal/repository"
	"ai-news/internal/scheduler"
)

// ScheduleUsecase handles schedules CRUD and triggers Scheduler reload after changes.
type ScheduleUsecase struct {
	scheduleRepo *repository.ScheduleRepo
	scheduler    *scheduler.Scheduler
}

// NewScheduleUsecase creates a ScheduleUsecase.
func NewScheduleUsecase(r *repository.ScheduleRepo, s *scheduler.Scheduler) *ScheduleUsecase {
	return &ScheduleUsecase{scheduleRepo: r, scheduler: s}
}

// Create adds a new schedule entry and reloads the cron scheduler.
func (uc *ScheduleUsecase) Create(ctx context.Context, s *domain.Schedule) (int64, error) {
	id, err := uc.scheduleRepo.Create(ctx, s)
	if err != nil {
		return 0, fmt.Errorf("schedule create: %w", err)
	}
	_ = uc.scheduler.ReloadAll(ctx)
	return id, nil
}

// GetByID returns a schedule or domain.ErrNotFound.
func (uc *ScheduleUsecase) GetByID(ctx context.Context, id int64) (*domain.Schedule, error) {
	return uc.scheduleRepo.GetByID(ctx, id)
}

// ListByType returns schedules filtered by type ("scrape" or "generate").
func (uc *ScheduleUsecase) ListByType(ctx context.Context, schedType string) ([]*domain.Schedule, error) {
	return uc.scheduleRepo.ListByType(ctx, schedType)
}

// List returns all schedules.
func (uc *ScheduleUsecase) List(ctx context.Context) ([]*domain.Schedule, error) {
	return uc.scheduleRepo.List(ctx)
}

// SetEnabled toggles the enabled flag and reloads the cron scheduler.
func (uc *ScheduleUsecase) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	s, err := uc.scheduleRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	s.Enabled = enabled
	if err := uc.scheduleRepo.Update(ctx, s); err != nil {
		return fmt.Errorf("schedule update: %w", err)
	}
	return uc.scheduler.ReloadAll(ctx)
}

// Delete removes a schedule and reloads the cron scheduler.
func (uc *ScheduleUsecase) Delete(ctx context.Context, id int64) error {
	if err := uc.scheduleRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("schedule delete: %w", err)
	}
	return uc.scheduler.ReloadAll(ctx)
}
