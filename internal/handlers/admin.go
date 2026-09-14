package handlers

import (
	"html/template"
	"net/http"
	"path/filepath"
	"wikifront/internal/middleware"
	"wikifront/internal/model"
)

type AdminClient interface {
	GetPendingDeleteArticles() ([]model.Article, error)
}

type AdminHandler struct {
	client    AdminClient
	templates *template.Template
}

func NewAdminHandler(client AdminClient) (*AdminHandler, error) {
	tmpl, err := template.ParseFiles(
		filepath.Join("web", "templates", "layouts", "base.html"),
		filepath.Join("web", "templates", "pages", "admin.html"),
	)
	if err != nil {
		return nil, err
	}
	return &AdminHandler{client: client, templates: tmpl}, nil
}

func (h *AdminHandler) ShowDashboard(w http.ResponseWriter, r *http.Request) {
	var currentUser *model.User
	if val := r.Context().Value(middleware.UserKey); val != nil {
		currentUser = val.(*model.User)
	}

	// Запрашиваем из wikiapi список "проблемных" статей
	articles, err := h.client.GetPendingDeleteArticles()
	if err != nil {
		http.Error(w, "Ошибка получения данных админки", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.templates.ExecuteTemplate(w, "base", map[string]interface{}{
		"User":     currentUser,
		"Articles": articles,
	})
}
