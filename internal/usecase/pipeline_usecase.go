package usecase

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"ai-news/internal/domain"
	"ai-news/internal/infra/scraper"
	"ai-news/internal/infra/thumbnail"
	"ai-news/internal/repository"
)

const scrapeWorkers = 5

// ScrapeUsecase implements Stage 1 of the pipeline: crawl sources and save articles.
// It uses an atomic.Bool to prevent concurrent runs.
type ScrapeUsecase struct {
	running      atomic.Bool
	pipelineRepo *repository.PipelineRepo
	sourceRepo   *repository.SourceRepo
	articleRepo  *repository.ArticleRepo
	thumbStore   *thumbnail.Store
	factory      scraper.Factory
}

// NewScrapeUsecase creates a ScrapeUsecase.
func NewScrapeUsecase(
	pr *repository.PipelineRepo,
	sr *repository.SourceRepo,
	ar *repository.ArticleRepo,
	ts *thumbnail.Store,
	f scraper.Factory,
) *ScrapeUsecase {
	return &ScrapeUsecase{
		pipelineRepo: pr,
		sourceRepo:   sr,
		articleRepo:  ar,
		thumbStore:   ts,
		factory:      f,
	}
}

// IsRunning reports whether a scrape job is currently in progress.
func (uc *ScrapeUsecase) IsRunning() bool {
	return uc.running.Load()
}

// Run executes Stage 1 of the pipeline. triggeredBy is "cron", "api", or "ui".
// If a scrape is already running, ErrPipelineActive is returned immediately.
func (uc *ScrapeUsecase) Run(ctx context.Context, triggeredBy string) error {
	if !uc.running.CompareAndSwap(false, true) {
		return domain.ErrPipelineActive
	}
	defer uc.running.Store(false)

	runID, err := uc.pipelineRepo.Create(ctx, &domain.PipelineRun{
		JobType:     domain.JobTypeScrape,
		Status:      domain.RunStatusRunning,
		TriggeredBy: domain.TriggeredBy(triggeredBy),
	})
	if err != nil {
		return fmt.Errorf("create pipeline run: %w", err)
	}

	count, scrapeErr := uc.scrapeAllSources(ctx, runID)

	if err := uc.pipelineRepo.UpdateCollected(ctx, runID, count); err != nil {
		log.Printf("scrape: update collected count: %v", err)
	}

	status := domain.RunStatusCompleted
	var errMsg *string
	if scrapeErr != nil {
		status = domain.RunStatusFailed
		msg := scrapeErr.Error()
		errMsg = &msg
	}
	if err := uc.pipelineRepo.Finish(ctx, runID, status, errMsg); err != nil {
		log.Printf("scrape: finish pipeline run: %v", err)
	}

	return scrapeErr
}

// scrapeAllSources fetches articles from all enabled sources using up to scrapeWorkers
// concurrent workers. Individual source failures are logged and counted but do not abort
// the overall run. Returns the total number of new articles saved.
func (uc *ScrapeUsecase) scrapeAllSources(ctx context.Context, runID int64) (int, error) {
	sources, err := uc.sourceRepo.ListEnabled(ctx)
	if err != nil {
		return 0, fmt.Errorf("list enabled sources: %w", err)
	}
	if len(sources) == 0 {
		return 0, nil
	}

	type result struct {
		count int
		err   error
	}

	sem := make(chan struct{}, scrapeWorkers)
	resultCh := make(chan result, len(sources))

	var wg sync.WaitGroup
	for _, src := range sources {
		src := src // capture loop variable
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			n, err := uc.fetchAndSave(ctx, runID, src)
			resultCh <- result{count: n, err: err}
		}()
	}
	wg.Wait()
	close(resultCh)

	total := 0
	for r := range resultCh {
		if r.err != nil {
			log.Printf("scrape source error (non-fatal): %v", r.err)
		}
		total += r.count
	}
	return total, nil
}

// fetchAndSave fetches articles from one source and saves new ones to the DB.
// Returns the count of newly inserted articles.
func (uc *ScrapeUsecase) fetchAndSave(ctx context.Context, runID int64, src *domain.Source) (int, error) {
	fetchFn := uc.factory(src.FetchMethod)
	items, err := fetchFn.Fetch(ctx, src)
	if err != nil {
		return 0, fmt.Errorf("fetch source %d (%s): %w", src.ID, src.URL, err)
	}

	count := 0
	for _, item := range items {
		article := &domain.Article{
			SourceID:      src.ID,
			PipelineRunID: &runID,
			Title:         item.Title,
			URL:           item.URL,
			Category:      src.Category,
		}
		if item.RawContent != "" {
			article.RawContent = &item.RawContent
		}

		newID, err := uc.articleRepo.Save(ctx, article)
		if err != nil {
			log.Printf("scrape: save article %q: %v", item.URL, err)
			continue
		}
		if newID == 0 {
			// URL already existed; skip thumbnail (already stored previously).
			continue
		}
		count++

		// Download and save thumbnail for new articles (non-fatal on failure).
		if item.RemoteThumb != "" && uc.thumbStore != nil {
			localPath, err := uc.thumbStore.DownloadAndSave(ctx, newID, item.RemoteThumb)
			if err != nil {
				log.Printf("scrape: thumbnail for article %d: %v", newID, err)
			} else if localPath != "" {
				if err := uc.articleRepo.SetThumbnailURL(ctx, newID, localPath); err != nil {
					log.Printf("scrape: set thumbnail url for article %d: %v", newID, err)
				}
			}
		}
	}
	return count, nil
}
