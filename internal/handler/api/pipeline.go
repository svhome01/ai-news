package api

import (
	"net/http"

	"ai-news/internal/domain"
	"ai-news/internal/handler"
	"ai-news/internal/repository"
	"ai-news/internal/usecase"
)

// PipelineHandler implements /api/pipeline/* endpoints.
type PipelineHandler struct {
	scrapeUC    *usecase.ScrapeUsecase
	generateUC  *usecase.GenerateUsecase
	pipelineRepo *repository.PipelineRepo
}

// NewPipelineHandler creates a PipelineHandler.
func NewPipelineHandler(
	su *usecase.ScrapeUsecase,
	gu *usecase.GenerateUsecase,
	pr *repository.PipelineRepo,
) *PipelineHandler {
	return &PipelineHandler{scrapeUC: su, generateUC: gu, pipelineRepo: pr}
}

// Trigger handles POST /api/pipeline/run?job_type=scrape|generate (default: generate).
// Runs the job in a goroutine and immediately returns the run_id.
func (h *PipelineHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	jobType := r.URL.Query().Get("job_type")
	if jobType == "" {
		jobType = "generate"
	}
	if jobType != "scrape" && jobType != "generate" {
		handler.WriteError(w, http.StatusBadRequest, "job_type must be 'scrape' or 'generate'")
		return
	}

	go func() {
		ctx := r.Context()
		switch jobType {
		case "scrape":
			if err := h.scrapeUC.Run(ctx, "api"); err != nil {
				_ = err
			}
		case "generate":
			if err := h.generateUC.Run(ctx, "api"); err != nil {
				_ = err
			}
		}
	}()

	handler.WriteJSON(w, http.StatusAccepted, map[string]string{
		"status":   "triggered",
		"job_type": jobType,
	})
}

// Status handles GET /api/pipeline/{id}.
func (h *PipelineHandler) Status(w http.ResponseWriter, r *http.Request) {
	id, err := handler.PathInt64(r, "id")
	if err != nil {
		handler.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	p, err := h.pipelineRepo.GetByID(r.Context(), id)
	if err != nil {
		if err == domain.ErrNotFound {
			handler.WriteError(w, http.StatusNotFound, "run not found")
			return
		}
		handler.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	handler.WriteJSON(w, http.StatusOK, map[string]any{
		"id":                 p.ID,
		"job_type":          string(p.JobType),
		"status":            string(p.Status),
		"triggered_by":      string(p.TriggeredBy),
		"current_step":      p.CurrentStep,
		"articles_collected": p.ArticlesCollected,
		"articles_selected":  p.ArticlesSelected,
		"error_message":     p.ErrorMessage,
		"started_at":        p.StartedAt,
		"finished_at":       p.FinishedAt,
	})
}
