package usecase

import (
	"context"
	"fmt"
	"log"
	"strings"

	"ai-news/internal/infra/storage"
	"ai-news/internal/infra/thumbnail"
	"ai-news/internal/repository"
)

// CleanupUsecase deletes broadcasts, MP3 files, and articles older than retention_days.
type CleanupUsecase struct {
	settingsRepo  *repository.SettingsRepo
	broadcastRepo *repository.BroadcastRepo
	articleRepo   *repository.ArticleRepo
	musicStore    *storage.MusicStore
	thumbStore    *thumbnail.Store
}

// NewCleanupUsecase creates a CleanupUsecase.
func NewCleanupUsecase(
	sr *repository.SettingsRepo,
	br *repository.BroadcastRepo,
	ar *repository.ArticleRepo,
	ms *storage.MusicStore,
	ts *thumbnail.Store,
) *CleanupUsecase {
	return &CleanupUsecase{
		settingsRepo:  sr,
		broadcastRepo: br,
		articleRepo:   ar,
		musicStore:    ms,
		thumbStore:    ts,
	}
}

// Run performs the full cleanup sequence:
//  1. Find and delete broadcasts older than retention_days (ON DELETE SET NULL clears articles.broadcast_id).
//  2. Delete each broadcast's MP3 from SMB.
//  3. NullifyRawContent for processed articles (DB size reduction).
//  4. Delete thumbnail files for old articles.
//  5. Delete old article rows.
func (uc *CleanupUsecase) Run(ctx context.Context) error {
	settings, err := uc.settingsRepo.Get(ctx)
	if err != nil {
		return fmt.Errorf("cleanup: get settings: %w", err)
	}
	days := settings.RetentionDays

	// 1. Find old broadcasts.
	broadcasts, err := uc.broadcastRepo.ListOlderThan(ctx, days)
	if err != nil {
		return fmt.Errorf("cleanup: list broadcasts: %w", err)
	}

	// 2. Delete broadcasts from DB (triggers ON DELETE SET NULL on articles.broadcast_id).
	if len(broadcasts) > 0 {
		ids := make([]int64, len(broadcasts))
		for i, b := range broadcasts {
			ids[i] = b.ID
		}
		if err := uc.broadcastRepo.DeleteByIDs(ctx, ids); err != nil {
			return fmt.Errorf("cleanup: delete broadcasts: %w", err)
		}
		log.Printf("cleanup: deleted %d broadcast records", len(broadcasts))

		// 3. Delete MP3 files from SMB (non-fatal per-file).
		for _, b := range broadcasts {
			if b.FilePath == "" {
				continue
			}
			if err := uc.musicStore.DeleteMP3(b.FilePath); err != nil {
				if !isNotFoundError(err) {
					log.Printf("cleanup: delete MP3 %s (non-fatal): %v", b.FilePath, err)
				}
			}
		}
	}

	// 4. NullifyRawContent for processed articles (broadcast_id IS NOT NULL means processed).
	// We pass 0 days — this NULLs raw_content for all articles where retention days have passed.
	if err := uc.articleRepo.NullifyRawContent(ctx, days); err != nil {
		log.Printf("cleanup: nullify raw_content (non-fatal): %v", err)
	}

	// 5. Find thumbnails for old articles before deleting rows.
	refs, err := uc.articleRepo.FindThumbnailsOlderThan(ctx, days)
	if err != nil {
		log.Printf("cleanup: find thumbnails (non-fatal): %v", err)
	}
	for _, ref := range refs {
		if err := uc.thumbStore.Delete(ref.ArticleID); err != nil {
			log.Printf("cleanup: delete thumbnail article %d (non-fatal): %v", ref.ArticleID, err)
		}
	}

	// 6. Delete old article rows.
	if err := uc.articleRepo.DeleteOlderThan(ctx, days); err != nil {
		return fmt.Errorf("cleanup: delete articles: %w", err)
	}

	log.Printf("cleanup: completed (retention_days=%d)", days)
	return nil
}

// isNotFoundError returns true for SMB "file not found" style errors.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "STATUS_OBJECT_NAME_NOT_FOUND") ||
		strings.Contains(msg, "STATUS_NO_SUCH_FILE") ||
		strings.Contains(msg, "not found")
}
