package main

import (
	"log"
	"net/http"
	"strings"
	"time"

	"wikifront/internal/client"
	"wikifront/internal/config"
	"wikifront/internal/handlers"
	"wikifront/internal/middleware"
	"wikifront/internal/model"
)

func main() {
	// 1. Загружаем конфигурацию (порты веб-сервера и URL бэкенда)
	cfg := config.Load()

	// 2. Инициализируем HTTP-клиент для отправки запросов в wikiapi
	apiClient := client.NewAPIClient(cfg.APIBaseURL)

	// 3. Инициализируем наши изолированные хэндлеры страниц
	viewHandler, err := handlers.NewArticleViewHandler(apiClient)
	if err != nil {
		log.Fatalf("Ошибка инициализации view-хэндлера: %v", err)
	}

	editHandler, err := handlers.NewArticleEditHandler(apiClient)
	if err != nil {
		log.Fatalf("Ошибка инициализации edit-хэндлера: %v", err)
	}

	authHandler, err := handlers.NewAuthHandler(apiClient)
	if err != nil {
		log.Fatalf("Ошибка инициализации auth-хэндлера: %v", err)
	}

	folderHandler, err := handlers.NewFolderHandler(apiClient)
	if err != nil {
		log.Fatalf("Ошибка инициализации folder-хэндлера: %v", err)
	}

	adminHandler, err := handlers.NewAdminHandler(apiClient)
	if err != nil {
		log.Fatalf("Ошибка инициализации admin-хэндлера: %v", err)
	}

	// 4. Настраиваем маршрутизатор (Mux)
	mux := http.NewServeMux()

	// Раздача локальных статических файлов фронтенда (наш кастомный js/editor.js, css)
	fs := http.FileServer(http.Dir("web/static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))


	// --- МАРШРУТЫ СТРАНИЦ АВТОРИЗАЦИИ ---
	
	// Вход и выход из системы (доступны всем, но middleware проверяет куку для шапки)
	mux.Handle("/auth/login", middleware.Authenticate(http.HandlerFunc(authHandler.ShowLogin)))
	mux.Handle("/auth/logout", middleware.Authenticate(http.HandlerFunc(authHandler.Logout)))


	// --- МАРШРУТЫ КОНТЕНТА (HTML) ---

	// Просмотр тематик и списков статей в них
	mux.Handle("/folders/", middleware.Authenticate(http.HandlerFunc(folderHandler.ListFoldersOrArticles)))

	// Просмотр и редактирование конкретных статей по уникальной ссылке (slug)
	mux.Handle("/", middleware.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/edit") {
			editHandler.ShowEditForm(w, r)
		} else {
			viewHandler.ViewArticle(w, r)
		}
	})))

	// Создание новой статьи
	// Защищаем цепочкой middleware: Аутентификация -> Проверка прав (только для Writer, Moderator, Admin)
	createPageHandler := http.HandlerFunc(editHandler.ShowCreateForm)
	protectedCreatePage := middleware.Authenticate(
		middleware.RequireRole(model.RoleWriter, model.RoleModifier, model.RoleAdmin)(createPageHandler),
	)
	mux.Handle("/create", protectedCreatePage)


	// --- МАРШРУТ АДМИНИСТРАТОРА ---

	// Панель админа: окончательное hard-delete удаление статей (Строго роль Admin)
	adminDashboard := http.HandlerFunc(adminHandler.ShowDashboard)
	protectedAdmin := middleware.Authenticate(
		middleware.RequireRole(model.RoleAdmin)(adminDashboard),
	)
	mux.Handle("/admin", protectedAdmin)


	// --- МАРШРУТ API ПРОКСИ ---

	// Прозрачное проксирование AJAX-запросов (сохранение, изменение, удаление) с фронта в wikiapi
	mux.Handle("/api/", middleware.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlers.ProxyToAPI(w, r, cfg.APIBaseURL)
	})))


	// 5. Запуск веб-сервера wikifront
	log.Printf("[wikifront] Сервер успешно запущен на порту %s", cfg.Port)
	log.Printf("[wikifront] Настроен на бэкенд wikiapi: %s", cfg.APIBaseURL)
	
	server := &http.Server{
		Addr:         cfg.Port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Критическая ошибка сервера: %v", err)
	}
}
