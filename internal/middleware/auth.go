package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"wikifront/internal/model"
)

// Ключ для контекста (используем кастомный тип, чтобы избежать конфликтов)
type contextKey string
const UserKey contextKey = "user"

func Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_user")
		if err != nil {
			// Куки нет — пользователь считается анонимным (Reader)
			next.ServeHTTP(w, r)
			return
		}

		// Десериализуем данные пользователя из куки (в реальном проекте тут будет JWT или ID сессии из Redis/БД)
		var user model.User
		if err := json.Unmarshal([]byte(cookie.Value), &user); err != nil {
			// Если кука побилась — очищаем её и пускаем как гостя
			http.SetCookie(w, &http.Cookie{Name: "session_user", Value: "", MaxAge: -1, Path: "/"})
			next.ServeHTTP(w, r)
			return
		}

		// Записываем пользователя в контекст
		ctx := context.WithValue(r.Context(), UserKey, &user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
