package config

import "os"

type Config struct {
	Port       string // Порт для wikifront (например, ":3000")
	APIBaseURL string // URL бэкенда wikiapi (например, "http://localhost:8080")
}

func Load() *Config {
	port := os.Getenv("WIKIFRONT_PORT")
	if port == "" {
		port = ":8051"
	}

	apiURL := os.Getenv("WIKIAPI_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8055"
	}

	return &Config{
		Port:       port,
		APIBaseURL: apiURL,
	}
}
