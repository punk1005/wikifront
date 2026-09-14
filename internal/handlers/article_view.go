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
	// Извлекаем slug из URL (например, из /wiki/some-slug-here)
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 || parts[2] == "" {
		http.Error(w, "Статья не найдена (неверный URL)", http.StatusNotFound)
		return
	}
	slug := parts[2]

	// 1. Запрашиваем данные у wikiapi через наш клиент
	article, blocks, files, err := h.client.GetArticleBySlug(slug)
	if err != nil {
		http.Error(w, "Ошибка при получении статьи с бэкенда", http.StatusInternalServerError)
		return
	}

	// 2. Получаем текущего пользователя из контекста (его туда положит Middleware авторизации)
	// Для тестирования пока представим, что юзер прилетит из сессии. Если его нет — он reader.
	var currentUser *model.User
	if val := r.Context().Value("user"); val != nil {
		currentUser = val.(*model.User)
	}

	// 3. Вычисляем права на редактирование/удаление статьи
	canEdit := false
	if currentUser != nil {
		if currentUser.Role == model.RoleAdmin || currentUser.Role == model.RoleModifier {
			canEdit = true
		} else if currentUser.Role == model.RoleWriter && article.AuthorID == currentUser.ID {
			canEdit = true
		}
	}

	// 4. Собираем данные для шаблона
	data := model.ArticlePageData{
		User:    currentUser,
		Article: article,
		Blocks:  blocks,
		Files:   files,
		CanEdit: canEdit,
	}

	// 5. Рендерим страницу. Название шаблона "base" определено внутри layouts/base.html
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
