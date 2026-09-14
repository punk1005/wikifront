package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"wikifront/internal/model"
)

type APIClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Login выполняет запрос авторизации к wikiapi
func (c *APIClient) Login(username, password string) (model.User, error) {
	// В будущем здесь будет POST запрос к wikiapi
	// Пока для тестов возвращаем мок-пользователя admin
	return model.User{ID: 1, Username: username, Role: model.RoleAdmin}, nil
}

// GetArticlesByFolder возвращает список статей в папке
func (c *APIClient) GetArticlesByFolder(folderSlug string) ([]model.Article, error) {
	// В будущем запрос к wikiapi
	return []model.Article{}, nil
}

// GetPendingDeleteArticles возвращает статьи на удаление
func (c *APIClient) GetPendingDeleteArticles() ([]model.Article, error) {
	// В будущем запрос к wikiapi
	return []model.Article{}, nil
}

// GetArticleBySlug запрашивает статью.
// Так как твой wikiapi работает через /get/filename, мы передаем slug как имя файла.
func (c *APIClient) GetArticleBySlug(slug string) (model.Article, []model.ArticleBlock, []model.ArticleFile, error) {
	url := fmt.Sprintf("%s/get/%s", c.baseURL, slug)
	
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return model.Article{}, nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return model.Article{}, nil, nil, fmt.Errorf("api returned status: %d", resp.StatusCode)
	}

	// Ожидаем, что wikiapi вернет composite-объект
	var result struct {
		Article model.Article        `json:"article"`
		Blocks  []model.ArticleBlock `json:"blocks"`
		Files   []model.ArticleFile  `json:"files"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return model.Article{}, nil, nil, err
	}

	return result.Article, result.Blocks, result.Files, nil
}

// GetFolders запрашивает список папок (например, для выпадающего списка в редакторе)
func (c *APIClient) GetFolders() ([]model.Folder, error) {
	url := fmt.Sprintf("%s/get/folders_list", c.baseURL)
	
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api returned status: %d", resp.StatusCode)
	}

	var folders []model.Folder
	if err := json.NewDecoder(resp.Body).Decode(&folders); err != nil {
		return nil, err
	}

	return folders, nil
}
