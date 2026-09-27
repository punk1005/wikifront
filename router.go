package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"log"
	//"golang.org/x/crypto/bcrypt"
	"crypto/sha256"
	"os"
	"path/filepath"
	"fmt"	
	"crypto/rand"
	"encoding/hex"
	"github.com/microcosm-cc/bluemonday"
)

var StaticFS embed.FS

func NewRouter(cfg *Config) http.Handler {
	targetURL, err := url.Parse(cfg.APIDB)
	if err != nil {
		panic("Неверный URL для API_DB: " + err.Error())
	}
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	staticFS, _ := fs.Sub(StaticFS, "static")
	fileServer := http.FileServer(http.FS(staticFS))

	// Вспомогательный хелпер для рендеринга html из embed
	serveHTML := func(htmlPath string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			data, err := StaticFS.ReadFile(htmlPath)
			if err != nil {
				http.Error(w, "Шаблон не найден", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(data)
		}
	}

	// Создаем базовый обработчик роутов
	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// 1. ОТДАЧА СТАТИКИ (.js, .css)
		if strings.HasPrefix(path, "/static/") {
			r.URL.Path = strings.TrimPrefix(path, "/static")
			fileServer.ServeHTTP(w, r)
			return
		}

		// 2. ХЕНДЛЕР АВТОРИЗАЦИИ (ПЕРЕХВАТ И МОДИФИКАЦИЯ ЗАПРОСА И ОТВЕТА С SHA256)
		if path == "/api/post/dologin" && r.Method == http.MethodPost {
			reqBytes, err := io.ReadAll(r.Body)
			if err == nil {
				var credentials map[string]string
				if err := json.Unmarshal(reqBytes, &credentials); err == nil {
					plainPassword := credentials["password"]
					
					hashBytes := sha256.Sum256([]byte(plainPassword))
					stableHash := fmt.Sprintf("%x", hashBytes)
					
					credentials["password_hash"] = stableHash
					log.Printf("[AUTH] Сгенерирован стабильный SHA256 хэш для %s: %s", credentials["username"], credentials["password_hash"])
					
					delete(credentials, "password") 
					
					newReqBytes, _ := json.Marshal(credentials)
					r.Body = io.NopCloser(bytes.NewBuffer(newReqBytes))
					r.ContentLength = int64(len(newReqBytes))
				}
			}

			proxy.ModifyResponse = func(resp *http.Response) error {
				if resp.StatusCode == http.StatusOK {
					bodyBytes, err := io.ReadAll(resp.Body)
					if err != nil {
						return err
					}
					resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

					var users []UserInfo
					if err := json.Unmarshal(bodyBytes, &users); err == nil && len(users) > 0 {
						// БЕЗОПАСНО: Создаем новый выделенный объект в куче, чтобы избежать nil pointer
						matchedUser := &UserInfo{
							ID:       users[0].ID,
							Username: users[0].Username,
							Role:     users[0].Role,
						}
						
						token := GlobalSessionManager.CreateSession(matchedUser)

						cookie := &http.Cookie{
							Name:     "session_id",
							Value:    token,
							Path:     "/",
							HttpOnly: true,
							SameSite: http.SameSiteLaxMode,
						}
						resp.Header.Add("Set-Cookie", cookie.String())
					}
				}
				return nil
			}

			r.URL.Path = strings.TrimPrefix(path, "/api")
			proxy.ServeHTTP(w, r)
			return
		}

		// 2.5 ХЕНДЛЕР ВЫХОДА (LOGOUT)
		if path == "/api/post/logout" {
			cookie, err := r.Cookie("session_id")
			if err == nil && cookie.Value != "" {
				GlobalSessionManager.DestroySession(cookie.Value)
			}

			http.SetCookie(w, &http.Cookie{
				Name:     "session_id",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success": true}`))
			return
		}

		// 2.7 ХЕНДЛЕР СОЗДАНИЯ ПОЛЬЗОВАТЕЛЯ АДМИНИСТРАТОРОМ (С ХЭШИРОВАНИЕМ SHA256)
		if path == "/api/post/createuser" && r.Method == http.MethodPost {
			// 1. Проверяем права пользователя из контекста (заполненного в MW_AuthRequired)
			user, _ := r.Context().Value(UserContextKey).(*UserInfo)
			if user == nil || user.Role != "admin" {
				http.Error(w, "403 Forbidden: Доступ разрешен только администраторам", http.StatusForbidden)
				return
			}

			// 2. Читаем и модифицируем входящий JSON (Сырой пароль -> SHA256)
			reqBytes, err := io.ReadAll(r.Body)
			if err == nil {
				var payload map[string]string
				if err := json.Unmarshal(reqBytes, &payload); err == nil {
					plainPassword := payload["password"]
					
					// Хэшируем в стабильный SHA256
					hashBytes := sha256.Sum256([]byte(plainPassword))
					stableHash := fmt.Sprintf("%x", hashBytes)
					
					payload["password_hash"] = stableHash
					delete(payload, "password") 
					
					log.Printf("[ADMIN] Администратор %s создает пользователя %s с ролью %s", user.Username, payload["username"], payload["role"])
					
					// Перезаписываем тело запроса для отправки в wikiapi
					newReqBytes, _ := json.Marshal(payload)
					r.Body = io.NopCloser(bytes.NewBuffer(newReqBytes))
					r.ContentLength = int64(len(newReqBytes))
				}
			}

			// 3. Важно: сбрасываем кастомный ModifyResponse, чтобы он случайно не попытался 
			// распарсить ответ как логин и не сломал структуру данных.
			proxy.ModifyResponse = nil 

			// 4. Отрезаем /api (путь станет /post/createuser)
			r.URL.Path = strings.TrimPrefix(path, "/api")

			// 5. Отправляем запрос в wikiapi. Прокси сам дождется ответа от базы,
			// заберет возвращенный JSON (с ID нового юзера) и полностью перешлиет его на фронтенд.
			proxy.ServeHTTP(w, r)
			return
		}

		// 2.8 ХЕНДЛЕР ПОЛУЧЕНИЯ СПИСКА ПОЛЬЗОВАТЕЛЕЙ (ТОЛЬКО ДЛЯ АДМИНИСТРАТОРА)
		if path == "/api/users" && r.Method == http.MethodGet {
			// Проверяем права из контекста
			user, _ := r.Context().Value(UserContextKey).(*UserInfo)
			if user == nil || user.Role != "admin" {
				http.Error(w, "403 Forbidden: Доступ запрещен", http.StatusForbidden)
				return
			}

			// Сбрасываем кастомный ModifyResponse, так как нам нужна чистая трансляция данных
			proxy.ModifyResponse = nil 

			// Отрезаем /api (путь превратится в GET /users для wikiapi)
			r.URL.Path = strings.TrimPrefix(path, "/api")

			// Проксирует запрос, дожидается ответа от wikiapi и отдает JSON пользователей на фронтенд
			proxy.ServeHTTP(w, r)
			return
		}

		// 2.9 ХЕНДЛЕР СМЕНЫ РОЛИ ПОЛЬЗОВАТЕЛЯ (ТОЛЬКО ДЛЯ АДМИНИСТРАТОРА)
		if path == "/api/post/update_user_role" && r.Method == http.MethodPost {
			// Проверяем права из контекста
			user, _ := r.Context().Value(UserContextKey).(*UserInfo)
			if user == nil || user.Role != "admin" {
				http.Error(w, "403 Forbidden: Доступ разрешен только администраторам", http.StatusForbidden)
				return
			}

			// Сбрасываем кастомный ModifyResponse ради чистой трансляции ответа
			proxy.ModifyResponse = nil 

			// Отрезаем /api (путь превратится в POST /post/updateuserrole для wikiapi)
			r.URL.Path = strings.TrimPrefix(path, "/api")

			// Проксируем запрос в базу знаний
			proxy.ServeHTTP(w, r)
			return
		}
		// 2.9.1 ХЭНДЛЕР ИЗМЕНЕНИЯ ПРОФИЛЯ ПОЛЬЗОВАТЕЛЯ
		if path == "/api/post/update_user_profile" && r.Method == http.MethodPost {
			user, _ := r.Context().Value(UserContextKey).(*UserInfo)
			if user == nil {
				http.Error(w, "403 Forbidden: Отсутствует авторизация", http.StatusForbidden)
				return
			}

			// 1. Читаем сырые байты JSON, пришедшие от фронтенда
			reqBytes, err := io.ReadAll(r.Body)
			if err != nil {
				log.Printf("[ERROR] Не удалось прочитать тело запроса: %v", err)
				http.Error(w, "400 Bad Request", http.StatusBadRequest)
				return
			}

			// Выводим в лог сервера то, что РЕАЛЬНО прислал фронтенд
			log.Printf("[PROFILE LOG] Входящий сырой JSON от фронта: %s", string(reqBytes))

			var payload map[string]interface{}
			if err := json.Unmarshal(reqBytes, &payload); err != nil {
				log.Printf("[ERROR] Ошибка unmarshal во WIKIFRONT: %v", err)
				http.Error(w, "400 Bad Request", http.StatusBadRequest)
				return
			}

			// Выводим в лог распарсенную мапу, чтобы убедиться в наличии user_id
			log.Printf("[PROFILE LOG] Распарсенная мапа: %+v", payload)

			// 2. Извлекаем ID целевого пользователя для проверки прав
			targetUserIDFloat, ok := payload["user_id"].(float64)
			if !ok {
				log.Printf("[WARNING] Ключ 'user_id' отсутствует в JSON или имеет неверный тип данных!")
				http.Error(w, "400 Bad Request: Отсутствует user_id", http.StatusBadRequest)
				return
			}
			targetUserID := int(targetUserIDFloat)

			// 3. ПРОВЕРКА РОЛЕЙ: Обычный юзер может править только свой ID, Админ — любой
			if user.Role != "admin" && user.ID != targetUserID {
				log.Printf("[SECURITY] Пользователь %s (ID: %d) пытался изменить чужой профиль (ID: %d)", user.Username, user.ID, targetUserID)
				http.Error(w, "403 Forbidden: Вы можете редактировать только свой профиль", http.StatusForbidden)
				return
			}

			log.Printf("[PROFILE] Пользователь %s успешно прошел валидацию для изменения профиля ID: %d", user.Username, targetUserID)

			// Восстанавливаем тело запроса в исходном чистом виде для передачи в wikiapi
			r.Body = io.NopCloser(bytes.NewBuffer(reqBytes))
			r.ContentLength = int64(len(reqBytes))

			// 4. Чистый прокси-транзит
			proxy.ModifyResponse = nil 
			r.URL.Path = strings.TrimPrefix(path, "/api") // Путь превратится в POST /post/update_user_profile
			proxy.ServeHTTP(w, r)
			return
		}
		// 2.9.2 ХЭНДЛЕР ПОЛУЧЕНИЯ ПРОФИЛЯ ПОЛЬЗОВАТЕЛЯ
		if path == "/api/get/my_profile" && r.Method == http.MethodGet {
			user, _ := r.Context().Value(UserContextKey).(*UserInfo)
			if user == nil {
				http.Error(w, "401 Unauthorized", http.StatusUnauthorized)
				return
			}

			// Отрезаем префикс /api для отправки в wikiapi
			r.URL.Path = strings.TrimPrefix(path, "/api")
			
			// Безопасно формируем параметры строки запроса (?user_id=...)
			q := r.URL.Query()
			q.Set("user_id", fmt.Sprintf("%d", user.ID))
			r.URL.RawQuery = q.Encode()

			// Проксируем запрос в wikiapi
			proxy.ModifyResponse = nil 
			proxy.ServeHTTP(w, r)
			return
		}

		// 2.10 ХЕНДЛЕР ПОЛУЧЕНИЯ СПИСКА ПАПОК (ДЛЯ АДМИНА И МОДЕРАТОРА)
		if path == "/api/get/folders" && r.Method == http.MethodGet {
			user, _ := r.Context().Value(UserContextKey).(*UserInfo)
			if user == nil {
				http.Error(w, "403 Forbidden: Недостаточно прав", http.StatusForbidden)
				return
			}

			proxy.ModifyResponse = nil 
			r.URL.Path = strings.TrimPrefix(path, "/api")
			proxy.ServeHTTP(w, r)
			return
		}

		// 2.11 ХЕНДЛЕР СОЗДАНИЯ ПАПКИ (С АВТОПОДСТАНОВКОЙ ID АВТОРА ИЗ СЕССИИ)
		if path == "/api/post/create_folder" && r.Method == http.MethodPost {
			user, _ := r.Context().Value(UserContextKey).(*UserInfo)
			// ИСКЛЮЧИЛИ WRITER: теперь только admin и moderator
			if user == nil || (user.Role != "admin" && user.Role != "moderator") {
				http.Error(w, "403 Forbidden: Создавать разделы могут только администраторы и модераторы", http.StatusForbidden)
				return
			}

			// Читаем JSON от фронтенда (там придут только name и slug)
			reqBytes, err := io.ReadAll(r.Body)
			if err == nil {
				var payload map[string]interface{}
				if err := json.Unmarshal(reqBytes, &payload); err == nil {
					// Автоматически подставляем ID авторизованного админа/модератора из сессии Go!
					payload["created_by"] = user.ID
					
					log.Printf("[ADMIN/MOD] %s создает новую тематику: %s (slug: %v)", user.Username, payload["name"], payload["slug"])
					
					newReqBytes, _ := json.Marshal(payload)
					r.Body = io.NopCloser(bytes.NewBuffer(newReqBytes))
					r.ContentLength = int64(len(newReqBytes))
				}
			}

			proxy.ModifyResponse = nil 
			r.URL.Path = strings.TrimPrefix(path, "/api")
			proxy.ServeHTTP(w, r)
			return
		}

		// 2.12 ХЕНДЛЕР СОЗДАНИЯ СТАТЬИ (ДЛЯ ВСЕХ КРОМЕ READER)
		if path == "/api/post/create_article" && r.Method == http.MethodPost {
			user, _ := r.Context().Value(UserContextKey).(*UserInfo)
			// Жестко блокируем роль 'reader'
			if user == nil || user.Role == "reader" {
				http.Error(w, "403 Forbidden: Роль 'Reader' не может создавать статьи", http.StatusForbidden)
				return
			}

			reqBytes, err := io.ReadAll(r.Body)
			if err == nil {
				var payload map[string]interface{}
				if err := json.Unmarshal(reqBytes, &payload); err == nil {
					// Автоматически внедряем ID автора из активной Go-сессии
					payload["author_id"] = user.ID
					
					log.Printf("[ARTICLE] Пользователь %s создает статью: %s", user.Username, payload["title"])
					
					newReqBytes, _ := json.Marshal(payload)
					r.Body = io.NopCloser(bytes.NewBuffer(newReqBytes))
					r.ContentLength = int64(len(newReqBytes))
				}
			}

			proxy.ModifyResponse = nil 
			r.URL.Path = strings.TrimPrefix(path, "/api")
			proxy.ServeHTTP(w, r)
			return
		}

		// 2.13 ХЕНДЛЕР ПОЛУЧЕНИЯ ДАННЫХ СТАТЬИ ПО СЛАГУ (ДОСТУПНО ВСЕМ РОЛЯМ)
		if path == "/api/get/article" && r.Method == http.MethodGet {
			// Любой авторизованный пользователь (прошедший глобальную MW_AuthRequired) имеет доступ
			proxy.ModifyResponse = nil 
			r.URL.Path = strings.TrimPrefix(path, "/api")
			proxy.ServeHTTP(w, r)
			return
		}

		// 2.14 ХЕНДЛЕР ПОЛУЧЕНИЯ ДАННЫХ СТАТЬИ ПО СЛАГУ (ДОСТУПНО ВСЕМ РОЛЯМ)
		if path == "/api/get/articlesnew" && r.Method == http.MethodGet {
			// Любой авторизованный пользователь (прошедший глобальную MW_AuthRequired) имеет доступ
			proxy.ModifyResponse = nil 
			r.URL.Path = strings.TrimPrefix(path, "/api")
			proxy.ServeHTTP(w, r)
			return
		}
		// 2.15 ХЭНДЛЕР ПОЛУЧЕНИЯ ДАННЫХ СОДЕРЖИМОЕ ПАПКИ
		if path == "/api/get/folder_content" && r.Method == http.MethodGet {
			// Получаем slug из параметров запроса (?slug=...)
			slug := r.URL.Query().Get("slug")
			if slug == "" {
				http.Error(w, "400 Bad Request: Отсутствует параметр slug", http.StatusBadRequest)
				return
			}

			// Сбрасываем кастомный ModifyResponse для чистой трансляции данных
			proxy.ModifyResponse = nil 

			// Отрезаем префикс /api, оставляя /get/folder_content?slug=...
			r.URL.Path = strings.TrimPrefix(path, "/api")

			// Проксируем запрос в wikiapi
			proxy.ServeHTTP(w, r)
			return
		}

		// 2.16 ПОЛУЧЕНИЕ БЛОКОВ СТАТЬИ ДЛЯ РЕДАКТОРА
		if path == "/api/get/article_blocks" && r.Method == http.MethodGet {
			proxy.ModifyResponse = nil 
			r.URL.Path = strings.TrimPrefix(path, "/api")
			proxy.ServeHTTP(w, r)
			return
		}

		// 2.17 ПЕРЕЗАПИСЬ/СОХРАНЕНИЕ БЛОКОВ СТАТЬИ (С ИСПРАВЛЕНИЕМ ПОД СТРУКТУРУ МАРЫ WIKIAPI)
		if path == "/api/post/save_article_blocks" && r.Method == http.MethodPost {
			user, _ := r.Context().Value(UserContextKey).(*UserInfo)
			if user == nil || user.Role == "reader" {
				http.Error(w, "403 Forbidden: Недостаточно прав", http.StatusForbidden)
				return
			}

			reqBytes, err := io.ReadAll(r.Body)
			if err == nil {
				var blocksArray []map[string]interface{}
				if err := json.Unmarshal(reqBytes, &blocksArray); err == nil && len(blocksArray) > 0 {
					
					articleSlug, _ := blocksArray[0]["article_slug"].(string)
					articleID := blocksArray[0]["article_id"]

					if !isUserAllowedToModifyArticle(user, articleSlug, Cfg.APIDB) {
						http.Error(w, "403 Forbidden: Вы можете редактировать только собственные статьи", http.StatusForbidden)
						return
					}

					// ИНИЦИАЛИЗИРУЕМ САНИТАРНЫЙ ФИЛЬТР HTML
					// UGCPolicy разрешает базовое форматирование (ссылки, абзацы, списки), 
					// но намертво вырезает <script>, <iframe>, onclick, onerror и javascript: ссылки.
					p := bluemonday.UGCPolicy()

					// Бежим циклом по всем блокам, которые прислал Quill
					for _, block := range blocksArray {
						delete(block, "article_slug") // Удаляем наш технический параметр

						// Достаем сырой HTML контент из блока текста
						if rawContent, ok := block["content"].(string); ok {
							// ОЧИЩАЕМ HTML НА ЛЕТУ ПЕРЕД ЗАПИСЬЮ В БД!
							sanitizedContent := p.Sanitize(rawContent)
							
							// Записываем очищенный безопасный HTML обратно в мапу
							block["content"] = sanitizedContent
						}
					}

					// Пересобираем JSON с уже очищенными и безопасными блоками
					jsonArrayBytes, _ := json.Marshal(blocksArray)
					wrappedPayload := map[string]interface{}{
						"article_id":  articleID,
						"blocks_json": string(jsonArrayBytes),
					}

					newReqBytes, _ := json.Marshal(wrappedPayload)
					r.Body = io.NopCloser(bytes.NewBuffer(newReqBytes))
					r.ContentLength = int64(len(newReqBytes))
				}
			}

			proxy.ModifyResponse = nil 
			r.URL.Path = strings.TrimPrefix(path, "/api")
			proxy.ServeHTTP(w, r)
			return
		}

		// 2.18 МЯГКОЕ УДАЛЕНИЕ СТАТЬИ
		if path == "/api/post/pending_delete_article" && r.Method == http.MethodPost {
			user, _ := r.Context().Value(UserContextKey).(*UserInfo)
			if user == nil || user.Role == "reader" {
				http.Error(w, "403 Forbidden", http.StatusForbidden)
				return
			}

			reqBytes, err := io.ReadAll(r.Body)
			if err == nil {
				var payload map[string]string
				if err := json.Unmarshal(reqBytes, &payload); err == nil {
					slug := payload["slug"]

					// Проверяем, имеет ли право данный юзер удалять её (свой-чужой)
					if !isUserAllowedToModifyArticle(user, slug, Cfg.APIDB) {
						http.Error(w, "403 Forbidden: Вы можете удалять только свои статьи", http.StatusForbidden)
						return
					}

					// Добавляем флаги для dologin-подобного исполнения SQL
					isStaff := (user.Role == "admin" || user.Role == "moderator")
					payload["user_id"] = fmt.Sprintf("%d", user.ID)
					payload["is_staff"] = fmt.Sprintf("%t", isStaff)

					newReqBytes, _ := json.Marshal(payload)
					r.Body = io.NopCloser(bytes.NewBuffer(newReqBytes))
					r.ContentLength = int64(len(newReqBytes))
				}
			}

			proxy.ModifyResponse = nil
			r.URL.Path = strings.TrimPrefix(path, "/api")
			proxy.ServeHTTP(w, r)
			return
		}


		//
		//   START FILES POINTS
		//
		// 2.100.1 ПОЛУЧЕНИЕ СПИСКА ФАЙЛОВ СТАТЬИ
		if path == "/api/get/article_files" && r.Method == http.MethodGet {
			proxy.ModifyResponse = nil 
			r.URL.Path = strings.TrimPrefix(path, "/api")
			proxy.ServeHTTP(w, r)
			return
		}

		// 2.100.2 УДАЛЕНИЕ ФАЙЛА ИЗ СТАТЬИ
		if path == "/api/post/delete_article_files" && r.Method == http.MethodPost {
			user, _ := r.Context().Value(UserContextKey).(*UserInfo)
			if user == nil || user.Role == "reader" {
				http.Error(w, "403 Forbidden: Недостаточно прав", http.StatusForbidden)
				return
			}

			// Читаем JSON запроса от фронтенда (там только id и article_id)
			reqBytes, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Ошибка чтения тела запроса", http.StatusBadRequest)
				return
			}

			var payload map[string]interface{}
			if err := json.Unmarshal(reqBytes, &payload); err == nil {
				fileID := payload["id"]
				articleID := payload["article_id"]

				// 1. ДЕЛАЕМ ВНУТРЕННИЙ ЗАПРОС К WIKIAPI, ЧТОБЫ УЗНАТЬ ИМЯ ФАЙЛА ПО ID
				apiURL := fmt.Sprintf("%s/get/file_info?id=%v&article_id=%v", Cfg.APIDB, fileID, articleID)
				apiResp, err := http.Get(apiURL)
				
				if err == nil && apiResp.StatusCode == http.StatusOK {
					apiBody, _ := io.ReadAll(apiResp.Body)
					apiResp.Body.Close()

					// Парсим ответ от базы знаний (массив строк)
					var filesResult []map[string]interface{}
					if err := json.Unmarshal(apiBody, &filesResult); err == nil && len(filesResult) > 0 {
						
						// Достаем оригинальное захешированное имя файла на диске
						if fileName, ok := filesResult[0]["file_name"].(string); ok && fileName != "" {
							safeFileName := filepath.Base(fileName)
							filePath := filepath.Join("./uploads", safeFileName)

							// 2. ФИЗИЧЕСКИ УДАЛЯЕМ ФАЙЛ С ДИСКА СЕРВЕРА
							if _, errStat := os.Stat(filePath); errStat == nil {
								if errRemove := os.Remove(filePath); errRemove != nil {
									log.Printf("[ERROR] Не удалось удалить физический файл %s: %v", filePath, errRemove)
								} else {
									log.Printf("[FILE] Файл %s успешно удален с диска сервером WIKIFRONT", safeFileName)
								}
							}
						}
					}
				}

				// Возвращаем исходное тело запроса (с id и article_id) обратно в поток для wikiapi
				r.Body = io.NopCloser(bytes.NewBuffer(reqBytes))
				r.ContentLength = int64(len(reqBytes))
			}

			// 3. ОТПРАВЛЯЕМ ЗАПРОС В WIKIAPI ДЛЯ УДАЛЕНИЯ СТРОКИ ИЗ БАЗЫ ДАННЫХ
			proxy.ModifyResponse = nil 
			r.URL.Path = strings.TrimPrefix(path, "/api")
			proxy.ServeHTTP(w, r)
			return
		}

		// 2.100.2 ФИЗИЧЕСКАЯ ЗАГРУЗКА ФАЙЛА НА ДИСК И ПРИВЯЗКА К БД через wikiapi
		if path == "/api/post/upload_file" && r.Method == http.MethodPost {
			user, _ := r.Context().Value(UserContextKey).(*UserInfo)
			if user == nil || user.Role == "reader" {
				http.Error(w, "403 Forbidden: Читатели не могут загружать файлы", http.StatusForbidden)
				return
			}

			// Ограничиваем максимальный размер файла, например, 20 МБ
			r.ParseMultipartForm(20 << 20)

			file, handler, err := r.FormFile("file")
			if err != nil {
				http.Error(w, "Ошибка чтения файла из запроса", http.StatusBadRequest)
				return
			}
			defer file.Close()

			articleID := r.FormValue("article_id")
			if articleID == "" {
				http.Error(w, "Отсутствует обязательный параметр article_id", http.StatusBadRequest)
				return
			}

			// Генерируем уникальное имя файла на сервере, чтобы избежать перезаписи (например: sha256 от времени + имени)
			ext := filepath.Ext(handler.Filename)
			randomBytes := make([]byte, 16)
			rand.Read(randomBytes)
			uniqueName := hex.EncodeToString(randomBytes) + ext

			// Путь куда физически сохраняем файл. 
			// Лучше всего создать папку "uploads" в корне вашего сервера рядом с бинарником wikifront
			uploadDir := "./uploads"
									
			targetPath := filepath.Join(uploadDir, uniqueName)

			// Создаем и открываем файл на запись
			dst, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
			if err != nil {
				http.Error(w, "Не удалось сохранить файл на диске сервера", http.StatusInternalServerError)
				return
			}
			defer dst.Close()

			if _, err := io.Copy(dst, file); err != nil {
				http.Error(w, "Ошибка записи файла", http.StatusInternalServerError)
				return
			}

			os.Chmod(targetPath, 0666)
		
			// Файл успешно сохранен на сервере! Теперь шлем метаданные в wikiapi, чтобы сделать запись в базу данных
			payload := map[string]interface{}{
				"article_id":    articleID,
				"file_name":     uniqueName,
				"original_name": handler.Filename,
			}

			jsonBytes, _ := json.Marshal(payload)
			
			// Делаем ручной внутренний POST-запрос к wikiapi эндпоинту /post/save_article_file
			apiURL := fmt.Sprintf("%s/post/save_article_file", Cfg.APIDB)
			apiReq, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewBuffer(jsonBytes))
			if err == nil {
				apiReq.Header.Set("Content-Type", "application/json")
				client := &http.Client{}
				apiResp, err := client.Do(apiReq)
				if err == nil && apiResp.StatusCode == http.StatusOK {
					// Пересылаем успешный ответ фронтенду
					w.Header().Set("Content-Type", "application/json")
					io.Copy(w, apiResp.Body)
					return
				}
			}

			http.Error(w, "Файл сохранен, но не удалось сделать запись в базу знаний через wikiapi", http.StatusInternalServerError)
			return
		}

		// 2.200.1 ПОЛУЧЕНИЕ СПИСКА СТАТЕЙ, ПОМЕЧЕННЫХ НА УДАЛЕНИЕ (ДЛЯ АДМИНА И МОДЕРАТОРА)
		if path == "/api/get/deleted_articles" && r.Method == http.MethodGet {
			user, _ := r.Context().Value(UserContextKey).(*UserInfo)
			if user == nil || (user.Role != "admin" && user.Role != "moderator") {
				http.Error(w, "403 Forbidden: Недостаточно прав", http.StatusForbidden)
				return
			}
			proxy.ModifyResponse = nil 
			r.URL.Path = strings.TrimPrefix(path, "/api")
			proxy.ServeHTTP(w, r)
			return
		}

		// 2.200.2 ПОЛНОЕ УДАЛЕНИЕ СТАТЬИ ИЗ СИСТЕМЫ (ТОЛЬКО ДЛЯ АДМИНА И МОДЕРАТОРА)
		if path == "/api/post/hard_delete_article" && r.Method == http.MethodPost {
			user, _ := r.Context().Value(UserContextKey).(*UserInfo)
			if user == nil || (user.Role != "admin" && user.Role != "moderator") {
				http.Error(w, "403 Forbidden: Недостаточно прав", http.StatusForbidden)
				return
			}

			reqBytes, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "400 Bad Request", http.StatusBadRequest)
				return
			}

			var payload map[string]interface{}
			if err := json.Unmarshal(reqBytes, &payload); err == nil {
				articleIDFloat, ok := payload["id"].(float64)
				if ok {
					articleID := int(articleIDFloat)

					// === ИСПОЛЬЗУЕМ ВАШ СУЩЕСТВУЮЩИЙ ЭНДПОИНТ ДЛЯ ПОЛУЧЕНИЯ ФАЙЛОВ ===
					apiURL := fmt.Sprintf("%s/get/article_files?article_id=%d", Cfg.APIDB, articleID)
					apiResp, errGet := http.Get(apiURL)
					
					if errGet == nil && apiResp.StatusCode == http.StatusOK {
						apiBody, _ := io.ReadAll(apiResp.Body)
						apiResp.Body.Close()

						var filesResult []map[string]interface{}
						if errJson := json.Unmarshal(apiBody, &filesResult); errJson == nil && len(filesResult) > 0 {
							
							// ЦИКЛОМ УДАЛЯЕМ ФАЙЛЫ С ДИСКА
							for _, fileMap := range filesResult {
								if fileName, exists := fileMap["file_name"].(string); exists && fileName != "" {
									safeFileName := filepath.Base(fileName)
									filePath := filepath.Join("./uploads", safeFileName)

									if _, errStat := os.Stat(filePath); errStat == nil {
										errRemove := os.Remove(filePath)
										if errRemove != nil {
											log.Printf("[ERROR] Не удалось удалить файл %s: %v", safeFileName, errRemove)
										} else {
											log.Printf("[FILE CLEANUP] Файл %s успешно удален с диска перед уничтожением статьи ID %d", safeFileName, articleID)
										}
									}
								}
							}
						}
					}
				}
			}

			log.Printf("[ADMIN] %s производит ПОЛНОЕ УДАЛЕНИЕ статьи ID %v из системы", user.Username, payload["id"])

			// Восстанавливаем тело запроса и проксируем команду удаления строки из БД
			r.Body = io.NopCloser(bytes.NewBuffer(reqBytes))
			r.ContentLength = int64(len(reqBytes))

			proxy.ModifyResponse = nil 
			r.URL.Path = strings.TrimPrefix(path, "/api")
			proxy.ServeHTTP(w, r)
			return
		}

		// 2.200.3 ВОССТАНОВЛЕНИЕ СТАТЬИ ИЗ БЭКЛОГА (ТОЛЬКО ДЛЯ АДМИНА И МОДЕРАТОРА)
		if path == "/api/post/restore_article" && r.Method == http.MethodPost {
			user, _ := r.Context().Value(UserContextKey).(*UserInfo)
			if user == nil || (user.Role != "admin" && user.Role != "moderator") {
				http.Error(w, "403 Forbidden: Недостаточно прав", http.StatusForbidden)
				return
			}
			proxy.ModifyResponse = nil 
			r.URL.Path = strings.TrimPrefix(path, "/api")
			proxy.ServeHTTP(w, r)
			return
		}

		// 2.300.1 ГЛОБАЛЬНЫЙ ПОИСК ПО БАЗЕ ЗНАНИЙ (ДОСТУПЕН ВСЕМ АВТОРИЗОВАННЫМ)
		if path == "/api/get/search_articles" && r.Method == http.MethodGet {
			// Достаточно общей авторизации от глобальной middleware
			proxy.ModifyResponse = nil 
			r.URL.Path = strings.TrimPrefix(path, "/api")
			proxy.ServeHTTP(w, r)
			return
		}


		// 
		//    END FILES POINTS
		//

		// 3. ОБЩИЙ РЕВЕРС-ПРОКСИ ДЛЯ ОСТАЛЬНЫХ API-ЭНДПОИНТОВ
		if strings.HasPrefix(path, "/api/") {
			// Сбрасываем кастомный перехватчик для остальных запросов к API
			proxy.ModifyResponse = nil
			r.URL.Path = strings.TrimPrefix(path, "/api")
			proxy.ServeHTTP(w, r)
			return
		}

		// 4. РОУТИНГ HTML СТРАНИЦ
		switch {
		case path == "/login":
			MW_Logger(serveHTML("static/pages/login/index.html")).ServeHTTP(w, r)

		case path == "/":
			MW_Logger(MW_Root(serveHTML("static/pages/index.html"))).ServeHTTP(w, r)

		case strings.HasPrefix(path, "/admin"):
			MW_Logger(MW_Admin(serveHTML("static/pages/admin/index.html"))).ServeHTTP(w, r)

		case strings.HasPrefix(path, "/article/"):
			MW_Logger(MW_Article(serveHTML("static/pages/article/index.html"))).ServeHTTP(w, r)

		case strings.HasPrefix(path, "/editarticle/"):
			editArticleCase(w, r, path, serveHTML)
		
		case strings.HasPrefix(path, "/folder/"):
			// Отдаем тот же шаблон папки (фронтенд сам заберет slug из URL)
			MW_Logger(MW_Article(serveHTML("static/pages/folder/index.html"))).ServeHTTP(w, r)			
	
		default:
			MW_Logger(serveHTML("static/pages/index.html")).ServeHTTP(w, r)
		}
	})

	// Оборачиваем весь наш роутер в глобальную middleware проверки сессий
	return MW_AuthRequired(mux)
}

// Проверяет, может ли пользователь редактировать/удалять статью по ее слагу
func isUserAllowedToModifyArticle(user *UserInfo, articleSlug string, apiDB string) bool {
	// Администраторы и модераторы могут править ВСЁ
	if user.Role == "admin" || user.Role == "moderator" {
		return true
	}
	// Читатели не могут править ничего
	if user.Role == "reader" {
		return false
	}

	// Если пользователь - writer, нужно проверить авторство статьи через wikiapi
	apiURL := fmt.Sprintf("%s/get/article_author?slug=%s", apiDB, articleSlug)
	resp, err := http.Get(apiURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	var articles []map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &articles); err == nil && len(articles) > 0 {
		// Приводим author_id из float64 (стандарт для json чисел) к int
		if authorIDFloat, ok := articles[0]["author_id"].(float64); ok {
			return int(authorIDFloat) == user.ID
		}
	}

	return false
}

// editArticleCase обрабатывает роутинг страницы редактирования статьи с проверкой прав доступа
func editArticleCase(w http.ResponseWriter, r *http.Request, path string, serveHTML func(string) http.HandlerFunc) {
	// 1. Вырезаем slug статьи из URL
	slug := strings.TrimPrefix(path, "/editarticle/")
	slug = strings.TrimSuffix(slug, "/") // Очищаем слэш на конце, если он есть

	// 2. Достаем пользователя из контекста, который заполнила MW_AuthRequired
	user, _ := r.Context().Value(UserContextKey).(*UserInfo)
	
	// 3. Жесткий фильтр ролей: гость или reader не пройдут
	if user == nil || user.Role == "reader" {
		http.Error(w, "403 Forbidden: Недостаточно прав", http.StatusForbidden)
		return
	}

	// 4. Проверка авторства статьи ДО отдачи HTML страницы
	if !isUserAllowedToModifyArticle(user, slug, Cfg.APIDB) {
		log.Printf("[SECURITY] Пользователь %s (роль %s) заблокирован при попытке открыть чужой редактор: /editarticle/%s", user.Username, user.Role, slug)
		http.Error(w, "403 Forbidden: Вы можете редактировать только собственные статьи", http.StatusForbidden)
		return
	}

	// 5. Если проверка пройдена, оборачиваем в MW-заглушки и отдаем фронтенд
	MW_Logger(MW_EditArticle(serveHTML("static/pages/editarticle/index.html"))).ServeHTTP(w, r)
}