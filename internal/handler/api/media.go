package api

import (
	"net/http"

	"ai-news/internal/domain"
	"ai-news/internal/handler"
	"ai-news/internal/infra/storage"
	"ai-news/internal/repository"
)

// MediaHandler serves MP3 files streamed from SMB.
// GET /media/{category}/latest
// GET /media/{category}/{id}
type MediaHandler struct {
	broadcastRepo *repository.BroadcastRepo
	musicStore    *storage.MusicStore
}

// NewMediaHandler creates a MediaHandler.
func NewMediaHandler(br *repository.BroadcastRepo, ms *storage.MusicStore) *MediaHandler {
	return &MediaHandler{broadcastRepo: br, musicStore: ms}
}

// ServeLatest streams the most recent broadcast MP3 for a category.
func (h *MediaHandler) ServeLatest(w http.ResponseWriter, r *http.Request) {
	category := r.PathValue("category")
	bc, err := h.broadcastRepo.GetLatest(r.Context(), category)
	if err != nil {
		if err == domain.ErrNotFound {
			http.Error(w, "no broadcast found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.stream(w, r, bc.FilePath)
}

// ServeByID streams a specific broadcast MP3 by its ID.
func (h *MediaHandler) ServeByID(w http.ResponseWriter, r *http.Request) {
	id, err := handler.PathInt64(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	bc, err := h.broadcastRepo.GetByID(r.Context(), id)
	if err != nil {
		if err == domain.ErrNotFound {
			http.Error(w, "broadcast not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.stream(w, r, bc.FilePath)
}

// stream reads the MP3 from SMB and writes it to the response.
func (h *MediaHandler) stream(w http.ResponseWriter, r *http.Request, filePath string) {
	if filePath == "" {
		http.Error(w, "broadcast has no file", http.StatusNotFound)
		return
	}
	if err := h.musicStore.StreamMP3(w, filePath); err != nil {
		// Headers may already be partially written; just log.
		_ = err
	}
}
