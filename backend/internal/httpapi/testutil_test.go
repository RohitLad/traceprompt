package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/traceprompt/traceprompt/backend/internal/models"
)

const testJWTSecret = "test-secret-32-chars-minimum-xyz!"

// fiberApp bundles the Fiber app with its test database.
type fiberApp struct {
	app *fiber.App
	db  *gorm.DB
}

// newTestApp spins up an isolated in-memory SQLite app per test.
// Auth tables are dialect-agnostic; Trace/Observation JSON uses
// serializer:json so they migrate cleanly here too.
func newTestApp(t *testing.T) *fiberApp {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	// Isolate: drop + recreate (shared cache reuses the same DB across tests).
	require.NoError(t, db.Migrator().DropTable(models.AllModels()...))
	require.NoError(t, db.AutoMigrate(models.AllModels()...))
	return &fiberApp{app: New(&Deps{DB: db, JWTSecret: testJWTSecret}), db: db}
}

func doRequest(t *testing.T, app *fiberApp, method, path string, body any, headers map[string]string) (int, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, path, rdr)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := app.app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var decoded map[string]any
	if len(raw) > 0 {
		require.NoError(t, json.Unmarshal(raw, &decoded), "body: %s", string(raw))
	}
	return resp.StatusCode, decoded
}

func register(t *testing.T, app *fiberApp, email string) (token string, orgID string) {
	t.Helper()
	code, body := doRequest(t, app, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": email, "password": "password-123", "name": "Test User",
	}, nil)
	require.Equal(t, http.StatusCreated, code, "body: %v", body)
	token, _ = body["token"].(string)
	require.NotEmpty(t, token)
	org, _ := body["org"].(map[string]any)
	require.NotNil(t, org)
	return token, org["id"].(string)
}

func authHeader(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func basicHeader(pub, sec string) map[string]string {
	creds := base64.StdEncoding.EncodeToString([]byte(pub + ":" + sec))
	return map[string]string{"Authorization": "Basic " + creds}
}

func createProject(t *testing.T, app *fiberApp, token, orgID, name string) string {
	t.Helper()
	code, body := doRequest(t, app, http.MethodPost, "/api/v1/projects",
		map[string]string{"name": name, "organizationId": orgID}, authHeader(token))
	require.Equal(t, http.StatusCreated, code, "body: %v", body)
	id, _ := body["id"].(string)
	require.NotEmpty(t, id)
	return id
}

func createKey(t *testing.T, app *fiberApp, token, projectID string) (pub, sec string) {
	t.Helper()
	code, body := doRequest(t, app, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%s/keys", projectID),
		map[string]string{"name": "ci"}, authHeader(token))
	require.Equal(t, http.StatusCreated, code, "body: %v", body)
	return body["publicKey"].(string), body["secret"].(string)
}
