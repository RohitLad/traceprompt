package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/traceprompt/traceprompt/backend/internal/ingest"
	"github.com/traceprompt/traceprompt/backend/internal/models"
)

// --- small HTTP helpers shared by phase-2 tests ---

func jsonBody(s string) io.Reader {
	return strings.NewReader(s)
}

func decodeJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var out map[string]any
	if len(raw) > 0 {
		require.NoError(t, json.Unmarshal(raw, &out), "body: %s", string(raw))
	}
	return out
}

func getPublic(t *testing.T, app *fiberApp, pub, sec, path string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, path, nil)
	require.NoError(t, err)
	for k, v := range basicHeader(pub, sec) {
		req.Header.Set(k, v)
	}
	resp, err := app.app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, decodeJSON(t, resp)
}

func postPublic(t *testing.T, app *fiberApp, pub, sec, path string, body any) (int, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, path, strings.NewReader(string(raw)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range basicHeader(pub, sec) {
		req.Header.Set(k, v)
	}
	resp, err := app.app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, decodeJSON(t, resp)
}

func getUI(t *testing.T, app *fiberApp, token, path string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, path, nil)
	require.NoError(t, err)
	for k, v := range authHeader(token) {
		req.Header.Set(k, v)
	}
	resp, err := app.app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, decodeJSON(t, resp)
}

// drain applies everything currently queued (simulates the worker).
func drain(t *testing.T, app *fiberApp, projectID string) {
	t.Helper()
	pid, err := uuid.Parse(projectID)
	require.NoError(t, err)
	store := ingest.NewStore(app.db)
	for _, it := range app.queue.Drain(1000) {
		require.NoError(t, store.Apply(context.Background(), pid, it.ToParsed()))
	}
}

func postIngestion(t *testing.T, app *fiberApp, pub, sec string, batch string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "/api/public/ingestion", jsonBody(batch))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range basicHeader(pub, sec) {
		req.Header.Set(k, v)
	}
	resp, err := app.app.Test(req, -1)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, decodeJSON(t, resp)
}

func TestIngestionRoundTrip(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "ing@example.com")
	projectID := createProject(t, app, token, orgID, "ing-proj")
	pub, sec := createKey(t, app, token, projectID)

	batch := `{"batch":[
		{"id":"e1","type":"trace-create","timestamp":"2026-01-01T00:00:00Z",
		 "body":{"id":"t1","name":"chat","userId":"u1","sessionId":"s1","tags":["web"]}},
		{"id":"e2","type":"generation-create","timestamp":"2026-01-01T00:00:01Z",
		 "body":{"id":"o1","traceId":"t1","name":"llm","model":"gpt-4o","input":"hi","output":"hello","usage":{"input":5,"output":7,"total":12}}},
		{"id":"e3","type":"score-create","timestamp":"2026-01-01T00:00:02Z",
		 "body":{"id":"sc1","traceId":"t1","observationId":"o1","name":"quality","value":0.95}},
		{"id":"e4","type":"bogus","timestamp":"2026-01-01T00:00:03Z","body":{"id":"x"}}
	]}`
	code, body := postIngestion(t, app, pub, sec, batch)
	assert.Equal(t, http.StatusMultiStatus, code, "body: %v", body)
	assert.Len(t, body["successes"], 3)
	assert.Len(t, body["errors"], 1)

	drain(t, app, projectID)

	// v2 observations read back the generation with usage.
	code, obs := getPublic(t, app, pub, sec, "/api/public/v2/observations?traceId=t1")
	require.Equal(t, http.StatusOK, code, "body: %v", obs)
	data, _ := obs["data"].([]any)
	require.Len(t, data, 1)
	row := data[0].(map[string]any)
	assert.Equal(t, "GENERATION", row["type"])
	assert.Equal(t, "gpt-4o", row["model"])
	assert.Equal(t, float64(5), row["inputUsage"])
	assert.Equal(t, "u1", row["userId"], "trace attrs joined onto rows")
	assert.Equal(t, "s1", row["sessionId"])

	// v3 scores read back the typed numeric value.
	code, scores := getPublic(t, app, pub, sec, "/api/public/v3/scores?traceId=t1")
	require.Equal(t, http.StatusOK, code)
	sdata, _ := scores["data"].([]any)
	require.Len(t, sdata, 1)
	assert.Equal(t, 0.95, sdata[0].(map[string]any)["value"])
	assert.Equal(t, "NUMERIC", sdata[0].(map[string]any)["dataType"])

	// UI traces list shows the trace with observation count.
	code, ui := getUI(t, app, token, fmt.Sprintf("/api/v1/projects/%s/traces", projectID))
	require.Equal(t, http.StatusOK, code, "body: %v", ui)
	udata, _ := ui["data"].([]any)
	require.Len(t, udata, 1)
	assert.Equal(t, float64(1), udata[0].(map[string]any)["observationCount"])

	// UI trace detail returns the observation tree.
	code, detail := getUI(t, app, token, fmt.Sprintf("/api/v1/projects/%s/traces/t1", projectID))
	require.Equal(t, http.StatusOK, code)
	d := detail["data"].(map[string]any)
	assert.Equal(t, "chat", d["name"])
	assert.Len(t, d["observations"], 1)
}

func TestIngestionRequiresAuth(t *testing.T) {
	app := newTestApp(t)
	code, _ := postIngestion(t, app, "pk-lf-nope", "sk-lf-nope", `{"batch":[]}`)
	assert.Equal(t, http.StatusUnauthorized, code)
}

func TestObservationsPagination(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "page@example.com")
	projectID := createProject(t, app, token, orgID, "page-proj")
	pub, sec := createKey(t, app, token, projectID)

	var batch strings.Builder
	batch.WriteString(`{"batch":[`)
	for i := 0; i < 5; i++ {
		if i > 0 {
			batch.WriteString(",")
		}
		fmt.Fprintf(&batch, `{"id":"e%d","type":"span-create","timestamp":"2026-01-01T00:00:0%dZ","body":{"id":"o%d","traceId":"t1","name":"s%d"}}`, i, i, i, i)
	}
	batch.WriteString(`]}`)
	code, _ := postIngestion(t, app, pub, sec, batch.String())
	require.Equal(t, http.StatusMultiStatus, code)
	drain(t, app, projectID)

	code, page1 := getPublic(t, app, pub, sec, "/api/public/v2/observations?traceId=t1&limit=2")
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, page1["data"], 2)
	meta, _ := page1["meta"].(map[string]any)
	cursor, _ := meta["cursor"].(string)
	require.NotEmpty(t, cursor, "full page must return a cursor")

	code, page2 := getPublic(t, app, pub, sec, "/api/public/v2/observations?traceId=t1&limit=2&cursor="+cursor)
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, page2["data"], 2)
}

func TestSingleScoreEndpoint(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "sc@example.com")
	projectID := createProject(t, app, token, orgID, "sc-proj")
	pub, sec := createKey(t, app, token, projectID)

	code, body := postPublic(t, app, pub, sec, "/api/public/scores",
		map[string]any{"traceId": "t9", "name": "label", "value": "good"})
	assert.Equal(t, http.StatusOK, code, "body: %v", body)

	code, scores := getPublic(t, app, pub, sec, "/api/public/v3/scores?traceId=t9")
	require.Equal(t, http.StatusOK, code)
	sdata, _ := scores["data"].([]any)
	require.Len(t, sdata, 1)
	assert.Equal(t, "good", sdata[0].(map[string]any)["value"])

	var n int64
	require.NoError(t, app.db.Model(&models.Score{}).Count(&n).Error)
	assert.Equal(t, int64(1), n)
}
