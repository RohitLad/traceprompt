// Package otel translates OTLP/HTTP trace payloads into ingestion events.
//
// Endpoint: POST /api/public/otel/v1/traces (BasicAuth pk:sk).
// Accepts the OTLP/HTTP JSON encoding (resourceSpans/scopeSpans/spans)
// and maps spans onto trace-create + observation-create events, reusing
// the idempotent store path. Responds 200 with {} (OTLP success).
//
// Attribute mapping (documented contract, Langfuse-inspired):
//
//	OTel identity            → traceId/observationId/parentObservationId
//	langfuse.trace.name      → trace name (else root span name)
//	langfuse.user.id         → trace userId
//	langfuse.session.id      → trace sessionId
//	langfuse.tags            → trace tags (array or comma-separated)
//	langfuse.environment     → observation environment
//	langfuse.observation.type→ observation type (else SPAN)
//	langfuse.observation.input/output → input/output (JSON-encoded)
//	langfuse.model.name, gen_ai.request.model, gen_ai.response.model → model
//	langfuse.usage.input/output/total, gen_ai.usage.input_tokens/output_tokens → usage
//	langfuse.prompt.name/version → prompt link
//	langfuse.level           → level (OTel ERROR status also maps to ERROR)
//	langfuse.metadata.*      → metadata entries (rest of attrs also land in metadata)
package otel

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/traceprompt/traceprompt/backend/internal/ingest"
)

// AnyValue is the OTLP attribute value union.
type AnyValue struct {
	StringValue *string       `json:"stringValue"`
	IntValue    *string       `json:"intValue"`
	DoubleValue *float64      `json:"doubleValue"`
	BoolValue   *bool         `json:"boolValue"`
	ArrayValue  *ArrayValue   `json:"arrayValue"`
	KvlistValue *KeyValueList `json:"kvlistValue"`
	BytesValue  *string       `json:"bytesValue"`
}

// ArrayValue holds repeated values.
type ArrayValue struct {
	Values []AnyValue `json:"values"`
}

// KeyValueList holds nested maps.
type KeyValueList struct {
	Values []KeyValue `json:"values"`
}

// KeyValue is one OTLP attribute.
type KeyValue struct {
	Key   string   `json:"key"`
	Value AnyValue `json:"value"`
}

// Span is one OTLP span.
type Span struct {
	TraceID           string      `json:"traceId"`
	SpanID            string      `json:"spanId"`
	ParentSpanID      string      `json:"parentSpanId"`
	Name              string      `json:"name"`
	StartTimeUnixNano string      `json:"startTimeUnixNano"`
	EndTimeUnixNano   string      `json:"endTimeUnixNano"`
	Attributes        []KeyValue  `json:"attributes"`
	Status            *SpanStatus `json:"status"`
}

// SpanStatus carries OTLP status (2 = ERROR).
type SpanStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// TracesRequest is the OTLP/HTTP JSON envelope.
type TracesRequest struct {
	ResourceSpans []struct {
		Resource *struct {
			Attributes []KeyValue `json:"attributes"`
		} `json:"resource"`
		ScopeSpans []struct {
			Spans []Span `json:"spans"`
		} `json:"scopeSpans"`
	} `json:"resourceSpans"`
}

// ToEvents converts an OTLP payload into ingestion events.
// Root spans (no parent) additionally emit trace-create; every span
// emits observation-create. Invalid spans are skipped with per-span errors.
func ToEvents(raw json.RawMessage) ([]ingest.Parsed, []ingest.EventError) {
	var req TracesRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, []ingest.EventError{{ID: "", Status: 400, Message: "invalid OTLP json"}}
	}
	var out []ingest.Parsed
	var errs []ingest.EventError
	now := time.Now().UTC()
	for _, rs := range req.ResourceSpans {
		resourceAttrs := map[string]AnyValue{}
		if rs.Resource != nil {
			for _, kv := range rs.Resource.Attributes {
				resourceAttrs[kv.Key] = kv.Value
			}
		}
		for _, ss := range rs.ScopeSpans {
			for _, sp := range ss.Spans {
				evs, err := spanToEvents(sp, resourceAttrs, now)
				if err != nil {
					errs = append(errs, ingest.EventError{ID: sp.SpanID, Status: 400, Message: err.Error()})
					continue
				}
				out = append(out, evs...)
			}
		}
	}
	return out, errs
}

func spanToEvents(sp Span, resourceAttrs map[string]AnyValue, now time.Time) ([]ingest.Parsed, error) {
	if sp.TraceID == "" || sp.SpanID == "" {
		return nil, fmt.Errorf("span requires traceId and spanId")
	}
	attrs := map[string]AnyValue{}
	for k, v := range resourceAttrs {
		attrs[k] = v
	}
	for _, kv := range sp.Attributes {
		attrs[kv.Key] = kv.Value // span attributes win over resource
	}
	get := func(key string) string {
		if v, ok := attrs[key]; ok {
			return anyToString(v)
		}
		return ""
	}

	start := nanoToTime(sp.StartTimeUnixNano, now)
	end := nanoToTime(sp.EndTimeUnixNano, now)

	var out []ingest.Parsed
	if sp.ParentSpanID == "" {
		// Root span defines the trace.
		traceName := firstNonEmpty(get("langfuse.trace.name"), sp.Name)
		traceBody, _ := json.Marshal(map[string]any{
			"id":        sp.TraceID,
			"name":      traceName,
			"userId":    nullIfEmpty(get("langfuse.user.id")),
			"sessionId": nullIfEmpty(get("langfuse.session.id")),
			"tags":      parseTags(attrs["langfuse.tags"]),
			"metadata":  leftoverMetadata(attrs),
		})
		out = append(out, ingest.Parsed{
			EventID: "otel-" + sp.TraceID, Type: ingest.TypeTraceCreate,
			Timestamp: start, Body: traceBody,
		})
	}

	obsType := firstNonEmpty(strings.ToUpper(get("langfuse.observation.type")), "SPAN")
	level := strings.ToUpper(get("langfuse.level"))
	statusMsg := ""
	if sp.Status != nil {
		if sp.Status.Code == 2 {
			level = "ERROR"
		}
		statusMsg = sp.Status.Message
	}
	if level == "" {
		level = "DEFAULT"
	}
	model := firstNonEmpty(
		get("langfuse.model.name"),
		get("gen_ai.request.model"),
		get("gen_ai.response.model"),
	)
	usageIn := firstInt(get("langfuse.usage.input"), get("gen_ai.usage.input_tokens"))
	usageOut := firstInt(get("langfuse.usage.output"), get("gen_ai.usage.output_tokens"))
	usageTotal := firstInt(get("langfuse.usage.total"))
	input := attrJSON(attrs["langfuse.observation.input"])
	output := attrJSON(attrs["langfuse.observation.output"])

	obsBody, _ := json.Marshal(map[string]any{
		"id":                  sp.SpanID,
		"traceId":             sp.TraceID,
		"parentObservationId": nullIfEmpty(sp.ParentSpanID),
		"type":                obsType,
		"name":                sp.Name,
		"startTime":           start.Format(time.RFC3339Nano),
		"endTime":             end.Format(time.RFC3339Nano),
		"input":               input,
		"output":              output,
		"metadata":            leftoverMetadata(attrs),
		"model":               nullIfEmpty(model),
		"usage": map[string]any{
			"input":  usageIn,
			"output": usageOut,
			"total":  usageTotal,
		},
		"level":         level,
		"statusMessage": nullIfEmpty(statusMsg),
		"environment":   firstNonEmpty(get("langfuse.environment"), "default"),
		"promptName":    nullIfEmpty(get("langfuse.prompt.name")),
		"promptVersion": firstInt(get("langfuse.prompt.version")),
	})
	out = append(out, ingest.Parsed{
		EventID: "otel-" + sp.SpanID, Type: ingest.TypeObservationCreate,
		Timestamp: start, Body: obsBody,
	})
	return out, nil
}

// knownAttrs are consumed into first-class fields; the rest lands in metadata.
var knownAttrs = map[string]bool{
	"langfuse.trace.name": true, "langfuse.user.id": true, "langfuse.session.id": true,
	"langfuse.tags": true, "langfuse.environment": true, "langfuse.observation.type": true,
	"langfuse.observation.input": true, "langfuse.observation.output": true,
	"langfuse.model.name": true, "gen_ai.request.model": true, "gen_ai.response.model": true,
	"langfuse.usage.input": true, "langfuse.usage.output": true, "langfuse.usage.total": true,
	"gen_ai.usage.input_tokens": true, "gen_ai.usage.output_tokens": true,
	"langfuse.prompt.name": true, "langfuse.prompt.version": true, "langfuse.level": true,
}

func leftoverMetadata(attrs map[string]AnyValue) map[string]any {
	meta := map[string]any{}
	for k, v := range attrs {
		if knownAttrs[k] {
			continue
		}
		if strings.HasPrefix(k, "langfuse.metadata.") {
			meta[strings.TrimPrefix(k, "langfuse.metadata.")] = anyToJSON(v)
			continue
		}
		meta[k] = anyToJSON(v)
	}
	return meta
}

func parseTags(v AnyValue) []string {
	if v.ArrayValue != nil {
		var out []string
		for _, e := range v.ArrayValue.Values {
			if s := anyToString(e); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	if s := anyToString(v); s != "" {
		var out []string
		for _, part := range strings.Split(s, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	}
	return []string{}
}

func anyToString(v AnyValue) string {
	switch {
	case v.StringValue != nil:
		return *v.StringValue
	case v.IntValue != nil:
		return *v.IntValue
	case v.DoubleValue != nil:
		return strconv.FormatFloat(*v.DoubleValue, 'f', -1, 64)
	case v.BoolValue != nil:
		return strconv.FormatBool(*v.BoolValue)
	case v.BytesValue != nil:
		return *v.BytesValue
	default:
		return ""
	}
}

func anyToJSON(v AnyValue) any {
	switch {
	case v.StringValue != nil:
		return *v.StringValue
	case v.IntValue != nil:
		if n, err := strconv.Atoi(*v.IntValue); err == nil {
			return n
		}
		return *v.IntValue
	case v.DoubleValue != nil:
		return *v.DoubleValue
	case v.BoolValue != nil:
		return *v.BoolValue
	case v.ArrayValue != nil:
		out := make([]any, 0, len(v.ArrayValue.Values))
		for _, e := range v.ArrayValue.Values {
			out = append(out, anyToJSON(e))
		}
		return out
	case v.KvlistValue != nil:
		m := map[string]any{}
		for _, kv := range v.KvlistValue.Values {
			m[kv.Key] = anyToJSON(kv.Value)
		}
		return m
	default:
		return nil
	}
}

// attrJSON encodes an attribute value as raw JSON for input/output storage.
func attrJSON(v AnyValue) any {
	if v.StringValue != nil {
		// Try to keep structured strings structured; else plain string.
		var probe any
		if err := json.Unmarshal([]byte(*v.StringValue), &probe); err == nil {
			return probe
		}
		return *v.StringValue
	}
	return anyToJSON(v)
}

func nanoToTime(raw string, fallback time.Time) time.Time {
	if raw == "" {
		return fallback
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return time.Unix(0, n).UTC()
	}
	return fallback
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstInt(vals ...string) *int {
	for _, v := range vals {
		if v == "" {
			continue
		}
		if n, err := strconv.Atoi(v); err == nil {
			return &n
		}
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
