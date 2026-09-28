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
