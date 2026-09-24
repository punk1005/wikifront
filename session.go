package main

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

// Информация о пользователе, которая хранится в сессии
type UserInfo struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"` // 'reader', 'writer', 'moderator', 'admin'
}

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*UserInfo
}

var GlobalSessionManager = &SessionManager{
	sessions: make(map[string]*UserInfo),
}

// Генерация случайного безопасного токена сессии
func generateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// Создание сессии и сохранение в мапу
func (sm *SessionManager) CreateSession(user *UserInfo) string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	token := generateToken()
	sm.sessions[token] = user
	return token
}

// Получение данных пользователя по токену сессии
func (sm *SessionManager) GetSession(token string) (*UserInfo, bool) {
	if token == "" {
		return nil, false
	}
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	user, exists := sm.sessions[token]
	return user, exists
}

// Удаление сессии (для Logout)
func (sm *SessionManager) DestroySession(token string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, token)
}
