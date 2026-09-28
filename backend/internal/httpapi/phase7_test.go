package httpapi

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScoreConfigEnforcement(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "ev@example.com")
	projectID := createProject(t, app, token, orgID, "ev-proj")
	pub, sec := createKey(t, app, token, projectID)
	base := fmt.Sprintf("/api/v1/projects/%s", projectID)

	// Declare schemas.
	code, _ := postUI(t, app, token, base+"/score-configs", map[string]any{
		"name": "quality", "dataType": "NUMERIC", "minValue": 0, "maxValue": 1,
	})
	require.Equal(t, http.StatusCreated, code)
	code, _ = postUI(t, app, token, base+"/score-configs", map[string]any{
		"name": "label", "dataType": "CATEGORICAL", "categories": []string{"good", "bad"},
	})
	require.Equal(t, http.StatusCreated, code)

	// Invalid config definitions rejected.
	code, _ = postUI(t, app, token, base+"/score-configs",
		map[string]any{"name": "x", "dataType": "WHATEVER"})
	assert.Equal(t, http.StatusBadRequest, code)
	code, _ = postUI(t, app, token, base+"/score-configs",
		map[string]any{"name": "quality", "dataType": "NUMERIC"})
	assert.Equal(t, http.StatusConflict, code)

	// In-range score passes; out-of-range is rejected (400, not stored).
	code, _ = postPublic(t, app, pub, sec, "/api/public/scores",
		map[string]any{"traceId": "t1", "name": "quality", "value": 0.8})
	assert.Equal(t, http.StatusOK, code)
	code, body := postPublic(t, app, pub, sec, "/api/public/scores",
		map[string]any{"traceId": "t1", "name": "quality", "value": 5})
	assert.Equal(t, http.StatusBadRequest, code, "body: %v", body)

	// Wrong category rejected; right one passes.
	code, _ = postPublic(t, app, pub, sec, "/api/public/scores",
		map[string]any{"traceId": "t1", "name": "label", "value": "meh"})
	assert.Equal(t, http.StatusBadRequest, code)
	code, _ = postPublic(t, app, pub, sec, "/api/public/scores",
		map[string]any{"traceId": "t1", "name": "label", "value": "good"})
	assert.Equal(t, http.StatusOK, code)

	// Undeclared names stay schemaless.
	code, _ = postPublic(t, app, pub, sec, "/api/public/scores",
		map[string]any{"traceId": "t1", "name": "freeform", "value": "anything"})
	assert.Equal(t, http.StatusOK, code)

	// Public config discovery works for runners.
	code, cfgs := getPublic(t, app, pub, sec, "/api/public/score-configs")
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, cfgs["data"], 2)
}

func TestAnnotationQueueFlow(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "aq@example.com")
	projectID := createProject(t, app, token, orgID, "aq-proj")
	pub, sec := createKey(t, app, token, projectID)
	base := fmt.Sprintf("/api/v1/projects/%s", projectID)

	// Seed a trace to review.
	code, _ := postIngestion(t, app, pub, sec, `{"batch":[
		{"id":"e1","type":"trace-create","timestamp":"2026-01-01T00:00:00Z","body":{"id":"rev-t1","name":"chat"}}
	]}`)
	require.Equal(t, http.StatusMultiStatus, code)
	drain(t, app, projectID)

	// Queue + item.
	code, q := postUI(t, app, token, base+"/annotation-queues", map[string]any{"name": "review"})
	require.Equal(t, http.StatusCreated, code, "body: %v", q)
	qid := q["id"].(string)
	code, _ = postUI(t, app, token, base+"/annotation-queues/"+qid, map[string]any{})
	assert.Equal(t, http.StatusMethodNotAllowed, code, "POST to queue detail is not routed")

	code, item := postUI(t, app, token, base+"/annotation-queues/"+qid+"/items",
		map[string]any{"traceId": "rev-t1"})
	require.Equal(t, http.StatusCreated, code, "body: %v", item)
	itemID := item["id"].(string)

	// Unknown trace rejected.
	code, _ = postUI(t, app, token, base+"/annotation-queues/"+qid+"/items",
		map[string]any{"traceId": "nope"})
	assert.Equal(t, http.StatusBadRequest, code)

	// Reviewer scores the trace (ANNOTATION source).
	code, _ = postUI(t, app, token,
		base+"/annotation-queues/"+qid+"/items/"+itemID+"/scores",
		map[string]any{"name": "review-q", "value": 1})
	require.Equal(t, http.StatusCreated, code)

	code, scores := getUI(t, app, token, base+"/scores?traceId=rev-t1")
	require.Equal(t, http.StatusOK, code)
	sdata, _ := scores["data"].([]any)
	require.Len(t, sdata, 1)
	assert.Equal(t, "review-q", sdata[0].(map[string]any)["name"])

	// Complete the item; pending count drops.
	code, _ = postUI(t, app, token,
		base+"/annotation-queues/"+qid+"/items/"+itemID+"/complete", map[string]any{})
	require.Equal(t, http.StatusOK, code)
	code, got := getUI(t, app, token, base+"/annotation-queues/"+qid+"?status=PENDING")
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, got["data"].(map[string]any)["items"])

	// Queue list shows totals.
	code, queues := getUI(t, app, token, base+"/annotation-queues")
	require.Equal(t, http.StatusOK, code)
	qdata, _ := queues["data"].([]any)
	require.Len(t, qdata, 1)
	assert.Equal(t, float64(1), qdata[0].(map[string]any)["total"])
	assert.Equal(t, float64(0), qdata[0].(map[string]any)["pending"])
}

func TestEvalIsolation(t *testing.T) {
	app := newTestApp(t)
	tokenA, orgA := register(t, app, "ea@example.com")
	tokenB, _ := register(t, app, "eb@example.com")
	projA := createProject(t, app, tokenA, orgA, "a")

	code, _ := postUI(t, app, tokenB,
		fmt.Sprintf("/api/v1/projects/%s/score-configs", projA),
		map[string]any{"name": "q", "dataType": "NUMERIC"})
	assert.Contains(t, []int{http.StatusForbidden, http.StatusNotFound}, code)
}
