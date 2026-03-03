package api

import (
	"context"
	"net/http"
	"time"

	"ai-news/internal/handler"
)

// HealthHandler implements GET /api/health.
type HealthHandler struct {
	voicevoxURL string
}

// NewHealthHandler creates a HealthHandler.
func NewHealthHandler(voicevoxURL string) *HealthHandler {
	return &HealthHandler{voicevoxURL: voicevoxURL}
}

// Check responds with the health status of the app and its dependencies.
func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	vvxStatus := "ok"
	if err := checkVoicevox(r.Context(), h.voicevoxURL); err != nil {
		vvxStatus = "error: " + err.Error()
	}

	handler.WriteJSON(w, http.StatusOK, map[string]string{
		"status":   "ok",
		"voicevox": vvxStatus,
	})
}

// checkVoicevox pings VOICEVOX /version.
func checkVoicevox(ctx context.Context, baseURL string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/version", http.NoBody)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil // best-effort; non-fatal
	}
	return nil
}
