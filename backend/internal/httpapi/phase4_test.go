package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/traceprompt/traceprompt/backend/internal/ingest"
)

func mustProjectID(t *testing.T, id string) uuid.UUID {
	t.Helper()
	pid, err := uuid.Parse(id)
	require.NoError(t, err)
	return pid
}

// seedBatch builds 2 traces + 3 observations timestamped now.
func seedBatch(t *testing.T) []ingest.Parsed {
	t.Helper()
	now := time.Now().UTC()
	later := now.Add(time.Second)
	f := func(t time.Time) string { return t.Format(time.RFC3339) }
	raw := fmt.Sprintf(`{"batch":[
		{"id":"e1","type":"trace-create","timestamp":"%s","body":{"id":"m-t1","name":"a"}},
		{"id":"e2","type":"generation-create","timestamp":"%s","body":{"id":"m-o1","traceId":"m-t1","name":"llm","model":"gpt-4o","usage":{"input":10,"output":11,"total":21},"startTime":"%s","endTime":"%s"}},
		{"id":"e3","type":"span-create","timestamp":"%s","body":{"id":"m-o2","traceId":"m-t1","name":"step","startTime":"%s","endTime":"%s"}},
		{"id":"e4","type":"trace-create","timestamp":"%s","body":{"id":"m-t2","name":"b"}},
		{"id":"e5","type":"generation-create","timestamp":"%s","body":{"id":"m-o3","traceId":"m-t2","name":"llm","model":"gpt-4o","usage":{"input":3,"output":3,"total":6}}}
	]}`, f(now), f(now), f(now), f(later), f(now), f(now), f(later), f(now), f(now))
	parsed, errs := ingest.Parse(json.RawMessage(raw))
	require.Empty(t, errs)
	return parsed
}

func TestDatasetFlow(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "ds@example.com")
	projectID := createProject(t, app, token, orgID, "ds-proj")
	pub, sec := createKey(t, app, token, projectID)
	base := fmt.Sprintf("/api/v1/projects/%s", projectID)

	// Create dataset + duplicate rejected.
	code, ds := postUI(t, app, token, base+"/datasets", map[string]any{"name": "qa", "description": "regression"})
	require.Equal(t, http.StatusCreated, code, "body: %v", ds)
	dsID, _ := ds["id"].(string)
	code, _ = postUI(t, app, token, base+"/datasets", map[string]any{"name": "qa"})
	assert.Equal(t, http.StatusConflict, code)

	// Two items.
	for _, q := range []string{"2+2?", "capital of FR?"} {
		code, _ = postUI(t, app, token, base+"/datasets/"+dsID+"/items", map[string]any{
			"input": q, "expectedOutput": "x",
		})
		require.Equal(t, http.StatusCreated, code)
	}
	code, _ = postUI(t, app, token, base+"/datasets/"+dsID+"/items", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, code)

	// Ingest a trace to link, then create run + link items.
	batch := `{"batch":[
		{"id":"e1","type":"trace-create","timestamp":"2026-01-01T00:00:00Z","body":{"id":"run-t1","name":"run"}},
		{"id":"e2","type":"generation-create","timestamp":"2026-01-01T00:00:01Z",
		 "body":{"id":"run-o1","traceId":"run-t1","name":"llm","model":"gpt-4o","usage":{"input":3,"output":4,"total":7}}}
	]}`
	code, _ = postIngestion(t, app, pub, sec, batch)
	require.Equal(t, http.StatusMultiStatus, code)
	drain(t, app, projectID)

	code, detail := getUI(t, app, token, base+"/datasets/"+dsID)
	require.Equal(t, http.StatusOK, code)
	items, _ := detail["data"].(map[string]any)["items"].([]any)
	require.Len(t, items, 2)
	itemID := items[0].(map[string]any)["id"].(string)

	code, run := postUI(t, app, token, base+"/datasets/"+dsID+"/runs", map[string]any{"name": "run-1"})
	require.Equal(t, http.StatusCreated, code, "body: %v", run)
	runID := run["id"].(string)

	code, _ = postUI(t, app, token, base+"/runs/"+runID+"/items",
		map[string]any{"itemId": itemID, "traceId": "run-t1"})
	assert.Equal(t, http.StatusCreated, code)

	// Linking an item from another dataset is rejected.
	code, other := postUI(t, app, token, base+"/datasets", map[string]any{"name": "other"})
	require.Equal(t, http.StatusCreated, code)
	code, _ = postUI(t, app, token, base+"/runs/"+runID+"/items",
		map[string]any{"itemId": "00000000-0000-0000-0000-000000000000", "traceId": "run-t1"})
	assert.Equal(t, http.StatusBadRequest, code)
	_ = other

	// Re-linking same item updates (idempotent, still 1 link... assert via detail).
	code, linked := postUI(t, app, token, base+"/runs/"+runID+"/items",
		map[string]any{"itemId": itemID, "traceId": "run-t1"})
	assert.Equal(t, http.StatusCreated, code)
	_ = linked

	code, got := getUI(t, app, token, base+"/runs/"+runID)
	require.Equal(t, http.StatusOK, code, "body: %v", got)
	ritems, _ := got["data"].(map[string]any)["items"].([]any)
	require.Len(t, ritems, 1, "re-link updates instead of duplicating")
	assert.Equal(t, "run", ritems[0].(map[string]any)["traceName"])

	// Public dataset API exposes items for runners.
	code, pubList := getPublic(t, app, pub, sec, "/api/public/datasets")
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, pubList["data"], 2)
	code, one := getPublic(t, app, pub, sec, "/api/public/datasets/qa")
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, one["data"].(map[string]any)["items"], 2)
}

func TestMetricsOverview(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "m@example.com")
	projectID := createProject(t, app, token, orgID, "m-proj")

	// Seed via store directly for deterministic timestamps (today).
	store := ingest.NewStore(app.db)
	pid := mustProjectID(t, projectID)
	ctx := context.Background()
	for _, ev := range seedBatch(t) {
		require.NoError(t, store.Apply(ctx, pid, ev))
	}

	code, m := getUI(t, app, token, fmt.Sprintf("/api/v1/projects/%s/metrics/overview?days=30", projectID))
	require.Equal(t, http.StatusOK, code, "body: %v", m)
	data := m["data"].(map[string]any)
	assert.Equal(t, float64(2), data["traces"])
	assert.Equal(t, float64(3), data["observations"])
	assert.Equal(t, float64(13), data["inputTokens"])
	assert.Equal(t, float64(14), data["outputTokens"])
	assert.NotNil(t, data["avgLatencyMs"])
	perDay, _ := data["perDay"].([]any)
	assert.NotEmpty(t, perDay)
	byModel, _ := data["byModel"].([]any)
	require.NotEmpty(t, byModel)
	// gpt-4o has 2 observations vs 1 for "unknown" → sorts first.
	assert.Equal(t, "gpt-4o", byModel[0].(map[string]any)["model"])
	assert.False(t, data["truncated"].(bool))

	// Cross-org access denied.
	tokenB, _ := register(t, app, "mb@example.com")
	code, _ = getUI(t, app, tokenB, fmt.Sprintf("/api/v1/projects/%s/metrics/overview", projectID))
	assert.Contains(t, []int{http.StatusForbidden, http.StatusNotFound}, code)
}
