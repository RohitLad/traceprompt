package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rawMap decodes a raw JSON document for postPublic (which re-marshals).
func rawMap(t *testing.T, raw string) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &out))
	return out
}

func postUI(t *testing.T, app *fiberApp, token, path string, body any) (int, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, path, jsonBody(string(raw)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range authHeader(token) {
		req.Header.Set(k, v)
	}
	resp, err := app.app.Test(req, -1)
	require.NoError(t, err)
	defer resp.Body.Close()
	return resp.StatusCode, decodeJSON(t, resp)
}

func TestOtelIngestionRoundTrip(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "otel@example.com")
	projectID := createProject(t, app, token, orgID, "otel-proj")
	pub, sec := createKey(t, app, token, projectID)

	payload := `{"resourceSpans":[{
		"resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "demo"}}]},
		"scopeSpans": [{"spans": [
			{"traceId": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "spanId": "bbbbbbbbbbbbbbbb", "name": "chat",
			 "startTimeUnixNano": "1760000000000000000", "endTimeUnixNano": "1760000001000000000",
			 "attributes": [{"key": "langfuse.user.id", "value": {"stringValue": "u7"}}]},
			{"traceId": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "spanId": "cccccccccccccccc",
			 "parentSpanId": "bbbbbbbbbbbbbbbb", "name": "llm",
			 "startTimeUnixNano": "1760000000000000000", "endTimeUnixNano": "1760000001000000000",
			 "attributes": [
				{"key": "langfuse.observation.type", "value": {"stringValue": "generation"}},
				{"key": "gen_ai.request.model", "value": {"stringValue": "gpt-4o-mini"}}
			 ]}
		]}]}]}`

	code, _ := postPublic(t, app, pub, sec, "/api/public/otel/v1/traces", rawMap(t, payload))
	assert.Equal(t, http.StatusOK, code)
	drain(t, app, projectID)

	code, obs := getPublic(t, app, pub, sec, "/api/public/v2/observations?traceId=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	require.Equal(t, http.StatusOK, code, "body: %v", obs)
	data, _ := obs["data"].([]any)
	require.Len(t, data, 2, "root + child observations persisted via queue items")

	code, _ = postPublic(t, app, "pk-lf-nope", "sk-lf-nope", "/api/public/otel/v1/traces", rawMap(t, payload))
	assert.Equal(t, http.StatusUnauthorized, code)
}

func TestPromptLifecycle(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "pr@example.com")
	projectID := createProject(t, app, token, orgID, "pr-proj")
	pub, sec := createKey(t, app, token, projectID)
	ui := func(path string) string { return fmt.Sprintf("/api/v1/projects/%s%s", projectID, path) }

	// Create text prompt → version 1 with production+latest labels.
	code, body := postUI(t, app, token, ui("/prompts"), map[string]any{
		"name": "greeting", "type": "text", "template": "Hello {{name}}",
	})
	require.Equal(t, http.StatusCreated, code, "body: %v", body)
	versions, _ := body["versions"].([]any)
	require.Len(t, versions, 1)

	// Duplicate name → 409.
	code, _ = postUI(t, app, token, ui("/prompts"), map[string]any{
		"name": "greeting", "type": "text", "template": "Hi",
	})
	assert.Equal(t, http.StatusConflict, code)

	// New version → v2, latest moves, production stays on v1.
	code, v2 := postUI(t, app, token, ui("/prompts/greeting/versions"), map[string]any{
		"template": "Hello {{name}}!", "commitMessage": "exclaim",
	})
	require.Equal(t, http.StatusCreated, code, "body: %v", v2)
	assert.Equal(t, float64(2), v2["version"])

	// Public fetch defaults to production (v1 template).
	code, fetched := getPublic(t, app, pub, sec, "/api/public/prompts/greeting")
	require.Equal(t, http.StatusOK, code, "body: %v", fetched)
	assert.Equal(t, "Hello {{name}}", fetched["prompt"])
	assert.Equal(t, float64(1), fetched["version"])

	// ?version=2 fetches v2 explicitly.
	code, fetched = getPublic(t, app, pub, sec, "/api/public/prompts/greeting?version=2")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "Hello {{name}}!", fetched["prompt"])

	// Promote v2 to production → public default flips.
	code, _ = postUI(t, app, token, ui("/prompts/greeting/labels"),
		map[string]any{"version": 2, "labels": []string{"production"}})
	require.Equal(t, http.StatusOK, code)
	code, fetched = getPublic(t, app, pub, sec, "/api/public/prompts/greeting")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, float64(2), fetched["version"])
}

func TestPromptValidation(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "pv@example.com")
	projectID := createProject(t, app, token, orgID, "pv-proj")
	ui := func(path string) string { return fmt.Sprintf("/api/v1/projects/%s%s", projectID, path) }

	code, _ := postUI(t, app, token, ui("/prompts"),
		map[string]any{"name": "empty", "type": "text"})
	assert.Equal(t, http.StatusBadRequest, code, "text prompt needs template")

	code, _ = postUI(t, app, token, ui("/prompts"),
		map[string]any{"name": "chatty", "type": "chat", "messages": []any{}})
	assert.Equal(t, http.StatusBadRequest, code, "chat prompt needs messages")

	code, body := postUI(t, app, token, ui("/prompts"), map[string]any{
		"name": "chatty", "type": "chat",
		"messages": []any{map[string]any{"role": "user", "content": "Hi {{name}}"}},
	})
	require.Equal(t, http.StatusCreated, code, "body: %v", body)

	code, _ = postUI(t, app, token, ui("/prompts/chatty/labels"),
		map[string]any{"version": 99, "labels": []string{"production"}})
	assert.Equal(t, http.StatusNotFound, code)
}

func TestPromptCrossProjectIsolation(t *testing.T) {
	app := newTestApp(t)
	tokenA, orgA := register(t, app, "pa@example.com")
	tokenB, orgB := register(t, app, "pb@example.com")
	projA := createProject(t, app, tokenA, orgA, "a")
	projB := createProject(t, app, tokenB, orgB, "b")

	code, _ := postUI(t, app, tokenA, fmt.Sprintf("/api/v1/projects/%s/prompts", projA),
		map[string]any{"name": "shared", "type": "text", "template": "A"})
	require.Equal(t, http.StatusCreated, code)

	// Same name in another project is fine (scoped uniqueness).
	code, _ = postUI(t, app, tokenB, fmt.Sprintf("/api/v1/projects/%s/prompts", projB),
		map[string]any{"name": "shared", "type": "text", "template": "B"})
	assert.Equal(t, http.StatusCreated, code)

	// Cross-org UI access denied.
	code, _ = getUI(t, app, tokenB, fmt.Sprintf("/api/v1/projects/%s/prompts/shared", projA))
	assert.Contains(t, []int{http.StatusForbidden, http.StatusNotFound}, code)
}
