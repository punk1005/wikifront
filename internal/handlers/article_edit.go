package handlers

import (
	"encoding/json"
	"html/template"
	"net/http"
	"path/filepath"
	"strings"

	"wikifront/internal/model"
)

// Описываем требования хэндлера к API-клиенту
type ArticleEditorClient interface {
	GetFolders() ([]model.Folder, error)
	GetArticleBySlug(slug string) (model.Article, []model.ArticleBlock, []model.ArticleFile, error)
}

type ArticleEditHandler struct {
	client    ArticleEditorClient
	templates *template.Template
}

func NewArticleEditHandler(client ArticleEditorClient) (*ArticleEditHandler, error) {
	tmpl, err := template.ParseFiles(
		filepath.Join("web", "templates", "layouts", "base.html"),
		filepath.Join("web", "templates", "pages", "editor.html"),
	)
	if err != nil {
		return nil, err
	}

	return &ArticleEditHandler{
		client:    client,
		templates: tmpl,
	}, nil
}

// Данные, которые мы прокидываем в шаблон editor.html
type EditorPageData struct {
	User        *model.User
	IsEdit      bool
	Article     model.Article
	Folders     []model.Folder
	BlocksJSON  template.JS // Передаем блоки строкой JSON, безопасной для встраивания в JS
}

// Показ страницы создания новой статьи
func (h *ArticleEditHandler) ShowCreateForm(w http.ResponseWriter, r *http.Request) {
	currentUser := h.getUserFromContext(r)
	
	// Ограничение: Читатель не может создавать статьи
	if currentUser == nil || currentUser.Role == model.RoleReader {
		http.Error(w, "Доступ запрещен. У вас нет прав для создания статей.", http.StatusForbidden)
		return
	}

	// Получаем список доступных папок для выпадающего списка
	folders, err := h.client.GetFolders()
	if err != nil {
		http.Error(w, "Ошибка при получении категорий", http.StatusInternalServerError)
		return
	}

	data := EditorPageData{
		User:    currentUser,
		IsEdit:  false,
		Folders: folders,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.templates.ExecuteTemplate(w, "base", data)
}

// ShowEditForm показывает страницу редактирования существующей статьи
func (h *ArticleEditHandler) ShowEditForm(w http.ResponseWriter, r *http.Request) {
	// 1. Получаем текущего пользователя из контекста через правильный ключ пакета middleware
	var currentUser *model.User
	if val := r.Context().Value(middleware.UserKey); val != nil {
		currentUser = val.(*model.User)
	}

	// Защита: Анонимы и Читатели не имеют доступа к редактору
	if currentUser == nil || currentUser.Role == model.RoleReader {
		http.Error(w, "Доступ запрещен.", http.StatusForbidden)
		return
	}

	// 2. Очищаем путь от ведущих/замыкающих слэшей (например, "/my-slug/edit/" -> "my-slug/edit")
	path := strings.Trim(r.URL.Path, "/")
	
	// Отрезаем суффикс "/edit", чтобы получить чистый slug статьи
	slug := strings.TrimSuffix(path, "/edit")

	// Проверяем корректность слага (он не должен быть пустым или содержать другие слэши)
	if slug == "" || strings.Contains(slug, "/") {
		http.Error(w, "Неверный формат URL статьи", http.StatusBadRequest)
		return
	}

	// 3. Запрашиваем статью и её блоки из API по чистому slug
	article, blocks, _, err := h.client.GetArticleBySlug(slug)
	if err != nil {
		http.Error(w, "Статья не найдена", http.StatusNotFound)
		return
	}

	// 4. ПРОВЕРКА РОЛЕЙ: Писатель может править только свою статью
	if currentUser.Role == model.RoleWriter && article.AuthorID != currentUser.ID {
		http.Error(w, "Вы можете редактировать только собственные статьи.", http.StatusForbidden)
		return
	}

	// 5. Переводим массив блоков в JSON для инициализации Quill-редакторов на фронтенде
	blocksBytes, err := json.Marshal(blocks)
	if err != nil {
		http.Error(w, "Ошибка сериализации блоков контента", http.StatusInternalServerError)
		return
	}

	// 6. Получаем папки для выпадающего списка
	folders, err := h.client.GetFolders()
	if err != nil {
		http.Error(w, "Ошибка при получении категорий", http.StatusInternalServerError)
		return
	}

	data := EditorPageData{
		User:       currentUser,
		IsEdit:     true,
		Article:    article,
		Folders:    folders,
		BlocksJSON: template.JS(blocksBytes), // template.JS предотвращает экранирование JSON-кавычек в теге <script>
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}


// Хелпер для извлечения юзера из контекста (сессии)
func (h *ArticleEditHandler) getUserFromContext(r *http.Request) *model.User {
	if val := r.Context().Value("user"); val != nil {
		return val.(*model.User)
	}
	return nil
}
