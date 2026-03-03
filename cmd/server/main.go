package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"ai-news/internal/config"
	"ai-news/internal/handler"
	apih "ai-news/internal/handler/api"
	webh "ai-news/internal/handler/web"
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
	musicStore     := storage.New(cfg.SMBHost, cfg.SMBUser, cfg.SMBPass, cfg.SMBShare, cfg.SMBMusicPath)

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
	scheduleUC := usecase.NewScheduleUsecase(scheduleRepo, nil) // scheduler set below

	generateUC := usecase.NewGenerateUsecase(
		pipelineRepo, settingsRepo, categoryRepo, articleRepo, broadcastRepo,
		cfg.GeminiAPIKey, cfg.MaxGeminiConcurrency,
		voicevoxClient, musicStore, naviClient,
		cfg.AppBaseURL,
	)

	cleanupUC := usecase.NewCleanupUsecase(settingsRepo, broadcastRepo, articleRepo, musicStore, thumbStore)

	// ── Scheduler ───────────────────────────────────────────────────────────
	sched := scheduler.New(
		scheduleRepo,
		scrapeUC.Run,
		generateUC.Run,
		cleanupUC.Run,
	)
	// Wire scheduler back into scheduleUC so CRUD changes trigger reload.
	scheduleUC = usecase.NewScheduleUsecase(scheduleRepo, sched)

	startCtx := context.Background()
	if err := sched.Start(startCtx); err != nil {
		log.Fatalf("scheduler start: %v", err)
	}

	// ── Templates ───────────────────────────────────────────────────────────
	tmplDir := "templates"
	funcMap := template.FuncMap{}

	parseFull := func(files ...string) *template.Template {
		paths := make([]string, len(files))
		for i, f := range files {
			paths[i] = filepath.Join(tmplDir, f)
		}
		return template.Must(template.New("").Funcs(funcMap).ParseFiles(paths...))
	}
	parseStandalone := func(file string) *template.Template {
		return template.Must(template.New("").Funcs(funcMap).ParseFiles(filepath.Join(tmplDir, file)))
	}

	dashTmpl       := parseFull("layout.html", "dashboard.html", "partials/article_list.html")
	articleListTmpl := parseStandalone("partials/article_list.html")
	sourcesTmpl    := parseFull("layout.html", "sources.html", "partials/source_row.html")
	sourceRowTmpl  := parseStandalone("partials/source_row.html")
	settingsTmpl   := parseFull("layout.html", "settings.html")
	catRowTmpl     := parseStandalone("settings.html") // for category-row define
	systemTmpl     := parseFull("layout.html", "system.html", "partials/pipeline_status.html")
	pipelineStatusTmpl := parseStandalone("partials/pipeline_status.html")

	// ── Handlers ────────────────────────────────────────────────────────────
	dashH     := webh.NewDashboardHandler(dashTmpl, articleListTmpl, articleUC, categoryUC, broadcastRepo)
	sourcesH  := webh.NewSourcesHandler(sourcesTmpl, sourceRowTmpl, sourceUC, categoryUC)
	settingsH := webh.NewSettingsHandler(settingsTmpl, catRowTmpl, settingsUC, categoryUC)
	systemH   := webh.NewSystemHandler(systemTmpl, pipelineStatusTmpl, scrapeUC, generateUC, cleanupUC, pipelineRepo)

	playH      := apih.NewPlayHandler(playbackUC)
	broadcastH := apih.NewBroadcastHandler(broadcastRepo)
	mediaH     := apih.NewMediaHandler(broadcastRepo, musicStore)
	pipelineH  := apih.NewPipelineHandler(scrapeUC, generateUC, pipelineRepo)
	voicevoxH  := apih.NewVoicevoxHandler(categoryUC)

	_ = scheduleUC // available for future schedule API handlers

	// ── HTTP router ─────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /api/health", handleHealthz)

	// Web UI
	mux.HandleFunc("GET /{$}", dashH.Page)
	mux.HandleFunc("GET /ui/articles", dashH.ArticleList)

	mux.HandleFunc("GET /ui/sources", sourcesH.Page)
	mux.HandleFunc("POST /ui/sources", sourcesH.Create)
	mux.HandleFunc("GET /ui/sources/{id}", sourcesH.GetView)
	mux.HandleFunc("GET /ui/sources/{id}/edit", sourcesH.GetEdit)
	mux.HandleFunc("PUT /ui/sources/{id}", sourcesH.Update)
	mux.HandleFunc("DELETE /ui/sources/{id}", sourcesH.Delete)

	mux.HandleFunc("GET /ui/settings", settingsH.Page)
	mux.HandleFunc("POST /ui/settings", settingsH.Update)
	mux.HandleFunc("POST /ui/settings/categories", settingsH.CreateCategory)
	mux.HandleFunc("DELETE /ui/settings/categories/{name}", settingsH.DeleteCategory)

	mux.HandleFunc("GET /ui/system", systemH.Page)
	mux.HandleFunc("POST /ui/system/trigger", systemH.Trigger)
	mux.HandleFunc("POST /ui/system/cleanup", systemH.Cleanup)
	mux.HandleFunc("GET /ui/system/pipeline/{id}/status", systemH.PipelineStatus)

	// REST API
	mux.HandleFunc("POST /api/play/stop", playH.Stop)
	mux.HandleFunc("POST /api/play/{category}", playH.Play)
	mux.HandleFunc("GET /api/broadcasts", broadcastH.List)
	mux.HandleFunc("GET /media/{category}/latest", mediaH.ServeLatest)
	mux.HandleFunc("GET /media/{category}/{id}", mediaH.ServeByID)
	mux.HandleFunc("POST /api/pipeline/run", pipelineH.Trigger)
	mux.HandleFunc("GET /api/pipeline/{id}", pipelineH.Status)
	mux.HandleFunc("GET /api/voicevox/speakers", voicevoxH.Speakers)

	// Apply middleware
	root := handler.Chain(mux,
		handler.Recover,
		handler.Logger,
		handler.RequestID,
	)

	// ── Server ──────────────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      root,
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

	schedDone := sched.Stop()
	<-schedDone.Done()

	httpCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(httpCtx); err != nil {
		log.Fatalf("http shutdown: %v", err)
	}
	log.Println("stopped")
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("ok\n"))
}
