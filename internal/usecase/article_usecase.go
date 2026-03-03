package usecase

import (
	"context"

	"ai-news/internal/domain"
	"ai-news/internal/repository"
)

// ArticleUsecase handles article retrieval for the web UI.
type ArticleUsecase struct {
	articleRepo *repository.ArticleRepo
}

// NewArticleUsecase creates an ArticleUsecase.
func NewArticleUsecase(a *repository.ArticleRepo) *ArticleUsecase {
	return &ArticleUsecase{articleRepo: a}
}

// ListByCategory returns the most recent articles for a category (newest first).
func (uc *ArticleUsecase) ListByCategory(ctx context.Context, category string, limit int) ([]*domain.Article, error) {
	if limit <= 0 {
		limit = 50
	}
	return uc.articleRepo.ListByCategory(ctx, category, limit)
}

// GetByID returns a single article or domain.ErrNotFound.
func (uc *ArticleUsecase) GetByID(ctx context.Context, id int64) (*domain.Article, error) {
	return uc.articleRepo.GetByID(ctx, id)
}
