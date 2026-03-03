package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ai-news/internal/config"
	"ai-news/internal/infra/navidrome"
	"ai-news/internal/infra/scraper"
	"ai-news/internal/infra/storage"
	"ai-news/internal/infra/thumbnail"
	"ai-news/internal/infra/voicevox"
	"ai-news/internal/repository"
	"ai-news/internal/scheduler"
	"ai-news/internal/usecase"
)

func main() {
	cfg := config.Load()

	// ── Database ────────────────────────────────────────────────────────────
	db, err := repository.OpenDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// ── Repositories ────────────────────────────────────────────────────────
	articleRepo   := repository.NewArticleRepo(db)
	sourceRepo    := repository.NewSourceRepo(db)
	broadcastRepo := repository.NewBroadcastRepo(db)
	pipelineRepo  := repository.NewPipelineRepo(db)
	settingsRepo  := repository.NewSettingsRepo(db)
	categoryRepo  := repository.NewCategoryRepo(db)
	scheduleRepo  := repository.NewScheduleRepo(db)

	// ── Infrastructure ──────────────────────────────────────────────────────
	thumbStore, err := thumbnail.New()
	if err != nil {
		log.Fatalf("thumbnail store: %v", err)
	}

	fetcherFactory := scraper.NewFactory(cfg.PlaywrightCDPEndpoint)

	voicevoxClient := voicevox.New(cfg.VOICEVOXUrl)

	musicStore := storage.New(cfg.SMBHost, cfg.SMBUser, cfg.SMBPass, cfg.SMBShare, cfg.SMBMusicPath)

	var naviClient *navidrome.Client
	if cfg.NavidromeURL != "" && cfg.NavidromeUser != "" {
		naviClient = navidrome.New(cfg.NavidromeURL, cfg.NavidromeUser, cfg.NavidromePass)
	}

	// ── Usecases ────────────────────────────────────────────────────────────
	sourceUC   := usecase.NewSourceUsecase(sourceRepo, categoryRepo)
	articleUC  := usecase.NewArticleUsecase(articleRepo)
	scrapeUC   := usecase.NewScrapeUsecase(pipelineRepo, sourceRepo, articleRepo, thumbStore, fetcherFactory)
	settingsUC := usecase.NewSettingsUsecase(settingsRepo)
	categoryUC := usecase.NewCategoryUsecase(categoryRepo, voicevoxClient)
	playbackUC := usecase.NewPlaybackUsecase(categoryRepo, broadcastRepo, cfg.AppBaseURL)

	generateUC := usecase.NewGenerateUsecase(
		pipelineRepo, settingsRepo, categoryRepo, articleRepo, broadcastRepo,
		cfg.GeminiAPIKey, cfg.MaxGeminiConcurrency,
		voicevoxClient, musicStore, naviClient,
		cfg.AppBaseURL,
	)

	cleanupUC := usecase.NewCleanupUsecase(settingsRepo, broadcastRepo, articleRepo, musicStore, thumbStore)

	// Keep unused references alive until handlers are wired in Step 4.
	_ = sourceUC
	_ = articleUC
	_ = settingsUC
	_ = categoryUC
	_ = playbackUC

	// ── Scheduler ───────────────────────────────────────────────────────────
	sched := scheduler.New(
		scheduleRepo,
		scrapeUC.Run,
		generateUC.Run,
		cleanupUC.Run,
	)

	startCtx := context.Background()
	if err := sched.Start(startCtx); err != nil {
		log.Fatalf("scheduler start: %v", err)
	}

	// ── HTTP router ─────────────────────────────────────────────────────────
	mux := http.NewServeMux()
	registerRoutes(mux)

	// ── Server ──────────────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("ai-news listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down...")

	// Stop the scheduler and wait for any running jobs to finish.
	schedDone := sched.Stop()
	<-schedDone.Done()

	httpCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(httpCtx); err != nil {
		log.Fatalf("http shutdown: %v", err)
	}
	log.Println("stopped")
}

// registerRoutes wires all HTTP handlers to the mux.
// Handler and usecase packages are added in Step 4.
func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /api/health", handleHealthz)
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintln(w, "ok")
}
