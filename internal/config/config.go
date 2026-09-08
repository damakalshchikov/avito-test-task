// Package config загружает конфигурацию сервиса kitchen из переменных окружения
package config

import (
	"fmt"
	"os"
	"time"
)

// Config реализует конфигурацию основного сервиса
type Config struct {
	HTTPAddr           string
	DatabaseURL        string
	AdminToken         string
	LogLevel           string
	OutboxPollInterval time.Duration
	ShutdownTimeout    time.Duration
}

// Load читает конфигурацию из переменных окружения с префиксом KITCHEN_
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:        getEnv("KITCHEN_HTTP_ADDR", ":8080"),
		DatabaseURL:     os.Getenv("KITCHEN_DATABASE_URL"),
		AdminToken:      os.Getenv("KITCHEN_ADMIN_TOKEN"),
		LogLevel:        getEnv("KITCHEN_LOG_LEVEL", "info"),
		ShutdownTimeout: 10 * time.Second,
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("KITCHEN_DATABASE_URL is required")
	}
	if cfg.AdminToken == "" {
		return Config{}, fmt.Errorf("KITCHEN_ADMIN_TOKEN is required")
	}

	interval, err := time.ParseDuration(getEnv("KITCHEN_OUTBOX_POLL_INTERVAL", "1s"))
	if err != nil {
		return Config{}, fmt.Errorf("parse KITCHEN_OUTBOX_POLL_INTERVAL: %w", err)
	}
	cfg.OutboxPollInterval = interval

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
