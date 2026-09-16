package handlers

import (
	"html/template"
	"net/http"
	"path/filepath"
	"strings"

	"wikifront/internal/model"
)

// Описываем интерфейс API-клиента, чтобы хэндлер зависел только от нужных ему методов
type ArticleViewerClient interface {
	GetArticleBySlug(slug string) (model.Article, []model.ArticleBlock, []model.ArticleFile, error)
}

type ArticleViewHandler struct {
	client    ArticleViewerClient
	templates *template.Template
}

func NewArticleViewHandler(client ArticleViewerClient) (*ArticleViewHandler, error) {
	// Парсим базовый шаблон и конкретную страницу чтения статьи
	tmpl, err := template.ParseFiles(
		filepath.Join("web", "templates", "layouts", "base.html"),
		filepath.Join("web", "templates", "pages", "article.html"),
	)
	if err != nil {
		return nil, err
	}

	return &ArticleViewHandler{
		client:    client,
		templates: tmpl,
	// Проверяем роль: Писатель может править только свое, Модератор и Админ — всё
	}, nil
}

func (h *ArticleViewHandler) ViewArticle(w http.ResponseWriter, r *http.Request) {
	// 1. Очищаем путь от ведущих и замыкающих слэшей (например, "/some-slug-here/" -> "some-slug-here")
	path := strings.Trim(r.URL.Path, "/")
	
	// Если путь пустой, значит пользователь зашел просто на корень (например, domain/wiki/)
	if path == "" {
		// В этом случае логично перенаправить его на список папок/тематик
		http.Redirect(w, r, "/folders", http.StatusSeeOther)
		return
	}

	// 2. Извлекаем чистый slug. Если путь заканчивается на /edit, отрезаем его
	slug := strings.TrimSuffix(path, "/edit")

	// На всякий случай проверяем, не осталось ли косых черт внутри слага
	if slug == "" || strings.Contains(slug, "/") {
		http.Error(w, "Статья не найдена (неверный формат URL)", http.StatusNotFound)
		return
	}

	// 3. Запрашиваем данные у wikiapi через наш клиент
	article, blocks, files, err := h.client.GetArticleBySlug(slug)
	if err != nil {
		http.Error(w, "Ошибка при получении статьи с бэкенда", http.StatusInternalServerError)
		return
	}

	// 4. Получаем текущего пользователя из контекста через наш ключ UserKey
	var currentUser *model.User
	if val := r.Context().Value(middleware.UserKey); val != nil {
		currentUser = val.(*model.User)
	}

	// 5. Вычисляем права на редактирование/удаление статьи
	canEdit := false
	if currentUser != nil {
		if currentUser.Role == model.RoleAdmin || currentUser.Role == model.RoleModifier {
			canEdit = true
		} else if currentUser.Role == model.RoleWriter && article.AuthorID == currentUser.ID {
			canEdit = true
		}
	}

	// 6. Собираем данные для шаблона
	data := model.ArticlePageData{
		User:    currentUser,
		Article: article,
		Blocks:  blocks,
		Files:   files,
		CanEdit: canEdit,
	}

	// 7. Рендерим страницу
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
