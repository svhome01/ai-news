package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"ai-news/internal/domain"
)

// WriteJSON encodes v as JSON and writes it with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Headers already sent; nothing useful to do.
		_ = err
	}
}

// WriteError sends a JSON error response.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// PathInt64 parses an integer path value from {name} in Go 1.22+ ServeMux.
func PathInt64(r *http.Request, name string) (int64, error) {
	s := r.PathValue(name)
	if s == "" {
		return 0, fmt.Errorf("missing path parameter %q", name)
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %q: %w", name, err)
	}
	return v, nil
}

// IsNotFound reports whether err is domain.ErrNotFound.
func IsNotFound(err error) bool {
	return err == domain.ErrNotFound
}
