package usecase

import (
	"context"
	"fmt"

	"ai-news/internal/domain"
	"ai-news/internal/repository"
)

// PlaybackResult holds the data returned to callers (Home Assistant, Web UI) for playback.
type PlaybackResult struct {
	BroadcastID int64
	Title       string
	MediaURL    string // HTTP URL for media_content_id
	DurationSec int
}

// PlaybackUsecase retrieves playback metadata for a category.
type PlaybackUsecase struct {
	categoryRepo  *repository.CategoryRepo
	broadcastRepo *repository.BroadcastRepo
	appBaseURL    string
}

// NewPlaybackUsecase creates a PlaybackUsecase.
func NewPlaybackUsecase(cr *repository.CategoryRepo, br *repository.BroadcastRepo, appBaseURL string) *PlaybackUsecase {
	return &PlaybackUsecase{categoryRepo: cr, broadcastRepo: br, appBaseURL: appBaseURL}
}

// GetLatest returns playback metadata for the latest broadcast of a category.
// Returns domain.ErrNotFound if the category does not exist or has no broadcasts.
func (uc *PlaybackUsecase) GetLatest(ctx context.Context, category string) (*PlaybackResult, error) {
	if _, err := uc.categoryRepo.GetByName(ctx, category); err != nil {
		return nil, fmt.Errorf("category %q: %w", category, err)
	}

	bc, err := uc.broadcastRepo.GetLatest(ctx, category)
	if err != nil {
		return nil, err
	}

	mediaURL := fmt.Sprintf("%s/media/%s/latest", uc.appBaseURL, category)
	dur := 0
	if bc.DurationSec != nil {
		dur = *bc.DurationSec
	}

	return &PlaybackResult{
		BroadcastID: bc.ID,
		Title:       bc.Title,
		MediaURL:    mediaURL,
		DurationSec: dur,
	}, nil
}

// GetByID returns playback metadata for a specific broadcast.
func (uc *PlaybackUsecase) GetByID(ctx context.Context, id int64) (*PlaybackResult, error) {
	bc, err := uc.broadcastRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	mediaURL := fmt.Sprintf("%s/media/%s/%d", uc.appBaseURL, bc.Category, bc.ID)
	dur := 0
	if bc.DurationSec != nil {
		dur = *bc.DurationSec
	}

	return &PlaybackResult{
		BroadcastID: bc.ID,
		Title:       bc.Title,
		MediaURL:    mediaURL,
		DurationSec: dur,
	}, nil
}

// GetBroadcast returns the raw Broadcast domain object for a category (latest).
func (uc *PlaybackUsecase) GetBroadcast(ctx context.Context, category string) (*domain.Broadcast, error) {
	return uc.broadcastRepo.GetLatest(ctx, category)
}
