package middleware

import (	
	"net/http"
	"wikifront/internal/model"
)

// RequireRole не дает пройти дальше, если у пользователя нет минимально необходимых прав
func RequireRole(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			val := r.Context().Value("user")
			if val == nil {
				http.Error(w, "Требуется авторизация", http.StatusUnauthorized)
				return
			}
			
			user := val.(*model.User)
			
			// Проверяем, есть ли роль пользователя в списке разрешенных
			hasAccess := false
			for _, role := range allowedRoles {
				if user.Role == role {
					hasAccess = true
					break
				}
			}

			if !hasAccess {
				http.Error(w, "Доступ запрещен: недостаточно прав", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
