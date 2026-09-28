package queue

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/traceprompt/traceprompt/backend/internal/ingest"
)

func TestMemoryEnqueueDrainLen(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	assert.Equal(t, 0, m.Len())

	pid := uuid.New()
	mk := func(id string) Item {
		return FromParsed(pid, ingest.Parsed{
			EventID: id, Type: ingest.TypeTraceCreate,
			Timestamp: time.Now().UTC(), Body: []byte(`{"id":"t"}`),
		})
	}
	require.NoError(t, m.Enqueue(ctx, []Item{mk("e1"), mk("e2"), mk("e3")}))
	assert.Equal(t, 3, m.Len())

	got := m.Drain(2)
	require.Len(t, got, 2)
	assert.Equal(t, "e1", got[0].EventID)
	assert.Equal(t, pid.String(), got[0].ProjectID)
	assert.Equal(t, 1, m.Len())

	rest := m.Drain(100)
	require.Len(t, rest, 1)
	assert.Equal(t, "e3", rest[0].EventID)
	assert.Equal(t, 0, m.Len())
	assert.Empty(t, m.Drain(10))
}

func TestItemRoundTrip(t *testing.T) {
	pid := uuid.New()
	ts := time.Now().UTC().Truncate(time.Second)
	p := ingest.Parsed{
		EventID: "e9", Type: ingest.TypeObservationCreate, ObsType: "GENERATION",
		Timestamp: ts, Body: []byte(`{"id":"o"}`),
	}
	it := FromParsed(pid, p)
	back := it.ToParsed()
	assert.Equal(t, p.EventID, back.EventID)
	assert.Equal(t, p.Type, back.Type)
	assert.Equal(t, p.ObsType, back.ObsType)
	assert.True(t, back.Timestamp.Equal(ts))
	assert.JSONEq(t, `{"id":"o"}`, string(back.Body))
}
