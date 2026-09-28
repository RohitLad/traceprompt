package httpapi

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/traceprompt/traceprompt/backend/internal/models"
)

// Covers previously untested list/detail paths and validation edges.
func TestListAndDetailCoverage(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "cov@example.com")
	projectID := createProject(t, app, token, orgID, "cov-proj")
	base := fmt.Sprintf("/api/v1/projects/%s", projectID)

	// Projects list + detail shape.
	code, body := getUI(t, app, token, "/api/v1/projects")
	require.Equal(t, http.StatusOK, code, "body: %v", body)
	assert.Len(t, body["data"], 1)
	code, body = getUI(t, app, token, base)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "cov-proj", body["data"].(map[string]any)["name"])

	// Datasets list with counts.
	code, _ = postUI(t, app, token, base+"/datasets", map[string]any{"name": "d1"})
	require.Equal(t, http.StatusCreated, code)
	code, body = getUI(t, app, token, base+"/datasets")
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, body["data"], 1)

	// Prompts list summary (no versions yet → productionVersion null).
	code, _ = postUI(t, app, token, base+"/prompts",
		map[string]any{"name": "p1", "type": "text", "template": "Hi"})
	require.Equal(t, http.StatusCreated, code)
	code, body = getUI(t, app, token, base+"/prompts")
	require.Equal(t, http.StatusOK, code)
	pdata, _ := body["data"].([]any)
	require.Len(t, pdata, 1)
	assert.Equal(t, float64(1), pdata[0].(map[string]any)["versions"])
	assert.Equal(t, float64(1), pdata[0].(map[string]any)["productionVersion"],
		"new prompts default to production v1")

	// Score configs list.
	code, _ = postUI(t, app, token, base+"/score-configs",
		map[string]any{"name": "q", "dataType": "NUMERIC"})
	require.Equal(t, http.StatusCreated, code)
	code, body = getUI(t, app, token, base+"/score-configs")
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, body["data"], 1)

	// Queues list (empty).
	code, body = getUI(t, app, token, base+"/annotation-queues")
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, body["data"])

	// Dataset runs list (empty) + run detail 404.
	code, body = getUI(t, app, token, base+"/datasets/"+datasetID(t, app, token, base, "d1")+"/runs")
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, body["data"])
	code, _ = getUI(t, app, token, base+"/runs/00000000-0000-0000-0000-000000000000")
	assert.Equal(t, http.StatusNotFound, code)
}

func datasetID(t *testing.T, app *fiberApp, token, base, name string) string {
	t.Helper()
	_, body := getUI(t, app, token, base+"/datasets")
	for _, d := range body["data"].([]any) {
		if d.(map[string]any)["name"] == name {
			return d.(map[string]any)["id"].(string)
		}
	}
	t.Fatalf("dataset %s not found", name)
	return ""
}

func TestKeyValidationEdges(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "kv@example.com")
	projectID := createProject(t, app, token, orgID, "kv-proj")
	base := fmt.Sprintf("/api/v1/projects/%s", projectID)

	code, _ := postUI(t, app, token, base+"/keys", map[string]any{"name": ""})
	assert.Equal(t, http.StatusBadRequest, code)

	code, _ = postUI(t, app, token, base+"/keys/not-a-uuid/revoke", nil)
	assert.Equal(t, http.StatusBadRequest, code, "malformed key id is 400, not 500")
}

func TestPromptVersionFallbackCopiesPrevious(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "vf@example.com")
	projectID := createProject(t, app, token, orgID, "vf-proj")
	base := fmt.Sprintf("/api/v1/projects/%s", projectID)

	code, _ := postUI(t, app, token, base+"/prompts",
		map[string]any{"name": "p", "type": "text", "template": "v1"})
	require.Equal(t, http.StatusCreated, code)

	// Empty payload copies v1 content into v2 (config-only change support).
	code, v2 := postUI(t, app, token, base+"/prompts/p/versions",
		map[string]any{"config": map[string]any{"temp": 0.5}})
	require.Equal(t, http.StatusCreated, code, "body: %v", v2)
	assert.Equal(t, float64(2), v2["version"])
}

func TestOtelBadBodyAndScoresPagination(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "oe@example.com")
	projectID := createProject(t, app, token, orgID, "oe-proj")
	pub, sec := createKey(t, app, token, projectID)
	base := fmt.Sprintf("/api/v1/projects/%s", projectID)

	code, _ := postPublic(t, app, pub, sec, "/api/public/otel/v1/traces",
		map[string]any{"resourceSpans": "nope"})
	assert.Equal(t, http.StatusBadRequest, code)

	// Seed 3 scores, page through UI list with limit=2.
	for _, v := range []int{1, 2, 3} {
		code, _ = postPublic(t, app, pub, sec, "/api/public/scores",
			map[string]any{"traceId": "t", "name": fmt.Sprintf("s%d", v), "value": v})
		require.Equal(t, http.StatusOK, code)
	}
	code, page1 := getUI(t, app, token, base+"/scores?limit=2")
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, page1["data"], 2)
	cursor, _ := page1["meta"].(map[string]any)["cursor"].(string)
	require.NotEmpty(t, cursor)
	code, page2 := getUI(t, app, token, base+"/scores?limit=2&cursor="+cursor)
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, page2["data"], 1)
}

func TestMetricsEmptyProject(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "me@example.com")
	projectID := createProject(t, app, token, orgID, "me-proj")

	code, body := getUI(t, app, token, fmt.Sprintf("/api/v1/projects/%s/metrics/overview", projectID))
	require.Equal(t, http.StatusOK, code, "body: %v", body)
	data := body["data"].(map[string]any)
	assert.Equal(t, float64(0), data["traces"])
	assert.Nil(t, data["avgLatencyMs"])
	assert.Empty(t, data["perDay"])
	assert.Empty(t, data["byModel"])
	assert.False(t, data["truncated"].(bool))
}

func TestSessionsEmptyAndScoreConfigRef(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "se@example.com")
	projectID := createProject(t, app, token, orgID, "se-proj")
	pub, sec := createKey(t, app, token, projectID)
	base := fmt.Sprintf("/api/v1/projects/%s", projectID)

	code, body := getUI(t, app, token, base+"/sessions")
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, body["data"])

	// configId mismatch: score name must equal config name.
	code, cfg := postUI(t, app, token, base+"/score-configs",
		map[string]any{"name": "strict", "dataType": "NUMERIC", "minValue": 0, "maxValue": 1})
	require.Equal(t, http.StatusCreated, code)
	code, _ = postPublic(t, app, pub, sec, "/api/public/scores",
		map[string]any{"traceId": "t", "name": "other", "value": 0.5, "configId": cfg["id"]})
	assert.Equal(t, http.StatusBadRequest, code)
	code, _ = postPublic(t, app, pub, sec, "/api/public/scores",
		map[string]any{"traceId": "t", "name": "strict", "value": 0.5, "configId": cfg["id"]})
	assert.Equal(t, http.StatusOK, code)
}

func TestCreateProjectOrgFallback(t *testing.T) {
	app := newTestApp(t)
	token, orgID := register(t, app, "fb@example.com")

	// Fresh account, zero projects: name-only creation lands in their org.
	code, body := postUI(t, app, token, "/api/v1/projects", map[string]any{"name": "first"})
	require.Equal(t, http.StatusCreated, code, "body: %v", body)
	assert.Equal(t, orgID, body["organizationId"])

	// Explicit org still works and wins.
	code, body = postUI(t, app, token, "/api/v1/projects",
		map[string]any{"name": "second", "organizationId": orgID})
	require.Equal(t, http.StatusCreated, code, "body: %v", body)

	// Unknown org is forbidden, not a silent misfile.
	// (An explicit zero UUID counts as omitted — same as absent.)
	code, _ = postUI(t, app, token, "/api/v1/projects",
		map[string]any{"name": "x", "organizationId": "11111111-1111-1111-1111-111111111111"})
	assert.Equal(t, http.StatusForbidden, code)
}

func TestCreateProjectMultiOrgRequiresChoice(t *testing.T) {
	app := newTestApp(t)
	tokenA, _ := register(t, app, "ma@example.com")
	_, orgB := register(t, app, "mb@example.com")

	// Make A a member of B's org too (direct insert mirrors an invite flow).
	var userA models.User
	require.NoError(t, app.db.First(&userA, "email = ?", "ma@example.com").Error)
	orgBID, err := uuid.Parse(orgB)
	require.NoError(t, err)
	require.NoError(t, app.db.Create(&models.Membership{
		UserID: userA.ID, OrganizationID: orgBID, Role: "member",
	}).Error)

	code, _ := postUI(t, app, tokenA, "/api/v1/projects", map[string]any{"name": "ambiguous"})
	assert.Equal(t, http.StatusBadRequest, code, "multi-org callers must choose")

	code, _ = postUI(t, app, tokenA, "/api/v1/projects",
		map[string]any{"name": "placed", "organizationId": orgB})
	assert.Equal(t, http.StatusCreated, code)
}
