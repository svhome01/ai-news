package api

import (
	"net/http"

	"ai-news/internal/domain"
	"ai-news/internal/handler"
	"ai-news/internal/usecase"
)

// PlayHandler implements POST /api/play/{category} and POST /api/play/stop.
type PlayHandler struct {
	playbackUC *usecase.PlaybackUsecase
}

// NewPlayHandler creates a PlayHandler.
func NewPlayHandler(uc *usecase.PlaybackUsecase) *PlayHandler {
	return &PlayHandler{playbackUC: uc}
}

// Stop is the POST /api/play/stop endpoint (exact match, higher priority than Play).
// The actual stop action is performed by Home Assistant; this just acknowledges.
func (h *PlayHandler) Stop(w http.ResponseWriter, r *http.Request) {
	handler.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Play handles POST /api/play/{category}.
// Returns broadcast metadata so the caller (HA) can use the media_url for play_media.
func (h *PlayHandler) Play(w http.ResponseWriter, r *http.Request) {
	category := r.PathValue("category")
	if category == "" {
		handler.WriteError(w, http.StatusBadRequest, "missing category")
		return
	}

	result, err := h.playbackUC.GetLatest(r.Context(), category)
	if err != nil {
		if handler.IsNotFound(err) {
			handler.WriteError(w, http.StatusNotFound, "no broadcast found for category: "+category)
			return
		}
		if err == domain.ErrNotFound {
			handler.WriteError(w, http.StatusNotFound, "unknown category: "+category)
			return
		}
		handler.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	handler.WriteJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"title":        result.Title,
		"media_url":    result.MediaURL,
		"broadcast_id": result.BroadcastID,
		"duration_sec": result.DurationSec,
	})
}
