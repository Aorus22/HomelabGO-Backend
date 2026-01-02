package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPPort       string
	DatabaseURL    string
	JWTSecret      string
	DataVolumePath string
}

func Load() (Config, error) {
	_ = godotenv.Load()
	_ = godotenv.Load("../../.env")

	cfg := Config{
		HTTPPort:       getEnv("HTTP_PORT", "8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		DataVolumePath: getEnv("DATA_VOLUME_PATH", "/data/volumes"),
	}

	if cfg.DatabaseURL == "" {
		host := os.Getenv("DB_HOST")
		user := os.Getenv("DB_USER")
		password := os.Getenv("DB_PASSWORD")
		name := os.Getenv("DB_NAME")
		port := getEnv("DB_PORT", "5432")
		sslMode := getEnv("DB_SSLMODE", "disable")

		if host == "" || user == "" || name == "" {
			return Config{}, errors.New("DATABASE_URL or DB_HOST/DB_USER/DB_NAME must be set")
		}

		cfg.DatabaseURL = fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=UTC",
			host, user, password, name, port, sslMode,
		)
	}

	if cfg.JWTSecret == "" {
		return Config{}, errors.New("JWT_SECRET must be set")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
