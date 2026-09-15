package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DBSource   string
	ServerPort string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Printf("error load env: %v", err)
	}

	return &Config{
		DBSource:   getEnv("DB_SOURCE", ""),
		ServerPort: getEnv("SERVER_PORT", ":12152"),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
