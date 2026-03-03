package web

import (
	"html/template"
	"net/http"
	"strconv"

	"ai-news/internal/domain"
	"ai-news/internal/usecase"
)

// SettingsHandler serves the settings management page.
type SettingsHandler struct {
	pageTmpl   *template.Template // layout.html + settings.html (includes category-row define)
	catRowTmpl *template.Template // settings.html standalone (for category-row HTMX fragment)
	settingsUC *usecase.SettingsUsecase
	categoryUC *usecase.CategoryUsecase
}

// NewSettingsHandler creates a SettingsHandler.
func NewSettingsHandler(
	pageTmpl, catRowTmpl *template.Template,
	suc *usecase.SettingsUsecase,
	cuc *usecase.CategoryUsecase,
) *SettingsHandler {
	return &SettingsHandler{
		pageTmpl:   pageTmpl,
		catRowTmpl: catRowTmpl,
		settingsUC: suc,
		categoryUC: cuc,
	}
}

type settingsPageData struct {
	Settings   *domain.AppSettings
	Categories []*domain.CategorySettings
	FlashMsg   string
	ErrMsg     string
}

// Page handles GET /ui/settings.
func (h *SettingsHandler) Page(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s, err := h.settingsUC.Get(ctx)
	if err != nil {
		renderError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cats, err := h.categoryUC.List(ctx)
	if err != nil {
		renderError(w, http.StatusInternalServerError, err.Error())
		return
	}
	data := settingsPageData{Settings: s, Categories: cats}
	if r.URL.Query().Get("saved") == "1" {
		data.FlashMsg = "設定を保存しました"
	}
	renderPage(w, h.pageTmpl, data)
}

// Update handles POST /ui/settings.
func (h *SettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		renderError(w, http.StatusBadRequest, err.Error())
		return
	}
	speed, _ := strconv.ParseFloat(r.FormValue("voicevox_speed_scale"), 64)
	retDays, _ := strconv.Atoi(r.FormValue("retention_days"))
	s := &domain.AppSettings{
		VoicevoxSpeedScale: speed,
		GeminiModel:        r.FormValue("gemini_model"),
		RetentionDays:      retDays,
	}
	if err := h.settingsUC.Update(r.Context(), s); err != nil {
		renderError(w, http.StatusInternalServerError, err.Error())
		return
	}
	http.Redirect(w, r, "/ui/settings?saved=1", http.StatusSeeOther)
}

// CreateCategory handles POST /ui/settings/categories (HTMX: returns new row).
func (h *SettingsHandler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	speakerID, _ := strconv.Atoi(r.FormValue("voicevox_speaker_id"))
	artCount, _ := strconv.Atoi(r.FormValue("articles_per_episode"))
	summChars, _ := strconv.Atoi(r.FormValue("summary_chars_per_article"))
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	c := &domain.CategorySettings{
		Category:               r.FormValue("category"),
		DisplayName:            r.FormValue("display_name"),
		TTSEngine:              r.FormValue("tts_engine"),
		VoicevoxSpeakerID:     speakerID,
		Language:               r.FormValue("language"),
		ArticlesPerEpisode:     artCount,
		SummaryCharsPerArticle: summChars,
		SortOrder:              sortOrder,
		Enabled:                r.FormValue("enabled") != "",
	}
	if err := h.categoryUC.Create(r.Context(), c); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.catRowTmpl.ExecuteTemplate(w, "category-row", c); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// DeleteCategory handles DELETE /ui/settings/categories/{name} (HTMX: removes row).
func (h *SettingsHandler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "missing category name", http.StatusBadRequest)
		return
	}
	if err := h.categoryUC.Delete(r.Context(), name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK) // HTMX replaces row with empty response
}
