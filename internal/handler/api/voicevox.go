package api

import (
	"net/http"

	"ai-news/internal/handler"
	"ai-news/internal/usecase"
)

// VoicevoxHandler proxies VOICEVOX speaker listing for the settings UI.
type VoicevoxHandler struct {
	categoryUC *usecase.CategoryUsecase
}

// NewVoicevoxHandler creates a VoicevoxHandler.
func NewVoicevoxHandler(uc *usecase.CategoryUsecase) *VoicevoxHandler {
	return &VoicevoxHandler{categoryUC: uc}
}

// Speakers handles GET /api/voicevox/speakers.
func (h *VoicevoxHandler) Speakers(w http.ResponseWriter, r *http.Request) {
	speakers, err := h.categoryUC.Speakers(r.Context())
	if err != nil {
		handler.WriteError(w, http.StatusServiceUnavailable, "voicevox unavailable: "+err.Error())
		return
	}
	handler.WriteJSON(w, http.StatusOK, speakers)
}
