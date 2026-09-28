package httpapi

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Full happy path: register -> project -> key -> public auth -> revoke -> denied.
func TestPhase1HappyPath(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "alice@example.com")
	projectID := createProject(t, app, token, orgID, "demo")
	pub, sec := createKey(t, app, token, projectID)

	// Public API with valid credentials returns the project (Langfuse shape).
	code, body := doRequest(t, app, http.MethodGet, "/api/public/projects", nil, basicHeader(pub, sec))
	require.Equal(t, http.StatusOK, code, "body: %v", body)
	data, _ := body["data"].([]any)
	require.Len(t, data, 1)

	// /me works with the JWT.
	code, body = doRequest(t, app, http.MethodGet, "/api/v1/me", nil, authHeader(token))
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "alice@example.com", body["email"])

	// Keys list never leaks the secret.
	code, body = doRequest(t, app, http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%s/keys", projectID), nil, authHeader(token))
	require.Equal(t, http.StatusOK, code)
	keys, _ := body["data"].([]any)
	require.Len(t, keys, 1)
	assert.NotContains(t, fmt.Sprintf("%v", keys[0]), sec)

	// Revoke -> public auth fails closed.
	code, _ = doRequest(t, app, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%s/keys/%s/revoke", projectID, keys[0].(map[string]any)["id"]),
		nil, authHeader(token))
	require.Equal(t, http.StatusOK, code)

	code, _ = doRequest(t, app, http.MethodGet, "/api/public/projects", nil, basicHeader(pub, sec))
	assert.Equal(t, http.StatusUnauthorized, code)
}

func TestRegisterValidationAndDuplicates(t *testing.T) {
	app := newTestApp(t)

	code, _ := doRequest(t, app, http.MethodPost, "/api/v1/auth/register",
		map[string]string{"email": "not-an-email", "password": "password-123", "name": "X"}, nil)
	assert.Equal(t, http.StatusBadRequest, code)

	code, _ = doRequest(t, app, http.MethodPost, "/api/v1/auth/register",
		map[string]string{"email": "bob@example.com", "password": "short", "name": "X"}, nil)
	assert.Equal(t, http.StatusBadRequest, code)

	register(t, app, "bob@example.com")
	code, _ = doRequest(t, app, http.MethodPost, "/api/v1/auth/register",
		map[string]string{"email": "BOB@example.com", "password": "password-123", "name": "X"}, nil)
	assert.Equal(t, http.StatusConflict, code, "emails are case-insensitive unique")
}

func TestLoginRejectsBadCredentials(t *testing.T) {
	app := newTestApp(t)
	register(t, app, "carol@example.com")

	code, _ := doRequest(t, app, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": "carol@example.com", "password": "wrong-pass-1"}, nil)
	assert.Equal(t, http.StatusUnauthorized, code)

	code, body := doRequest(t, app, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": "carol@example.com", "password": "password-123"}, nil)
	require.Equal(t, http.StatusOK, code)
	assert.NotEmpty(t, body["token"])
}

func TestAuthzBoundaries(t *testing.T) {
	app := newTestApp(t)
	tokenA, orgA := register(t, app, "a@example.com")
	tokenB, _ := register(t, app, "b@example.com")
	projectA := createProject(t, app, tokenA, orgA, "a-proj")

	// No token -> 401.
	code, _ := doRequest(t, app, http.MethodGet, "/api/v1/projects", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, code)

	// Cross-org access -> 403/404 (never leak existence details beyond status).
	code, _ = doRequest(t, app, http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%s", projectA), nil, authHeader(tokenB))
	assert.Contains(t, []int{http.StatusForbidden, http.StatusNotFound}, code)

	// Cross-org key creation forbidden.
	code, _ = doRequest(t, app, http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%s/keys", projectA),
		map[string]string{"name": "x"}, authHeader(tokenB))
	assert.Contains(t, []int{http.StatusForbidden, http.StatusNotFound}, code)

	// Public API without credentials -> 401.
	code, _ = doRequest(t, app, http.MethodGet, "/api/public/projects", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, code)

	// Public API with wrong secret -> 401.
	pub, _ := createKey(t, app, tokenA, projectA)
	code, _ = doRequest(t, app, http.MethodGet, "/api/public/projects", nil, basicHeader(pub, "sk-lf-wrong"))
	assert.Equal(t, http.StatusUnauthorized, code)
}

func TestProjectValidation(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "d@example.com")

	code, _ := doRequest(t, app, http.MethodPost, "/api/v1/projects",
		map[string]string{"name": "", "organizationId": orgID}, authHeader(token))
	assert.Equal(t, http.StatusBadRequest, code)

	code, _ = doRequest(t, app, http.MethodPost, "/api/v1/projects",
		map[string]string{"name": "x", "organizationId": "not-a-uuid"}, authHeader(token))
	assert.Equal(t, http.StatusBadRequest, code)
}
