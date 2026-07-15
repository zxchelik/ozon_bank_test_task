package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	StorageMemory   = "memory"
	StoragePostgres = "postgres"
)

type Config struct {
	HTTPAddr         string
	StorageType      string
	PostgresDSN      string
	BaseURL          string
	ShutdownTimeout  time.Duration
	ReadinessTimeout time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr:         env("HTTP_ADDR", ":8080"),
		StorageType:      strings.ToLower(env("STORAGE_TYPE", StorageMemory)),
		PostgresDSN:      os.Getenv("POSTGRES_DSN"),
		BaseURL:          strings.TrimRight(env("BASE_URL", "http://localhost:8080"), "/"),
		ShutdownTimeout:  10 * time.Second,
		ReadinessTimeout: 2 * time.Second,
	}
	var err error
	if cfg.ShutdownTimeout, err = envDuration("SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout); err != nil {
		return nil, err
	}
	if cfg.ReadinessTimeout, err = envDuration("READINESS_TIMEOUT", cfg.ReadinessTimeout); err != nil {
		return nil, err
	}
	if cfg.StorageType != StorageMemory && cfg.StorageType != StoragePostgres {
		return nil, fmt.Errorf("STORAGE_TYPE must be %q or %q", StorageMemory, StoragePostgres)
	}
	if cfg.StorageType == StoragePostgres && strings.TrimSpace(cfg.PostgresDSN) == "" {
		return nil, fmt.Errorf("POSTGRES_DSN is required for postgres storage")
	}
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("BASE_URL must not be empty")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return value, nil
}
