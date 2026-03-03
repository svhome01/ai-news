package usecase

import (
	"context"
	"fmt"

	"ai-news/internal/domain"
	"ai-news/internal/infra/voicevox"
	"ai-news/internal/repository"
)

// CategoryUsecase handles category_settings CRUD and VOICEVOX speaker listing.
type CategoryUsecase struct {
	categoryRepo   *repository.CategoryRepo
	voicevoxClient *voicevox.Client
}

// NewCategoryUsecase creates a CategoryUsecase.
func NewCategoryUsecase(r *repository.CategoryRepo, vc *voicevox.Client) *CategoryUsecase {
	return &CategoryUsecase{categoryRepo: r, voicevoxClient: vc}
}

// Create validates and creates a new category.
// Returns domain.ErrReservedWord if the name is reserved, domain.ErrInvalidInput if the
// name is empty or otherwise invalid.
func (uc *CategoryUsecase) Create(ctx context.Context, c *domain.CategorySettings) error {
	if c.Category == "" {
		return fmt.Errorf("%w: category name is required", domain.ErrInvalidInput)
	}
	if domain.ReservedCategories[c.Category] {
		return fmt.Errorf("%w: category %q is reserved", domain.ErrReservedWord, c.Category)
	}
	if err := uc.categoryRepo.Create(ctx, c); err != nil {
		return fmt.Errorf("category create: %w", err)
	}
	return nil
}

// GetByName returns a category or domain.ErrNotFound.
func (uc *CategoryUsecase) GetByName(ctx context.Context, name string) (*domain.CategorySettings, error) {
	return uc.categoryRepo.GetByName(ctx, name)
}

// List returns all categories.
func (uc *CategoryUsecase) List(ctx context.Context) ([]*domain.CategorySettings, error) {
	return uc.categoryRepo.List(ctx)
}

// ListEnabled returns all enabled categories ordered by sort_order.
func (uc *CategoryUsecase) ListEnabled(ctx context.Context) ([]*domain.CategorySettings, error) {
	return uc.categoryRepo.ListEnabled(ctx)
}

// Update overwrites an existing category's mutable fields.
func (uc *CategoryUsecase) Update(ctx context.Context, c *domain.CategorySettings) error {
	return uc.categoryRepo.Update(ctx, c)
}

// Delete removes a category by name.
func (uc *CategoryUsecase) Delete(ctx context.Context, name string) error {
	if domain.ReservedCategories[name] {
		return fmt.Errorf("%w: category %q is reserved", domain.ErrReservedWord, name)
	}
	return uc.categoryRepo.Delete(ctx, name)
}

// Speakers returns the list of available VOICEVOX speakers.
func (uc *CategoryUsecase) Speakers(ctx context.Context) ([]voicevox.Speaker, error) {
	if uc.voicevoxClient == nil {
		return nil, fmt.Errorf("voicevox client not configured")
	}
	return uc.voicevoxClient.Speakers(ctx)
}
