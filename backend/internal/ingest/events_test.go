package ingest

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseValidBatch(t *testing.T) {
	raw := json.RawMessage(`{"batch":[
		{"id":"e1","type":"trace-create","timestamp":"2026-01-01T00:00:00Z","body":{"id":"t1","name":"chat"}},
		{"id":"e2","type":"generation-create","timestamp":"2026-01-01T00:00:01Z","body":{"id":"o1","traceId":"t1","name":"llm","model":"gpt-4o"}},
		{"id":"e3","type":"score-create","timestamp":"2026-01-01T00:00:02Z","body":{"traceId":"t1","name":"quality","value":0.9}}
	]}`)
	parsed, errs := Parse(raw)
	require.Empty(t, errs)
	require.Len(t, parsed, 3)
	assert.Equal(t, TypeTraceCreate, parsed[0].Type)
	// Alias normalized, default obs type preserved.
	assert.Equal(t, TypeObservationCreate, parsed[1].Type)
	assert.Equal(t, "GENERATION", parsed[1].ObsType)
	assert.Equal(t, TypeScoreCreate, parsed[2].Type)
}

func TestParseCollectsPerEventErrors(t *testing.T) {
	raw := json.RawMessage(`{"batch":[
		{"id":"e1","type":"trace-create","timestamp":"2026-01-01T00:00:00Z","body":{"id":"t1"}},
		{"id":"e2","type":"nope","timestamp":"2026-01-01T00:00:00Z","body":{"id":"x"}},
		{"id":"","type":"trace-create","timestamp":"2026-01-01T00:00:00Z","body":{"id":"t2"}},
		{"id":"e4","type":"score-create","timestamp":"2026-01-01T00:00:00Z","body":{"name":"q"}}
	]}`)
	parsed, errs := Parse(raw)
	assert.Len(t, parsed, 1)
	require.Len(t, errs, 3)
	assert.Equal(t, "e2", errs[0].ID)
	assert.Equal(t, 400, errs[0].Status)
}

func TestParseRejectsOversizeAndBadJSON(t *testing.T) {
	_, errs := Parse(json.RawMessage(`not json`))
	require.Len(t, errs, 1)

	big := `{"batch":[`
	for i := 0; i < MaxBatchEvents+1; i++ {
		if i > 0 {
			big += ","
		}
		big += `{"id":"e` + string(rune('0'+i%10)) + `","type":"trace-create","body":{"id":"t"}}`
	}
	big += `]}`
	_, errs = Parse(json.RawMessage(big))
	require.Len(t, errs, 1)
	assert.Equal(t, 413, errs[0].Status)
}

func TestParseAcceptsLegacyTimestamps(t *testing.T) {
	raw := json.RawMessage(`{"batch":[
		{"id":"e1","type":"trace-create","timestamp":"2026-01-01 10:00:00","body":{"id":"t1"}}
	]}`)
	parsed, errs := Parse(raw)
	require.Empty(t, errs)
	require.Len(t, parsed, 1)
}
