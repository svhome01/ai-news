package usecase

import (
	"context"

	"ai-news/internal/domain"
	"ai-news/internal/repository"
)

// SettingsUsecase handles reading and updating global application settings.
type SettingsUsecase struct {
	settingsRepo *repository.SettingsRepo
}

// NewSettingsUsecase creates a SettingsUsecase.
func NewSettingsUsecase(r *repository.SettingsRepo) *SettingsUsecase {
	return &SettingsUsecase{settingsRepo: r}
}

// Get returns the current application settings.
func (uc *SettingsUsecase) Get(ctx context.Context) (*domain.AppSettings, error) {
	return uc.settingsRepo.Get(ctx)
}

// Update persists new settings values.
func (uc *SettingsUsecase) Update(ctx context.Context, s *domain.AppSettings) error {
	return uc.settingsRepo.Update(ctx, s)
}
