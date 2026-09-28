package playground

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRender(t *testing.T) {
	assert.Equal(t, "Hi Ada!", Render("Hi {{name}}!", map[string]string{"name": "Ada"}))
	assert.Equal(t, "Hi  !", Render("Hi {{ name }} !", nil), "missing vars render empty")
	assert.Equal(t, "no vars", Render("no vars", map[string]string{"a": "b"}))
}

func TestMissing(t *testing.T) {
	assert.Equal(t, []string{"name", "day"}, Missing("Hi {{name}}, {{day}} {{name}}", map[string]string{}))
	assert.Empty(t, Missing("Hi {{name}}", map[string]string{"name": "x"}))
}

func mockProvider(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		var req ChatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.NotEmpty(t, req.Model)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRunSuccess(t *testing.T) {
	srv := mockProvider(t, 200, `{"model":"gpt-4o-mini",
		"choices":[{"message":{"role":"assistant","content":"Hello!"}}],
		"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`)
	res, err := NewClient().Run(context.Background(), srv.URL, "test-key", "gpt-4o-mini",
		[]Message{{Role: "user", Content: "Hi"}})
	require.NoError(t, err)
	assert.Equal(t, "Hello!", res.Output)
	assert.Equal(t, 5, res.InputTokens)
	assert.Equal(t, 3, res.OutputTokens)
}

func TestRunFailures(t *testing.T) {
	_, err := NewClient().Run(context.Background(), "", "k", "m", nil)
	assert.Error(t, err, "missing baseUrl rejected")

	srv := mockProvider(t, 401, `{"error":"bad key"}`)
	_, err = NewClient().Run(context.Background(), srv.URL, "test-key", "m",
		[]Message{{Role: "user", Content: "Hi"}})
	assert.ErrorContains(t, err, "401")

	empty := mockProvider(t, 200, `{"choices":[]}`)
	_, err = NewClient().Run(context.Background(), empty.URL, "test-key", "m",
		[]Message{{Role: "user", Content: "Hi"}})
	assert.ErrorContains(t, err, "no choices")
}
