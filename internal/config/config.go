package config

import (
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DBSource    string
	ServerPort  string
	JWTSecret   string
	JWTDuration time.Duration
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Printf("error load env: %v", err)
	}

	duration, err := time.ParseDuration(getEnv("JWT_DURATION", "24h"))
	if err != nil {
		log.Fatalf("invalid JWT_DURATION: %v", err)
	}

	return &Config{
		DBSource:    mustEnv("DB_SOURCE"),
		ServerPort:  getEnv("SERVER_PORT", ":12152"),
		JWTSecret:   mustEnv("JWT_SECRET"),
		JWTDuration: duration,
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func mustEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("%s is required", key)
	}
	return value
}
