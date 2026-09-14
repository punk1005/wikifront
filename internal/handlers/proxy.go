package handlers

import (
	"io"
	"net/http"
	"strings"
	"wikifront/internal/model"
)

// ProxyToAPI прозрачно перенаправляет запросы с фронта на бэкенд wikiapi
func ProxyToAPI(w http.ResponseWriter, r *http.Request, apiBaseURL string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Метод не поддерживается прокси", http.StatusMethodNotAllowed)
		return
	}

	apiPath := strings.TrimPrefix(r.URL.Path, "/api")
	targetURL := apiBaseURL + apiPath

	proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, "Ошибка формирования прокси-запроса", http.StatusInternalServerError)
		return
	}

	for key, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}
	
	// Передаем данные пользователя в заголовках, если он авторизован
	// Используем строковый литерал "user", так как пакет handlers не видит contextKey из middleware
	if userVal := r.Context().Value("user"); userVal != nil {
		user := userVal.(*model.User)
		proxyReq.Header.Set("X-User-Role", user.Role)
	}

	client := &http.Client{}
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, "Бэкенд wikiapi недоступен", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
