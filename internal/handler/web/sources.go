package web

import (
	"html/template"
	"net/http"

	"ai-news/internal/domain"
	"ai-news/internal/handler"
	"ai-news/internal/usecase"
)

// SourcesHandler serves the sources management page.
type SourcesHandler struct {
	pageTmpl   *template.Template // layout.html + sources.html + source_row.html
	rowTmpl    *template.Template // source_row.html (standalone, for HTMX)
	sourceUC   *usecase.SourceUsecase
	categoryUC *usecase.CategoryUsecase
}

// NewSourcesHandler creates a SourcesHandler.
func NewSourcesHandler(
	pageTmpl, rowTmpl *template.Template,
	suc *usecase.SourceUsecase,
	cuc *usecase.CategoryUsecase,
) *SourcesHandler {
	return &SourcesHandler{
		pageTmpl:   pageTmpl,
		rowTmpl:    rowTmpl,
		sourceUC:   suc,
		categoryUC: cuc,
	}
}

type sourcesPageData struct {
	Sources    []*domain.Source
	Categories []*domain.CategorySettings
}

type sourceEditData struct {
	Source     *domain.Source
	Categories []*domain.CategorySettings
}

// Page handles GET /ui/sources.
func (h *SourcesHandler) Page(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sources, err := h.sourceUC.List(ctx)
	if err != nil {
		renderError(w, http.StatusInternalServerError, err.Error())
		return
	}
	categories, err := h.categoryUC.List(ctx)
	if err != nil {
		renderError(w, http.StatusInternalServerError, err.Error())
		return
	}
	renderPage(w, h.pageTmpl, sourcesPageData{Sources: sources, Categories: categories})
}

// Create handles POST /ui/sources (HTMX: returns new view row).
func (h *SourcesHandler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s := &domain.Source{
		Name:        r.FormValue("name"),
		URL:         r.FormValue("url"),
		Category:    r.FormValue("category"),
		FetchMethod: domain.FetchMethod(r.FormValue("fetch_method")),
		Enabled:     r.FormValue("enabled") != "",
	}
	if css := r.FormValue("css_selector"); css != "" {
		s.CSSSelector = &css
	}
	id, err := h.sourceUC.Create(r.Context(), s)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.ID = id

	if err := h.rowTmpl.ExecuteTemplate(w, "source-row", s); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// GetView handles GET /ui/sources/{id} (HTMX: returns view row, used by cancel button).
func (h *SourcesHandler) GetView(w http.ResponseWriter, r *http.Request) {
	id, err := handler.PathInt64(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s, err := h.sourceUC.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := h.rowTmpl.ExecuteTemplate(w, "source-row", s); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// GetEdit handles GET /ui/sources/{id}/edit (HTMX: returns edit form row).
func (h *SourcesHandler) GetEdit(w http.ResponseWriter, r *http.Request) {
	id, err := handler.PathInt64(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s, err := h.sourceUC.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	categories, err := h.categoryUC.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.rowTmpl.ExecuteTemplate(w, "source-edit-row", sourceEditData{
		Source:     s,
		Categories: categories,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Update handles PUT /ui/sources/{id} (HTMX: returns updated view row).
func (h *SourcesHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := handler.PathInt64(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s := &domain.Source{
		ID:          id,
		Name:        r.FormValue("name"),
		URL:         r.FormValue("url"),
		Category:    r.FormValue("category"),
		FetchMethod: domain.FetchMethod(r.FormValue("fetch_method")),
		Enabled:     r.FormValue("enabled") != "",
	}
	if css := r.FormValue("css_selector"); css != "" {
		s.CSSSelector = &css
	}
	if err := h.sourceUC.Update(r.Context(), s); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.rowTmpl.ExecuteTemplate(w, "source-row", s); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Delete handles DELETE /ui/sources/{id} (HTMX: returns empty to remove row).
func (h *SourcesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := handler.PathInt64(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.sourceUC.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK) // HTMX replaces row with empty response
}
