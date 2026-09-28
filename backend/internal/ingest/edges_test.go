package ingest

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPermanentDetection(t *testing.T) {
	assert.False(t, IsPermanent(nil))
	assert.False(t, IsPermanent(fmt.Errorf("transient")))
	assert.True(t, IsPermanent(Permanent("bad value")))
	assert.True(t, IsPermanent(fmt.Errorf("wrap: %w", Permanent("bad value"))))
	assert.Equal(t, "bad value", Permanent("bad value").Error())
}

func TestNormalizeTypeAliases(t *testing.T) {
	cases := map[string][2]string{
		"trace-create":       {TypeTraceCreate, ""},
		"observation-create": {TypeObservationCreate, ""},
		"span-create":        {TypeObservationCreate, "SPAN"},
		"generation-create":  {TypeObservationCreate, "GENERATION"},
		"event-create":       {TypeObservationCreate, "EVENT"},
		"observation-update": {TypeObservationUpdate, ""},
		"span-update":        {TypeObservationUpdate, ""},
		"generation-update":  {TypeObservationUpdate, ""},
		"score-create":       {TypeScoreCreate, ""},
	}
	for in, want := range cases {
		norm, obs, err := normalizeType(in)
		assert.NoError(t, err, in)
		assert.Equal(t, want[0], norm, in)
		assert.Equal(t, want[1], obs, in)
	}
	_, _, err := normalizeType("nope")
	assert.Error(t, err)
}

func TestValidateBodyEdges(t *testing.T) {
	// Score missing each required piece.
	for _, body := range []string{
		`{"name":"q","value":1}`,
		`{"traceId":"t","value":1}`,
		`{"traceId":"t","name":"q"}`,
		`[1,2]`,
	} {
		err := validateBody(TypeScoreCreate, []byte(body))
		assert.Error(t, err, body)
	}
	// Trace/observation without id.
	assert.Error(t, validateBody(TypeTraceCreate, []byte(`{"name":"x"}`)))
	assert.Error(t, validateBody(TypeObservationCreate, []byte(`{}`)))
	// Bad timestamp rejected at parse.
	_, errs := Parse([]byte(`{"batch":[{"id":"e","type":"trace-create","timestamp":"yesterday","body":{"id":"t"}}]}`))
	assert.Len(t, errs, 1)
}
