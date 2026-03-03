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
	"ai-news/internal/repository"
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
	// Assigned to blank identifiers until usecases and handlers are wired in Steps 2–4.
	_ = repository.NewArticleRepo(db)
	_ = repository.NewSourceRepo(db)
	_ = repository.NewBroadcastRepo(db)
	_ = repository.NewPipelineRepo(db)
	_ = repository.NewSettingsRepo(db)
	_ = repository.NewCategoryRepo(db)
	_ = repository.NewScheduleRepo(db)

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

	// Graceful shutdown: drain in-flight requests on SIGINT/SIGTERM.
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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("stopped")
}

// registerRoutes wires all HTTP handlers to the mux.
// Handler and usecase packages are added here in Steps 2–4.
func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /api/health", handleHealthz)
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintln(w, "ok")
}
