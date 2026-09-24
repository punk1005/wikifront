package main

import (
	"bufio"
	"os"
	"strings"
)

// Объявляем глобальную переменную для всего проекта
var Cfg *Config

type Config struct {
	Port  string
	APIDB string
	AppPrefix string
}

func LoadConfig() *Config {
	cfg := &Config{
		Port:  "8050",
		APIDB: "http://punk-wikiapi:8051",
		AppPrefix: "",
	}

	file, err := os.Open(".env")
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				switch key {
				case "PORT":
					cfg.Port = val
				case "API_DB":
					cfg.APIDB = val
				case "APP_PREFIX":
				    cfg.AppPrefix = val	
				}
			}
		}
	}

	if envPort := os.Getenv("PORT"); envPort != "" {
		cfg.Port = envPort
	}
	if envAPI := os.Getenv("API_DB"); envAPI != "" {
		cfg.APIDB = envAPI
	}

	return cfg
}
