package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLLM starts a fake OpenAI-compatible server replying with fixed text.
func mockLLM(t *testing.T, reply string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"model":"mock","choices":[{"message":{"role":"assistant","content":%q}}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, reply)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func providerPayload(baseURL string) map[string]string {
	return map[string]string{"baseUrl": baseURL, "apiKey": "test-key", "model": "mock"}
}

func TestPlaygroundRunWithMockProvider(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "pg@example.com")
	projectID := createProject(t, app, token, orgID, "pg-proj")
	llmURL := mockLLM(t, "Mocked reply")

	code, body := postUI(t, app, token,
		fmt.Sprintf("/api/v1/projects/%s/playground/run", projectID),
		map[string]any{
			"template":    "Hello {{name}}",
			"variables":   map[string]string{"name": "Ada"},
			"provider":    providerPayload(llmURL),
			"saveAsTrace": true,
		})
	require.Equal(t, http.StatusOK, code, "body: %v", body)
	data := body["data"].(map[string]any)
	assert.Equal(t, "Mocked reply", data["output"])
	require.NotNil(t, data["traceId"], "saved run links a trace")

	// Trace persisted and visible in UI list.
	code, traces := getUI(t, app, token, fmt.Sprintf("/api/v1/projects/%s/traces", projectID))
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, traces["data"], 1)

	// Missing variables rejected before provider call.
	code, _ = postUI(t, app, token,
		fmt.Sprintf("/api/v1/projects/%s/playground/run", projectID),
		map[string]any{
			"template":  "Hello {{name}}",
			"variables": map[string]string{},
			"provider":  providerPayload(llmURL),
		})
	assert.Equal(t, http.StatusBadRequest, code)

	// Template + promptName together rejected.
	code, _ = postUI(t, app, token,
		fmt.Sprintf("/api/v1/projects/%s/playground/run", projectID),
		map[string]any{
			"template": "x", "promptName": "y",
			"provider": providerPayload(llmURL),
		})
	assert.Equal(t, http.StatusBadRequest, code)

	// Provider errors surface as 502, never leak the key.
	code, errBody := postUI(t, app, token,
		fmt.Sprintf("/api/v1/projects/%s/playground/run", projectID),
		map[string]any{
			"template": "x",
			"provider": map[string]string{"baseUrl": "http://127.0.0.1:1", "apiKey": "sk-secret", "model": "m"},
		})
	assert.Equal(t, http.StatusBadGateway, code)
	assert.NotContains(t, fmt.Sprintf("%v", errBody), "sk-secret")
}

func TestPlaygroundRunFromStoredPrompt(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "pgp@example.com")
	projectID := createProject(t, app, token, orgID, "pgp-proj")
	base := fmt.Sprintf("/api/v1/projects/%s", projectID)
	llmURL := mockLLM(t, "Mocked reply")

	code, _ := postUI(t, app, token, base+"/prompts",
		map[string]any{"name": "greet", "type": "text", "template": "Hi {{who}}"})
	require.Equal(t, http.StatusCreated, code)

	code, body := postUI(t, app, token, base+"/playground/run", map[string]any{
		"promptName": "greet",
		"variables":  map[string]string{"who": "Bo"},
		"provider":   providerPayload(llmURL),
	})
	require.Equal(t, http.StatusOK, code, "body: %v", body)
	assert.Equal(t, "Mocked reply", body["data"].(map[string]any)["output"])
}

func TestSessionsAndScoresUI(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "ss@example.com")
	projectID := createProject(t, app, token, orgID, "ss-proj")
	pub, sec := createKey(t, app, token, projectID)
	base := fmt.Sprintf("/api/v1/projects/%s", projectID)

	batch := `{"batch":[
		{"id":"e1","type":"trace-create","timestamp":"2026-01-01T00:00:00Z","body":{"id":"s-t1","name":"chat","sessionId":"sess-1"}},
		{"id":"e2","type":"trace-create","timestamp":"2026-01-01T00:01:00Z","body":{"id":"s-t2","name":"chat","sessionId":"sess-1"}},
		{"id":"e3","type":"score-create","timestamp":"2026-01-01T00:00:02Z","body":{"id":"s-s1","traceId":"s-t1","name":"q","value":0.5}}
	]}`
	code, _ := postIngestion(t, app, pub, sec, batch)
	require.Equal(t, http.StatusMultiStatus, code)
	drain(t, app, projectID)

	code, sessions := getUI(t, app, token, base+"/sessions")
	require.Equal(t, http.StatusOK, code, "body: %v", sessions)
	sdata, _ := sessions["data"].([]any)
	require.Len(t, sdata, 1)
	assert.Equal(t, "sess-1", sdata[0].(map[string]any)["sessionId"])
	assert.Equal(t, float64(2), sdata[0].(map[string]any)["traceCount"])

	code, straces := getUI(t, app, token, base+"/sessions/sess-1/traces")
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, straces["data"], 2)

	code, scores := getUI(t, app, token, base+"/scores")
	require.Equal(t, http.StatusOK, code, "body: %v", scores)
	scdata, _ := scores["data"].([]any)
	require.Len(t, scdata, 1)
	assert.Equal(t, 0.5, scdata[0].(map[string]any)["value"])

	code, filtered := getUI(t, app, token, base+"/scores?traceId=nope")
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, filtered["data"])
}
