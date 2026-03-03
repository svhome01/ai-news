package web

import (
	"context"
	"html/template"
	"net/http"

	"ai-news/internal/domain"
	"ai-news/internal/handler"
	"ai-news/internal/repository"
	"ai-news/internal/usecase"
)

// SystemHandler serves the system control page.
type SystemHandler struct {
	pageTmpl     *template.Template // layout.html + system.html + pipeline_status.html
	statusTmpl   *template.Template // pipeline_status.html (standalone, for HTMX)
	scrapeUC     *usecase.ScrapeUsecase
	generateUC   *usecase.GenerateUsecase
	cleanupUC    *usecase.CleanupUsecase
	pipelineRepo *repository.PipelineRepo
}

// NewSystemHandler creates a SystemHandler.
func NewSystemHandler(
	pageTmpl, statusTmpl *template.Template,
	su *usecase.ScrapeUsecase,
	gu *usecase.GenerateUsecase,
	cu *usecase.CleanupUsecase,
	pr *repository.PipelineRepo,
) *SystemHandler {
	return &SystemHandler{
		pageTmpl:     pageTmpl,
		statusTmpl:   statusTmpl,
		scrapeUC:     su,
		generateUC:   gu,
		cleanupUC:    cu,
		pipelineRepo: pr,
	}
}

type systemPageData struct {
	Runs []*domain.PipelineRun
}

// Page handles GET /ui/system.
func (h *SystemHandler) Page(w http.ResponseWriter, r *http.Request) {
	runs, err := h.pipelineRepo.List(r.Context(), 20)
	if err != nil {
		renderError(w, http.StatusInternalServerError, err.Error())
		return
	}
	renderPage(w, h.pageTmpl, systemPageData{Runs: runs})
}

// Trigger handles POST /ui/system/trigger?job_type=scrape|generate.
// Runs the job in a goroutine and immediately returns the pipeline status fragment.
func (h *SystemHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	jobType := r.URL.Query().Get("job_type")
	if jobType != "scrape" && jobType != "generate" {
		http.Error(w, "job_type must be 'scrape' or 'generate'", http.StatusBadRequest)
		return
	}

	go func() {
		ctx := context.Background() // detached: request context cancelled on response send
		switch jobType {
		case "scrape":
			_ = h.scrapeUC.Run(ctx, "ui")
		case "generate":
			_ = h.generateUC.Run(ctx, "ui")
		}
	}()

	// Return a placeholder status card.
	run := &domain.PipelineRun{
		JobType:     domain.JobType(jobType),
		Status:      domain.RunStatusRunning,
		TriggeredBy: domain.TriggeredByUI,
		StartedAt:   "実行中...",
	}
	if err := h.statusTmpl.ExecuteTemplate(w, "pipeline-status", run); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Cleanup handles POST /ui/system/cleanup.
func (h *SystemHandler) Cleanup(w http.ResponseWriter, r *http.Request) {
	if err := h.cleanupUC.Run(r.Context()); err != nil {
		http.Error(w, "クリーンアップ失敗: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte(`<p style="color:green">✅ クリーンアップ完了</p>`))
}

// PipelineStatus handles GET /ui/system/pipeline/{id}/status (HTMX polling).
func (h *SystemHandler) PipelineStatus(w http.ResponseWriter, r *http.Request) {
	id, err := handler.PathInt64(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p, err := h.pipelineRepo.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := h.statusTmpl.ExecuteTemplate(w, "pipeline-status", p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
