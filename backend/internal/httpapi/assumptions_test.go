package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/traceprompt/traceprompt/backend/internal/auth"
	"github.com/traceprompt/traceprompt/backend/internal/ingest"
	"github.com/traceprompt/traceprompt/backend/internal/models"
)

// Assumption tests: each pins one implicit contract of the stack so a
// future change breaks a test instead of a user. If any of these feels
// wrong, change the code AND the test together — never just the test.

// A user row without a membership can never log in (register always creates
// all three atomically; manual provisioning must include membership).
func TestLoginWithoutMembership401(t *testing.T) {
	app := newTestApp(t)
	hash, err := auth.HashPassword("password-123")
	require.NoError(t, err)
	require.NoError(t, app.db.Create(&models.User{
		Email: "lonely@example.com", Name: "Lonely", PasswordHash: hash,
	}).Error)

	code, body := doRequest(t, app, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": "lonely@example.com", "password": "password-123"}, nil)
	assert.Equal(t, http.StatusUnauthorized, code, "body: %v", body)
}

// observation-update for an unknown id silently creates (upsert semantics).
func TestObservationUpdateCreatesWhenMissing(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "up@example.com")
	projectID := createProject(t, app, token, orgID, "up")
	pid := mustProjectID(t, projectID)
	store := ingest.NewStore(app.db)

	parsed, errs := ingest.Parse([]byte(`{"batch":[
		{"id":"e1","type":"observation-update","timestamp":"2026-01-01T00:00:00Z",
		 "body":{"id":"ghost","traceId":"ghost-t","output":"hi"}}
	]}`))
	require.Empty(t, errs)
	require.NoError(t, store.Apply(context.Background(), pid, parsed[0]))

	var n int64
	require.NoError(t, app.db.Model(&models.Observation{}).
		Where("project_id = ? AND observation_id = ?", pid, "ghost").Count(&n).Error)
	assert.Equal(t, int64(1), n)
}

// Scores without an external id are NOT idempotent: at-least-once redelivery
// duplicates them. Callers needing exactly-once must send stable ids.
func TestNilScoreIDDuplicatesOnRetry(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "dup@example.com")
	projectID := createProject(t, app, token, orgID, "dup")
	pid := mustProjectID(t, projectID)
	store := ingest.NewStore(app.db)

	parsed, errs := ingest.Parse([]byte(`{"batch":[
		{"id":"e1","type":"score-create","timestamp":"2026-01-01T00:00:00Z",
		 "body":{"traceId":"t","name":"q","value":1}}
	]}`))
	require.Empty(t, errs)
	require.NoError(t, store.Apply(context.Background(), pid, parsed[0]))
	require.NoError(t, store.Apply(context.Background(), pid, parsed[0]))

	var n int64
	require.NoError(t, app.db.Model(&models.Score{}).Count(&n).Error)
	assert.Equal(t, int64(2), n, "nil external ids duplicate on redelivery by design")
}

// Empty OTLP payload is a successful no-op, not a 400.
func TestOtelEmptySpans200(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "oe2@example.com")
	projectID := createProject(t, app, token, orgID, "oe2")
	pub, sec := createKey(t, app, token, projectID)

	code, body := postPublic(t, app, pub, sec, "/api/public/otel/v1/traces",
		map[string]any{"resourceSpans": []any{}})
	assert.Equal(t, http.StatusOK, code, "body: %v", body)
}

// Garbage cursors fall back to the first page instead of 400.
func TestBadCursorFallsBack(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "cur@example.com")
	projectID := createProject(t, app, token, orgID, "cur")
	pub, sec := createKey(t, app, token, projectID)

	code, body := getPublic(t, app, pub, sec, "/api/public/v2/observations?cursor=!!!&limit=5")
	assert.Equal(t, http.StatusOK, code, "body: %v", body)
	assert.Empty(t, body["data"])
}

// BOOLEAN scores serialize as JSON booleans on the v3 API.
func TestBooleanScoreV3Serialization(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "bv@example.com")
	projectID := createProject(t, app, token, orgID, "bv")
	pub, sec := createKey(t, app, token, projectID)

	code, _ := postPublic(t, app, pub, sec, "/api/public/scores",
		map[string]any{"traceId": "t", "name": "ok", "value": 1, "dataType": "BOOLEAN"})
	require.Equal(t, http.StatusOK, code)
	code, scores := getPublic(t, app, pub, sec, "/api/public/v3/scores?traceId=t")
	require.Equal(t, http.StatusOK, code)
	sdata := scores["data"].([]any)
	require.Len(t, sdata, 1)
	assert.Equal(t, true, sdata[0].(map[string]any)["value"])
	assert.Equal(t, "BOOLEAN", sdata[0].(map[string]any)["dataType"])
}

// A configId from another project is rejected, not applied cross-tenant.
func TestForeignConfigIDRejected(t *testing.T) {
	app := newTestApp(t)
	tokenA, orgA := register(t, app, "fa@example.com")
	tokenB, orgB := register(t, app, "fb@example.com")
	projA := createProject(t, app, tokenA, orgA, "a")
	projB := createProject(t, app, tokenB, orgB, "b")
	pubB, secB := createKey(t, app, tokenB, projB)

	code, cfg := postUI(t, app, tokenA, fmt.Sprintf("/api/v1/projects/%s/score-configs", projA),
		map[string]any{"name": "q", "dataType": "NUMERIC"})
	require.Equal(t, http.StatusCreated, code)
	code, _ = postPublic(t, app, pubB, secB, "/api/public/scores",
		map[string]any{"traceId": "t", "name": "q", "value": 0.5, "configId": cfg["id"]})
	assert.Equal(t, http.StatusBadRequest, code)
}

// days=0 clamps to 1, huge values clamp to 365 — never 400, never unbounded.
func TestMetricsDaysClamped(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "mc@example.com")
	projectID := createProject(t, app, token, orgID, "mc")

	code, body := getUI(t, app, token, fmt.Sprintf("/api/v1/projects/%s/metrics/overview?days=0", projectID))
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, float64(1), body["data"].(map[string]any)["days"])
	code, body = getUI(t, app, token, fmt.Sprintf("/api/v1/projects/%s/metrics/overview?days=99999", projectID))
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, float64(365), body["data"].(map[string]any)["days"])
}

// Unknown prompt labels 404; latest label resolves.
func TestPublicPromptLabelMiss(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "pl@example.com")
	projectID := createProject(t, app, token, orgID, "pl")
	pub, sec := createKey(t, app, token, projectID)
	base := fmt.Sprintf("/api/v1/projects/%s", projectID)

	code, _ := postUI(t, app, token, base+"/prompts",
		map[string]any{"name": "p", "type": "text", "template": "v1"})
	require.Equal(t, http.StatusCreated, code)
	code, body := getPublic(t, app, pub, sec, "/api/public/prompts/p?label=staging")
	assert.Equal(t, http.StatusNotFound, code, "body: %v", body)
	code, body = getPublic(t, app, pub, sec, "/api/public/prompts/p?label=latest")
	require.Equal(t, http.StatusOK, code, "body: %v", body)
}

// Unknown queue status filter is an empty 200, not an error.
func TestQueueInvalidStatusFilterEmpty(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "qf@example.com")
	projectID := createProject(t, app, token, orgID, "qf")

	code, q := postUI(t, app, token, fmt.Sprintf("/api/v1/projects/%s/annotation-queues", projectID),
		map[string]any{"name": "q"})
	require.Equal(t, http.StatusCreated, code)
	code, got := getUI(t, app, token,
		fmt.Sprintf("/api/v1/projects/%s/annotation-queues/%s?status=BOGUS", projectID, q["id"]))
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, got["data"].(map[string]any)["items"])
}

// Re-revoking a key is a 200 no-op, and revoked keys stay listed.
func TestRevokeIdempotent(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "rk@example.com")
	projectID := createProject(t, app, token, orgID, "rk")

	pub, _ := createKey(t, app, token, projectID)
	code, keys := getUI(t, app, token, fmt.Sprintf("/api/v1/projects/%s/keys", projectID))
	require.Equal(t, http.StatusOK, code)
	kid := keys["data"].([]any)[0].(map[string]any)["id"].(string)

	path := fmt.Sprintf("/api/v1/projects/%s/keys/%s/revoke", projectID, kid)
	code, _ = postUI(t, app, token, path, map[string]any{})
	require.Equal(t, http.StatusOK, code)
	code, body := postUI(t, app, token, path, map[string]any{})
	require.Equal(t, http.StatusOK, code, "re-revoke is idempotent")
	assert.NotNil(t, body["revokedAt"])

	code, _ = getPublic(t, app, pub, "sk-lf-wrong", "/api/public/projects")
	assert.Equal(t, http.StatusUnauthorized, code)
}

// Run links don't require the trace to exist (link now, ingest later).
func TestRunLinkWithoutTraceAllowed(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "rl@example.com")
	projectID := createProject(t, app, token, orgID, "rl")
	base := fmt.Sprintf("/api/v1/projects/%s", projectID)

	code, ds := postUI(t, app, token, base+"/datasets", map[string]any{"name": "d"})
	require.Equal(t, http.StatusCreated, code)
	code, item := postUI(t, app, token, base+"/datasets/"+ds["id"].(string)+"/items",
		map[string]any{"input": "q"})
	require.Equal(t, http.StatusCreated, code)
	code, run := postUI(t, app, token, base+"/datasets/"+ds["id"].(string)+"/runs",
		map[string]any{"name": "r"})
	require.Equal(t, http.StatusCreated, code)
	code, _ = postUI(t, app, token, base+"/runs/"+run["id"].(string)+"/items",
		map[string]any{"itemId": item["id"], "traceId": "not-yet-ingested"})
	assert.Equal(t, http.StatusCreated, code)
}

// Empty-string sessionIds never surface as sessions.
func TestEmptySessionExcluded(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "es@example.com")
	projectID := createProject(t, app, token, orgID, "es")
	pub, sec := createKey(t, app, token, projectID)

	code, _ := postIngestion(t, app, pub, sec, `{"batch":[
		{"id":"e1","type":"trace-create","timestamp":"2026-01-01T00:00:00Z",
		 "body":{"id":"t1","name":"t","sessionId":""}}
	]}`)
	require.Equal(t, http.StatusMultiStatus, code)
	drain(t, app, projectID)

	code, sessions := getUI(t, app, token, fmt.Sprintf("/api/v1/projects/%s/sessions", projectID))
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, sessions["data"])
}

// Observation updates silently ignore usage/environment/prompt fields.
func TestUpdateIgnoresUsageDocumented(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "uu@example.com")
	projectID := createProject(t, app, token, orgID, "uu")
	pid := mustProjectID(t, projectID)
	store := ingest.NewStore(app.db)
	ctx := context.Background()

	apply := func(typ, body string) {
		t.Helper()
		parsed, errs := ingest.Parse([]byte(
			`{"batch":[{"id":"e1","type":"` + typ + `","timestamp":"2026-01-01T00:00:00Z","body":` + body + `}]}`))
		require.Empty(t, errs)
		require.NoError(t, store.Apply(ctx, pid, parsed[0]))
	}
	apply("generation-create", `{"id":"o","traceId":"t","model":"m1","usage":{"input":5,"output":5,"total":10}}`)
	apply("observation-update", `{"id":"o","model":"m2","usage":{"input":99,"output":99,"total":198},"environment":"staging"}`)

	var o models.Observation
	require.NoError(t, app.db.First(&o, "observation_id = ?", "o").Error)
	assert.Equal(t, "m2", *o.Model, "model IS updated")
	assert.Equal(t, 5, *o.UsageInput, "usage is NOT updated (documented silent drop)")
	assert.Equal(t, "default", o.Environment, "environment is NOT updated (documented silent drop)")
}

// Malformed (non-base64) BasicAuth is a clean 401, not a 500.
func TestMalformedBasicAuth401(t *testing.T) {
	app := newTestApp(t)
	code, _ := doRequest(t, app, http.MethodGet, "/api/public/projects", nil,
		map[string]string{"Authorization": "Basic !!!not-base64!!!"})
	assert.Equal(t, http.StatusUnauthorized, code)
}

// Unknown dataset via public API is 404.
func TestGetPublicDataset404(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "pd@example.com")
	projectID := createProject(t, app, token, orgID, "pd")
	pub, sec := createKey(t, app, token, projectID)

	code, _ := getPublic(t, app, pub, sec, "/api/public/datasets/nope")
	assert.Equal(t, http.StatusNotFound, code)
}
