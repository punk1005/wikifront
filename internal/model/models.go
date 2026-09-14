package model

import (
	"html/template"
	"time"
)

// Роли пользователей
const (
	RoleReader    = "reader"
	RoleWriter    = "writer"
	RoleModifier  = "moderator"
	RoleAdmin     = "admin"
)

// User описывает пользователя в системе
type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// Article описывает метаданные статьи
type Article struct {
	ID               int       `json:"id"`
	FolderID         int       `json:"folder_id"`
	FolderName       string    `json:"folder_name"`
	Title            string    `json:"title"`
	Slug             string    `json:"slug"`
	AuthorID         int       `json:"author_id"`
	AuthorName       string    `json:"author_name"`
	IsDeletedPending bool      `json:"is_deleted_pending"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ArticleBlock описывает один контентный HTML-блок
type ArticleBlock struct {
	ID              int           `json:"id"`
	ArticleID       int           `json:"article_id"`
	Content         template.HTML `json:"content"` // Важно: template.HTML разрешает рендер тегов
	OrdinalPosition int           `json:"ordinal_position"`
}

// ArticleFile описывает прикрепленный к статье файл
type ArticleFile struct {
	ID           int    `json:"id"`
	ArticleID    int    `json:"article_id"`
	FileName     string `json:"file_name"`     // Имя на сервере (UUID)
	OriginalName string `json:"original_name"` // Исходное имя
}

// ArticlePageData собирает все данные вместе для передачи в шаблон article.html
type ArticlePageData struct {
	User    *User
	Article Article
	Blocks  []ArticleBlock
	Files   []ArticleFile
	CanEdit bool // Флаг, разрешено ли текущему юзеру видеть кнопки редактирования
}

// Folder описывает тематическую папку (категорию)
type Folder struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedBy int       `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}