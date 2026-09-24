package main

import (
	"context"
	"log"
	"net/http"
	"strings"
)

type contextKey string
const UserContextKey contextKey = "user"

// Главная middleware для проверки авторизации
func MW_AuthRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// ТОТАЛЬНАЯ ПРОВЕРКА НА NIL
		if w == nil {
			log.Printf("[CRITICAL ERROR] http.ResponseWriter IS NIL AT START OF MW_AuthRequired!")
		}
		if r == nil {
			log.Printf("[CRITICAL ERROR] *http.Request IS NIL AT START OF MW_AuthRequired!")
			return // Предотвращаем панику дальше
		}

		path := r.URL.Path

		// Исключения, куда пускаем без кук
		if path == "/login" || path == "/api/post/dologin" || strings.HasPrefix(path, "/static/") {
			if next == nil {
				log.Printf("[CRITICAL ERROR] next HandlerFunc is NIL for path: %s", path)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			next(w, r)
			return
		}

		redirectPath := "/login"
		if Cfg != nil {
			redirectPath = Cfg.AppPrefix + "/login"
		} else {
			log.Printf("[WARNING] Cfg is NIL inside middleware for path: %s", path)
		}

		cookie, err := r.Cookie("session_id")
		if err != nil {
			// ЛОГИРУЕМ ПЕРЕД РЕДИРЕКТОМ
			log.Printf("[DEBUG] Кука отсутствует для пути %s. Попытка редиректа на %s", path, redirectPath)
			
			if w == nil || r == nil {
				log.Printf("[CRITICAL ERROR] Сбоит перед http.Redirect: w=%v, r=%v", w, r)
				return
			}
			
			http.Redirect(w, r, redirectPath, http.StatusSeeOther)
			return
		}

		user, found := GlobalSessionManager.GetSession(cookie.Value)
		if !found || user == nil {
			log.Printf("[DEBUG] Сессия не найдена или протухла для пути %s. Удаляем куку и редиректим.", path)
			http.SetCookie(w, &http.Cookie{
				Name:     "session_id",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
			})
			http.Redirect(w, r, redirectPath, http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), UserContextKey, user)
		next(w, r.WithContext(ctx))
	}
}

func MW_Logger(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[%s] %s", r.Method, r.URL.Path)
		next(w, r)
	}
}

func MW_Root(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Любой авторизованный пользователь может видеть главную
		next(w, r)
	}
}

func MW_Admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := r.Context().Value(UserContextKey).(*UserInfo)
		if user == nil || user.Role != "admin" {
			http.Error(w, "403 Forbidden: Требуются права Администратора", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func MW_Article(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Читать могут все авторизованные (reader, writer, moderator, admin)
		next(w, r)
	}
}

func MW_EditArticle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := r.Context().Value(UserContextKey).(*UserInfo)
		// Запрещаем роль 'reader' редактировать статьи
		if user == nil || user.Role == "reader" {
			http.Error(w, "403 Forbidden: Недостаточно прав для редактирования", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}
