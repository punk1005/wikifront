package handlers

import (
	"html/template"
	"net/http"
	"path/filepath"
	"strings"
	"wikifront/internal/middleware"
	"wikifront/internal/model"
)

type FolderClient interface {
	GetFolders() ([]model.Folder, error)
	GetArticlesByFolder(folderSlug string) ([]model.Article, error)
}

type FolderHandler struct {
	client    FolderClient
	templates *template.Template
}

func NewFolderHandler(client FolderClient) (*FolderHandler, error) {
	tmpl, err := template.ParseFiles(
		filepath.Join("web", "templates", "layouts", "base.html"),
		filepath.Join("web", "templates", "pages", "folders.html"),
	)
	if err != nil {
		return nil, err
	}
	return &FolderHandler{client: client, templates: tmpl}, nil
}

func (h *FolderHandler) ListFoldersOrArticles(w http.ResponseWriter, r *http.Request) {
	var currentUser *model.User
	if val := r.Context().Value(middleware.UserKey); val != nil {
		currentUser = val.(*model.User)
	}

	// Парсим URL: /folders или /folders/some-folder-slug
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if len(parts) == 1 {
		// Показываем все папки
		folders, _ := h.client.GetFolders()
		h.templates.ExecuteTemplate(w, "base", map[string]interface{}{
			"User":    currentUser,
			"Folders": folders,
			"IsList":  true,
		})
		return
	}

	// Показываем статьи внутри конкретной папки
	folderSlug := parts[1]
	articles, _ := h.client.GetArticlesByFolder(folderSlug)
	h.templates.ExecuteTemplate(w, "base", map[string]interface{}{
		"User":       currentUser,
		"Articles":   articles,
		"FolderSlug": folderSlug,
		"IsList":     false,
	})
}
