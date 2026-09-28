package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/traceprompt/traceprompt/backend/internal/ingest"
	"github.com/traceprompt/traceprompt/backend/internal/models"
)

// TestFreshAccountJourney walks the exact path a new user takes:
// register -> empty projects -> first project (name only) -> key ->
// ingest -> worker drain -> every read surface. This is the journey
// whose first step shipped broken (org chicken-and-egg); it must never
// regress to per-handler passing while the whole fails.
func TestFreshAccountJourney(t *testing.T) {
	app := newTestApp(t)

	// 1. Register; brand-new account owns nothing.
	token, _ := register(t, app, "journey@example.com")
	code, body := getUI(t, app, token, "/api/v1/projects")
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, body["data"], "fresh account starts project-less")

	// 2. First project with name only (no org known client-side).
	code, proj := postUI(t, app, token, "/api/v1/projects", map[string]any{"name": "first"})
	require.Equal(t, http.StatusCreated, code, "body: %v", proj)
	projectID := proj["id"].(string)
	require.NotEmpty(t, proj["organizationId"])

	// 3. API key for SDK ingestion.
	pub, sec := createKey(t, app, token, projectID)

	// 4. Ingest a trace + generation + score in one batch.
	batch := `{"batch":[
		{"id":"e1","type":"trace-create","timestamp":"2026-01-01T00:00:00Z",
		 "body":{"id":"j-t1","name":"chat","userId":"u1","sessionId":"s1"}},
		{"id":"e2","type":"generation-create","timestamp":"2026-01-01T00:00:01Z",
		 "body":{"id":"j-o1","traceId":"j-t1","name":"llm","model":"gpt-4o","input":"hi","usage":{"input":2,"output":3,"total":5}}},
		{"id":"e3","type":"score-create","timestamp":"2026-01-01T00:00:02Z",
		 "body":{"id":"j-s1","traceId":"j-t1","name":"q","value":0.8}}
	]}`
	code, res := postIngestion(t, app, pub, sec, batch)
	require.Equal(t, http.StatusMultiStatus, code)
	assert.Len(t, res["successes"], 3)
	assert.Empty(t, res["errors"])
	drain(t, app, projectID)

	// 5. Every read surface agrees.
	code, traces := getUI(t, app, token, fmt.Sprintf("/api/v1/projects/%s/traces", projectID))
	require.Equal(t, http.StatusOK, code)
	tdata := traces["data"].([]any)
	require.Len(t, tdata, 1)
	assert.Equal(t, float64(1), tdata[0].(map[string]any)["observationCount"])

	code, detail := getUI(t, app, token, fmt.Sprintf("/api/v1/projects/%s/traces/j-t1", projectID))
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, detail["data"].(map[string]any)["observations"], 1)

	code, obs := getPublic(t, app, pub, sec, "/api/public/v2/observations?traceId=j-t1")
	require.Equal(t, http.StatusOK, code)
	odata := obs["data"].([]any)
	require.Len(t, odata, 1)
	assert.Equal(t, "u1", odata[0].(map[string]any)["userId"], "trace attrs joined")

	code, scores := getPublic(t, app, pub, sec, "/api/public/v3/scores?traceId=j-t1")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, scores["data"], 1)

	code, sessions := getUI(t, app, token, fmt.Sprintf("/api/v1/projects/%s/sessions", projectID))
	require.Equal(t, http.StatusOK, code)
	require.Len(t, sessions["data"], 1)

	code, uiscores := getUI(t, app, token, fmt.Sprintf("/api/v1/projects/%s/scores?traceId=j-t1", projectID))
	require.Equal(t, http.StatusOK, code)
	require.Len(t, uiscores["data"], 1)
}

// TestEvalJourney covers the evaluation loop end to end: schema declared,
// conforming data stored, violating data rejected at apply time (207 still
// accepts the batch — failure surfaces in the worker path), then human
// review through a queue to completion.
func TestEvalJourney(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "evalj@example.com")
	projectID := createProject(t, app, token, orgID, "evalj")
	pub, sec := createKey(t, app, token, projectID)
	pid := mustProjectID(t, projectID)
	base := fmt.Sprintf("/api/v1/projects/%s", projectID)

	code, _ := postUI(t, app, token, base+"/score-configs",
		map[string]any{"name": "q", "dataType": "NUMERIC", "minValue": 0, "maxValue": 1})
	require.Equal(t, http.StatusCreated, code)

	// Batch with one good + one violating score: ingestion accepts both.
	batch := `{"batch":[
		{"id":"e1","type":"trace-create","timestamp":"2026-01-01T00:00:00Z","body":{"id":"ev-t1","name":"t"}},
		{"id":"e2","type":"score-create","timestamp":"2026-01-01T00:00:01Z","body":{"id":"ev-good","traceId":"ev-t1","name":"q","value":0.5}},
		{"id":"e3","type":"score-create","timestamp":"2026-01-01T00:00:02Z","body":{"id":"ev-bad","traceId":"ev-t1","name":"q","value":99}}
	]}`
	code, res := postIngestion(t, app, pub, sec, batch)
	require.Equal(t, http.StatusMultiStatus, code)
	assert.Len(t, res["successes"], 3, "envelope validation passes; schema checked at apply")

	// Drain with error tolerance: exactly one permanent failure expected.
	var permanent int
	store := ingest.NewStore(app.db)
	for _, it := range app.queue.Drain(100) {
		if err := store.Apply(context.Background(), pid, it.ToParsed()); err != nil {
			require.True(t, ingest.IsPermanent(err), "schema violation must be permanent, got: %v", err)
			permanent++
		}
	}
	assert.Equal(t, 1, permanent)
	var n int64
	require.NoError(t, app.db.Model(&models.Score{}).Where("project_id = ?", pid).Count(&n).Error)
	assert.Equal(t, int64(1), n, "violating score never stored")

	// Human review of the trace through a queue.
	code, q := postUI(t, app, token, base+"/annotation-queues", map[string]any{"name": "rev"})
	require.Equal(t, http.StatusCreated, code)
	qid := q["id"].(string)
	code, item := postUI(t, app, token, base+"/annotation-queues/"+qid+"/items",
		map[string]any{"traceId": "ev-t1"})
	require.Equal(t, http.StatusCreated, code)
	itemID := item["id"].(string)
	code, _ = postUI(t, app, token,
		base+"/annotation-queues/"+qid+"/items/"+itemID+"/scores",
		map[string]any{"name": "human", "value": "good"})
	require.Equal(t, http.StatusCreated, code)
	code, _ = postUI(t, app, token,
		base+"/annotation-queues/"+qid+"/items/"+itemID+"/complete", map[string]any{})
	require.Equal(t, http.StatusOK, code)
	code, got := getUI(t, app, token, base+"/annotation-queues/"+qid+"?status=PENDING")
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, got["data"].(map[string]any)["items"])
}

// TestPlaygroundJourney runs a stored chat prompt and proves the saved run
// links back to a trace generation carrying the prompt reference.
func TestPlaygroundJourney(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "pgj@example.com")
	projectID := createProject(t, app, token, orgID, "pgj")
	base := fmt.Sprintf("/api/v1/projects/%s", projectID)
	llmURL := mockLLM(t, "Mocked!")

	code, _ := postUI(t, app, token, base+"/prompts", map[string]any{
		"name": "helper", "type": "chat",
		"messages": []any{map[string]any{"role": "system", "content": "Be brief {{tone}}"}},
	})
	require.Equal(t, http.StatusCreated, code)

	code, body := postUI(t, app, token, base+"/playground/run", map[string]any{
		"promptName":  "helper",
		"variables":   map[string]string{"tone": "kind"},
		"provider":    providerPayload(llmURL),
		"saveAsTrace": true,
	})
	require.Equal(t, http.StatusOK, code, "body: %v", body)
	data := body["data"].(map[string]any)
	require.NotNil(t, data["traceId"])
	tid := data["traceId"].(string)

	code, detail := getUI(t, app, token, base+"/traces/"+tid)
	require.Equal(t, http.StatusOK, code)
	obs := detail["data"].(map[string]any)["observations"].([]any)
	require.Len(t, obs, 1)
	assert.Equal(t, "helper", obs[0].(map[string]any)["promptName"])
	assert.Equal(t, float64(1), obs[0].(map[string]any)["promptVersion"])

	// Without saveAsTrace no trace is created.
	code, body = postUI(t, app, token, base+"/playground/run", map[string]any{
		"template": "Hi",
		"provider": providerPayload(llmURL),
	})
	require.Equal(t, http.StatusOK, code)
	assert.Nil(t, body["data"].(map[string]any)["traceId"])
}
