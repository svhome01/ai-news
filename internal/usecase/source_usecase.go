package usecase

import (
	"context"
	"fmt"

	"ai-news/internal/domain"
	"ai-news/internal/repository"
)

// SourceUsecase handles business logic for news source management.
type SourceUsecase struct {
	sourceRepo   *repository.SourceRepo
	categoryRepo *repository.CategoryRepo
}

// NewSourceUsecase creates a SourceUsecase.
func NewSourceUsecase(s *repository.SourceRepo, c *repository.CategoryRepo) *SourceUsecase {
	return &SourceUsecase{sourceRepo: s, categoryRepo: c}
}

// Create validates and persists a new source.
func (uc *SourceUsecase) Create(ctx context.Context, s *domain.Source) (int64, error) {
	if err := uc.validateCategory(ctx, s.Category); err != nil {
		return 0, err
	}
	id, err := uc.sourceRepo.Create(ctx, s)
	if err != nil {
		return 0, fmt.Errorf("source create: %w", err)
	}
	return id, nil
}

// GetByID returns a source or domain.ErrNotFound.
func (uc *SourceUsecase) GetByID(ctx context.Context, id int64) (*domain.Source, error) {
	return uc.sourceRepo.GetByID(ctx, id)
}

// List returns all sources.
func (uc *SourceUsecase) List(ctx context.Context) ([]*domain.Source, error) {
	return uc.sourceRepo.List(ctx)
}

// Update validates and overwrites a source.
func (uc *SourceUsecase) Update(ctx context.Context, s *domain.Source) error {
	if err := uc.validateCategory(ctx, s.Category); err != nil {
		return err
	}
	return uc.sourceRepo.Update(ctx, s)
}

// Delete removes a source by ID.
func (uc *SourceUsecase) Delete(ctx context.Context, id int64) error {
	return uc.sourceRepo.Delete(ctx, id)
}

// validateCategory rejects unknown category names.
func (uc *SourceUsecase) validateCategory(ctx context.Context, category string) error {
	_, err := uc.categoryRepo.GetByName(ctx, category)
	if err == domain.ErrNotFound {
		return fmt.Errorf("%w: category %q does not exist", domain.ErrInvalidInput, category)
	}
	return err
}
