package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all runtime configuration, loaded from env with sane defaults.
// No globals are mutated — call Load() once in main and inject.
type Config struct {
	Env         string
	Port        int
	DatabaseURL string
	RedisURL    string
	JWTSecret   string
	// PublicRateLimit caps /api/public requests per IP per minute.
	// SDKs batch aggressively; 300 default tolerates bursts while
	// bounding abuse. 0 disables (local dev only).
	PublicRateLimit int
}

// Load reads configuration from environment variables.
func Load() Config {
	return Config{
		Env:             envOr("APP_ENV", "development"),
		Port:            envIntOr("PORT", 3000),
		DatabaseURL:     envOr("DATABASE_URL", "postgres://traceprompt:traceprompt@localhost:5432/traceprompt?sslmode=disable"),
		RedisURL:        envOr("REDIS_URL", "redis://localhost:6379/0"),
		JWTSecret:       envOr("JWT_SECRET", "dev-only-change-me"),
		PublicRateLimit: envIntOr("PUBLIC_RATE_LIMIT_PER_MIN", 300),
	}
}

// Addr returns the listen address for Fiber.
func (c Config) Addr() string {
	return fmt.Sprintf(":%d", c.Port)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
