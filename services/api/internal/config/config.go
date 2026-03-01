package config

import (
	"fmt"
	"os"
)

type Config struct {
	DatabaseURL    string
	NatsURL        string
	GarageEndpoint string
	GarageAccessKey string
	GarageSecretKey string
	GarageBucket   string
	ListenAddr     string
}

func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		NatsURL:        getEnvDefault("NATS_URL", "nats://localhost:4222"),
		GarageEndpoint: getEnvDefault("GARAGE_ENDPOINT", "http://localhost:3900"),
		GarageAccessKey: os.Getenv("GARAGE_ACCESS_KEY"),
		GarageSecretKey: os.Getenv("GARAGE_SECRET_KEY"),
		GarageBucket:   getEnvDefault("GARAGE_BUCKET", "skadia-originals"),
		ListenAddr:     getEnvDefault("LISTEN_ADDR", ":8080"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.GarageAccessKey == "" {
		return nil, fmt.Errorf("GARAGE_ACCESS_KEY is required")
	}
	if cfg.GarageSecretKey == "" {
		return nil, fmt.Errorf("GARAGE_SECRET_KEY is required")
	}

	return cfg, nil
}

func getEnvDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
