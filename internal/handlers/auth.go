package handlers

import (	
	"encoding/json"
	"html/template"
	"net/http"
	"path/filepath"
	"time"
	"wikifront/internal/model"
)

type AuthClient interface {
	Login(username, password string) (model.User, error)
}

type AuthHandler struct {
	client    AuthClient
	templates *template.Template
}

func NewAuthHandler(client AuthClient) (*AuthHandler, error) {
	tmpl, err := template.ParseFiles(
		filepath.Join("web", "templates", "layouts", "base.html"),
		filepath.Join("web", "templates", "pages", "login.html"),
	)
	if err != nil {
		return nil, err
	}
	return &AuthHandler{client: client, templates: tmpl}, nil
}

func (h *AuthHandler) ShowLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		h.templates.ExecuteTemplate(w, "base", nil)
		return
	}

	// Обработка POST-формы логина
	username := r.FormValue("username")
	password := r.FormValue("password")

	user, err := h.client.Login(username, password)
	if err != nil {
		// В случае ошибки рендерим страницу снова с сообщением (упрощенно)
		http.Error(w, "Неверный логин или пароль", http.StatusUnauthorized)
		return
	}

	// Кодируем юзера в куку для Middleware
	userBytes, _ := json.Marshal(user)
	http.SetCookie(w, &http.Cookie{
		Name:     "session_user",
		Value:    string(userBytes),
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true, // Защита от XSS
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Удаляем куку сессии
	http.SetCookie(w, &http.Cookie{
		Name:     "session_user",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
