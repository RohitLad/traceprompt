package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadDefaults(t *testing.T) {
	cfg := Load()
	assert.Equal(t, "development", cfg.Env)
	assert.Equal(t, 3000, cfg.Port)
	assert.Equal(t, ":3000", cfg.Addr())
	assert.NotEmpty(t, cfg.DatabaseURL)
	assert.NotEmpty(t, cfg.RedisURL)
	assert.Equal(t, 300, cfg.PublicRateLimit)
}

func TestAddrFormat(t *testing.T) {
	cfg := Load()
	cfg.Port = 8080
	assert.Equal(t, ":8080", cfg.Addr())
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("PORT", "8080")
	t.Setenv("DATABASE_URL", "postgres://example/db")
	t.Setenv("REDIS_URL", "redis://example:6379/1")
	t.Setenv("JWT_SECRET", "s3cret")
	t.Setenv("PUBLIC_RATE_LIMIT_PER_MIN", "42")

	cfg := Load()
	assert.Equal(t, "production", cfg.Env)
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "postgres://example/db", cfg.DatabaseURL)
	assert.Equal(t, "redis://example:6379/1", cfg.RedisURL)
	assert.Equal(t, "s3cret", cfg.JWTSecret)
	assert.Equal(t, 42, cfg.PublicRateLimit)
}

func TestInvalidPortFallsBack(t *testing.T) {
	t.Setenv("PORT", "not-a-number")
	assert.Equal(t, 3000, Load().Port)
}
