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
	// RefreshDuration is how long a login lasts without activity; JWTDuration should stay short.
	RefreshDuration time.Duration
	// ResendAPIKey is optional: without it emails are printed to the log.
	ResendAPIKey string
	MailFrom     string
	// AppURL is the frontend origin that links in emails point to.
	AppURL string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Printf("error load env: %v", err)
	}

	return &Config{
		DBSource:        mustEnv("DB_SOURCE"),
		ServerPort:      getEnv("SERVER_PORT", ":12152"),
		JWTSecret:       mustEnv("JWT_SECRET"),
		JWTDuration:     getDuration("JWT_DURATION", "15m"),
		RefreshDuration: getDuration("REFRESH_DURATION", "720h"),
		ResendAPIKey:    os.Getenv("RESEND_API_KEY"),
		MailFrom:        getEnv("MAIL_FROM", "Saluyu <noreply@fikriachmad.dev>"),
		AppURL:          getEnv("APP_URL", "http://localhost:5173"),
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

func getDuration(key, fallback string) time.Duration {
	d, err := time.ParseDuration(getEnv(key, fallback))
	if err != nil {
		log.Fatalf("invalid %s: %v", key, err)
	}
	return d
}
