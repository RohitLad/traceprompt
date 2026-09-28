package otel

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/traceprompt/traceprompt/backend/internal/ingest"
)

const sampleOTLP = `{"resourceSpans":[{
	"resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "demo"}}]},
	"scopeSpans": [{"spans": [
		{"traceId": "aaaabbbbccccddddeeeeffff00001111", "spanId": "1111222233334444", "name": "chat",
		 "startTimeUnixNano": "1760000000000000000", "endTimeUnixNano": "1760000001000000000",
		 "attributes": [
			{"key": "langfuse.user.id", "value": {"stringValue": "u1"}},
			{"key": "langfuse.tags", "value": {"arrayValue": {"values": [{"stringValue": "web"}]}}},
			{"key": "langfuse.metadata.tenant", "value": {"stringValue": "acme"}}
		 ]},
		{"traceId": "aaaabbbbccccddddeeeeffff00001111", "spanId": "5555666677778888", "parentSpanId": "1111222233334444",
		 "name": "llm", "startTimeUnixNano": "1760000000000000000", "endTimeUnixNano": "1760000001000000000",
		 "attributes": [
			{"key": "langfuse.observation.type", "value": {"stringValue": "generation"}},
			{"key": "gen_ai.request.model", "value": {"stringValue": "gpt-4o"}},
			{"key": "gen_ai.usage.input_tokens", "value": {"intValue": "10"}},
			{"key": "langfuse.observation.input", "value": {"stringValue": "hi"}}
		 ]}
	]}]
}]}`

func TestToEvents(t *testing.T) {
	evs, errs := ToEvents(json.RawMessage(sampleOTLP))
	require.Empty(t, errs)
	// Root span emits trace-create + observation-create; child emits observation-create.
	require.Len(t, evs, 3)
	assert.Equal(t, ingest.TypeTraceCreate, evs[0].Type)
	assert.Equal(t, ingest.TypeObservationCreate, evs[1].Type)
	assert.Equal(t, ingest.TypeObservationCreate, evs[2].Type)

	var traceBody map[string]any
	require.NoError(t, json.Unmarshal(evs[0].Body, &traceBody))
	assert.Equal(t, "aaaabbbbccccddddeeeeffff00001111", traceBody["id"])
	assert.Equal(t, "chat", traceBody["name"])
	assert.Equal(t, "u1", traceBody["userId"])
	assert.Equal(t, []any{"web"}, traceBody["tags"])
	meta, _ := traceBody["metadata"].(map[string]any)
	assert.Equal(t, "acme", meta["tenant"], "langfuse.metadata.* flattened")
	assert.Equal(t, "demo", meta["service.name"], "unknown attrs preserved")

	var obsBody map[string]any
	require.NoError(t, json.Unmarshal(evs[2].Body, &obsBody))
	assert.Equal(t, "GENERATION", obsBody["type"], "observation type uppercased")
	assert.Equal(t, "gpt-4o", obsBody["model"])
	usage, _ := obsBody["usage"].(map[string]any)
	assert.Equal(t, float64(10), usage["input"])
	assert.Equal(t, "hi", obsBody["input"])
}

func TestToEventsRejectsBadSpans(t *testing.T) {
	evs, errs := ToEvents(json.RawMessage(`{"resourceSpans":[{
		"scopeSpans": [{"spans": [
			{"traceId": "", "spanId": "", "name": "bad"},
			{"traceId": "abc", "spanId": "def", "name": "good",
			 "startTimeUnixNano": "not-a-number",
			 "attributes": [{"key": "x", "value": {"boolValue": true}}]}
		]}]}]}`))
	require.Len(t, errs, 1, "span without ids rejected")
	require.Len(t, evs, 2, "good span still emits trace + observation (bad timestamp falls back)")
}

func TestToEventsRejectsBadJSON(t *testing.T) {
	_, errs := ToEvents(json.RawMessage(`{oops`))
	require.Len(t, errs, 1)
}
