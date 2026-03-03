package web

import (
	"html/template"
	"log"
	"net/http"
)

// renderPage executes the named "layout.html" template from t with the given data.
func renderPage(w http.ResponseWriter, t *template.Template, data any) {
	if err := t.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("template render error: %v", err)
		http.Error(w, "テンプレートエラー", http.StatusInternalServerError)
	}
}

// renderError writes an HTTP error response with a plain text message.
func renderError(w http.ResponseWriter, status int, msg string) {
	http.Error(w, msg, status)
}
