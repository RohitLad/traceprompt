package httpapi

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/traceprompt/traceprompt/backend/internal/models"
	"github.com/traceprompt/traceprompt/backend/internal/queue"
)

func TestPublicRateLimit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Migrator().DropTable(models.AllModels()...))
	require.NoError(t, db.AutoMigrate(models.AllModels()...))
	app := New(&Deps{DB: db, JWTSecret: testJWTSecret, Queue: queue.NewMemory(), PublicRateLimit: 2})

	limited := false
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest(http.MethodGet, "/api/public/projects", nil)
		resp, err := app.Test(req, -1)
		require.NoError(t, err)
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	assert.True(t, limited, "expected 429 after exceeding 2 req/min")

	// Health and UI API are never rate-limited.
	req, _ := http.NewRequest(http.MethodGet, "/api/health", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
