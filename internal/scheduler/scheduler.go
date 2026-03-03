package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/robfig/cron/v3"

	"ai-news/internal/repository"
)

const (
	scrapeTimeout  = 10 * time.Minute
	generateTimeout = 30 * time.Minute
	cleanupTimeout  = 5 * time.Minute

	// cleanupHourJST is the hour in JST at which the cleanup job runs.
	// We store all schedules in JST (TZ set in the container), so robfig/cron
	// will fire at the wall-clock time configured by the TZ env var.
	cleanupHourJST = 3
)

// ScrapeFunc is the callback signature for the scrape pipeline job.
type ScrapeFunc func(ctx context.Context, triggeredBy string) error

// GenerateFunc is the callback signature for the generate pipeline job.
type GenerateFunc func(ctx context.Context, triggeredBy string) error

// CleanupFunc is the callback signature for the cleanup job.
type CleanupFunc func(ctx context.Context) error

// Scheduler wraps robfig/cron and loads scrape/generate schedules from the DB.
// Each enabled schedule row registers one independent cron entry.
// A fixed cleanup job runs daily at 03:00 (local time per TZ env var).
type Scheduler struct {
	c            *cron.Cron
	scheduleRepo *repository.ScheduleRepo
	scrapeFunc   ScrapeFunc
	generateFunc GenerateFunc
	cleanupFunc  CleanupFunc
	entryIDs     []cron.EntryID
}

// New creates a Scheduler.  Call Start() to begin processing.
func New(
	sr *repository.ScheduleRepo,
	scrape ScrapeFunc,
	generate GenerateFunc,
	cleanup CleanupFunc,
) *Scheduler {
	c := cron.New(
		cron.WithSeconds(),
		cron.WithChain(
			cron.SkipIfStillRunning(cron.DefaultLogger),
			cron.Recover(cron.DefaultLogger),
		),
	)
	return &Scheduler{
		c:            c,
		scheduleRepo: sr,
		scrapeFunc:   scrape,
		generateFunc: generate,
		cleanupFunc:  cleanup,
	}
}

// Start loads schedules from the DB, registers cron entries, and starts the scheduler.
func (s *Scheduler) Start(ctx context.Context) error {
	if err := s.loadAll(ctx); err != nil {
		return fmt.Errorf("scheduler start: %w", err)
	}
	s.c.Start()
	log.Printf("scheduler: started with %d entries (including cleanup)", len(s.entryIDs))
	return nil
}

// ReloadAll removes all dynamic entries, re-reads the DB, and re-registers them.
// The cleanup entry is re-added as well.  Call this after any schedule CRUD.
func (s *Scheduler) ReloadAll(ctx context.Context) error {
	for _, id := range s.entryIDs {
		s.c.Remove(id)
	}
	s.entryIDs = nil

	if err := s.loadAll(ctx); err != nil {
		return fmt.Errorf("scheduler reload: %w", err)
	}
	log.Printf("scheduler: reloaded %d entries", len(s.entryIDs))
	return nil
}

// Stop signals the scheduler to cease firing new jobs and waits for running jobs
// to finish.  The returned context is done when all active jobs have completed.
func (s *Scheduler) Stop() context.Context {
	return s.c.Stop()
}

// loadAll registers scrape and generate entries from the DB and the fixed cleanup entry.
func (s *Scheduler) loadAll(ctx context.Context) error {
	scrapes, err := s.scheduleRepo.ListByType(ctx, "scrape")
	if err != nil {
		return err
	}
	for _, sched := range scrapes {
		if !sched.Enabled {
			continue
		}
		spec := fmt.Sprintf("0 %d %d * * *", sched.Minute, sched.Hour)
		id, err := s.c.AddFunc(spec, s.makeScrapeJob())
		if err != nil {
			log.Printf("scheduler: add scrape entry %v: %v", spec, err)
			continue
		}
		s.entryIDs = append(s.entryIDs, id)
	}

	generates, err := s.scheduleRepo.ListByType(ctx, "generate")
	if err != nil {
		return err
	}
	for _, sched := range generates {
		if !sched.Enabled {
			continue
		}
		spec := fmt.Sprintf("0 %d %d * * *", sched.Minute, sched.Hour)
		id, err := s.c.AddFunc(spec, s.makeGenerateJob())
		if err != nil {
			log.Printf("scheduler: add generate entry %v: %v", spec, err)
			continue
		}
		s.entryIDs = append(s.entryIDs, id)
	}

	// Fixed cleanup at 03:00 every day (not in the schedules table).
	cleanupSpec := fmt.Sprintf("0 0 %d * * *", cleanupHourJST)
	id, err := s.c.AddFunc(cleanupSpec, s.makeCleanupJob())
	if err != nil {
		return fmt.Errorf("add cleanup cron: %w", err)
	}
	s.entryIDs = append(s.entryIDs, id)

	return nil
}

// makeScrapeJob returns a cron function for the scrape pipeline.
func (s *Scheduler) makeScrapeJob() func() {
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), scrapeTimeout)
		defer cancel()
		if err := s.scrapeFunc(ctx, "cron"); err != nil {
			log.Printf("scheduler: scrape job error: %v", err)
		}
	}
}

// makeGenerateJob returns a cron function for the generate pipeline.
func (s *Scheduler) makeGenerateJob() func() {
	return func() {
		if s.generateFunc == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), generateTimeout)
		defer cancel()
		if err := s.generateFunc(ctx, "cron"); err != nil {
			log.Printf("scheduler: generate job error: %v", err)
		}
	}
}

// makeCleanupJob returns a cron function for the daily cleanup.
func (s *Scheduler) makeCleanupJob() func() {
	return func() {
		if s.cleanupFunc == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		if err := s.cleanupFunc(ctx); err != nil {
			log.Printf("scheduler: cleanup job error: %v", err)
		}
	}
}
