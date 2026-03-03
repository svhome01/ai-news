package api

import (
	"net/http"
	"strconv"

	"ai-news/internal/handler"
	"ai-news/internal/repository"
)

// BroadcastHandler implements GET /api/broadcasts.
type BroadcastHandler struct {
	broadcastRepo *repository.BroadcastRepo
}

// NewBroadcastHandler creates a BroadcastHandler.
func NewBroadcastHandler(r *repository.BroadcastRepo) *BroadcastHandler {
	return &BroadcastHandler{broadcastRepo: r}
}

// List handles GET /api/broadcasts?category=tech&limit=7.
func (h *BroadcastHandler) List(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	limit := 7
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}

	broadcasts, err := h.broadcastRepo.ListByCategory(r.Context(), category, limit)
	if err != nil {
		handler.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type item struct {
		ID          int64   `json:"id"`
		Category    string  `json:"category"`
		Title       string  `json:"title"`
		FileURL     *string `json:"file_url"`
		DurationSec *int    `json:"duration_sec"`
		CreatedAt   string  `json:"created_at"`
	}

	items := make([]item, len(broadcasts))
	for i, b := range broadcasts {
		items[i] = item{
			ID:          b.ID,
			Category:    b.Category,
			Title:       b.Title,
			FileURL:     b.FileURL,
			DurationSec: b.DurationSec,
			CreatedAt:   b.CreatedAt,
		}
	}
	handler.WriteJSON(w, http.StatusOK, items)
}
