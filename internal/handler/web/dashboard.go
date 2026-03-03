package web

import (
	"html/template"
	"net/http"

	"ai-news/internal/domain"
	"ai-news/internal/repository"
	"ai-news/internal/usecase"
)

// DashboardHandler serves the main dashboard page and HTMX article list fragment.
type DashboardHandler struct {
	pageTmpl     *template.Template // layout.html + dashboard.html + article_list.html
	fragmentTmpl *template.Template // article_list.html (standalone)
	articleUC    *usecase.ArticleUsecase
	categoryUC   *usecase.CategoryUsecase
	broadcastRepo *repository.BroadcastRepo
}

// NewDashboardHandler creates a DashboardHandler.
func NewDashboardHandler(
	pageTmpl, fragmentTmpl *template.Template,
	auc *usecase.ArticleUsecase,
	cuc *usecase.CategoryUsecase,
	br *repository.BroadcastRepo,
) *DashboardHandler {
	return &DashboardHandler{
		pageTmpl:      pageTmpl,
		fragmentTmpl:  fragmentTmpl,
		articleUC:     auc,
		categoryUC:    cuc,
		broadcastRepo: br,
	}
}

type dashboardData struct {
	Categories []*domain.CategorySettings
	Category   string
	Broadcast  *domain.Broadcast
}

// Page handles GET /.
func (h *DashboardHandler) Page(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	categories, err := h.categoryUC.ListEnabled(ctx)
	if err != nil {
		renderError(w, http.StatusInternalServerError, err.Error())
		return
	}

	category := r.URL.Query().Get("category")
	if category == "" && len(categories) > 0 {
		category = categories[0].Category
	}

	var bc *domain.Broadcast
	if category != "" {
		bc, _ = h.broadcastRepo.GetLatest(ctx, category) // non-fatal
	}

	renderPage(w, h.pageTmpl, dashboardData{
		Categories: categories,
		Category:   category,
		Broadcast:  bc,
	})
}

type articleListData struct {
	Articles []*domain.Article
	Category string
}

// ArticleList handles GET /ui/articles (HTMX fragment).
func (h *DashboardHandler) ArticleList(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	articles, err := h.articleUC.ListByCategory(r.Context(), category, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := h.fragmentTmpl.ExecuteTemplate(w, "article-list", articleListData{
		Articles: articles,
		Category: category,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
