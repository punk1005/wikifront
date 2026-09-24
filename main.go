package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"
)

//go:embed static/*
var staticEmbedFS embed.FS

func main() {
	StaticFS = staticEmbedFS
	Cfg = LoadConfig()
	if Cfg == nil {
		log.Fatalf("Критическая ошибка: Не удалось загрузить конфигурацию")
	}

	router := NewRouter(Cfg)

	addr := fmt.Sprintf(":%s", Cfg.Port)
	log.Printf("Сервер WIKIFRONT успешно запущен на порту %s", Cfg.Port)
	log.Printf("Проксирование API настроено на: %s", Cfg.APIDB)

	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("Ошибка запуска сервера: %v", err)
	}
}
