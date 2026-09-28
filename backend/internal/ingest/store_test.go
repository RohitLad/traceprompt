package ingest

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/traceprompt/traceprompt/backend/internal/models"
)

func testStore(t *testing.T) (*Store, uuid.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Migrator().DropTable(models.AllModels()...))
	require.NoError(t, db.AutoMigrate(models.AllModels()...))
	pid := uuid.New()
	return NewStore(db), pid
}

func mustParse(t *testing.T, typ string, body string) Parsed {
	t.Helper()
	parsed, errs := Parse(json.RawMessage(
		`{"batch":[{"id":"e1","type":"` + typ + `","timestamp":"2026-01-01T00:00:00Z","body":` + body + `}]}`))
	require.Empty(t, errs)
	require.Len(t, parsed, 1)
	return parsed[0]
}

func count(t *testing.T, s *Store, model any) int64 {
	t.Helper()
	var n int64
	require.NoError(t, s.db.Model(model).Count(&n).Error)
	return n
}

func TestApplyTraceUpsertIdempotent(t *testing.T) {
	s, pid := testStore(t)
	ctx := context.Background()

	p := mustParse(t, "trace-create", `{"id":"t1","name":"chat","userId":"u1","tags":["web"]}`)
	require.NoError(t, s.Apply(ctx, pid, p))
	require.NoError(t, s.Apply(ctx, pid, p), "re-delivery must converge")

	assert.Equal(t, int64(1), count(t, s, &models.Trace{}))
	var tr models.Trace
	require.NoError(t, s.db.First(&tr, "trace_id = ?", "t1").Error)
	assert.Equal(t, "chat", tr.Name)
	require.NotNil(t, tr.UserID)
	assert.Equal(t, "u1", *tr.UserID)
	assert.Equal(t, []string{"web"}, []string(tr.Tags))
}

func TestApplyObservationWithUsageAndUpdate(t *testing.T) {
	s, pid := testStore(t)
	ctx := context.Background()

	create := mustParse(t, "generation-create",
		`{"id":"o1","traceId":"t1","name":"llm","model":"gpt-4o","input":"hi","usage":{"input":10,"output":20,"total":30}}`)
	require.NoError(t, s.Apply(ctx, pid, create))

	var o models.Observation
	require.NoError(t, s.db.First(&o, "observation_id = ?", "o1").Error)
	assert.Equal(t, models.ObservationType("GENERATION"), o.Type)
	require.NotNil(t, o.UsageInput)
	assert.Equal(t, 10, *o.UsageInput)
	assert.Equal(t, "default", o.Environment)

	// Update must not clobber name/model, only touches provided fields.
	update := mustParse(t, "observation-update", `{"id":"o1","output":"hello","statusMessage":"done"}`)
	require.NoError(t, s.Apply(ctx, pid, update))
	require.NoError(t, s.db.First(&o, "observation_id = ?", "o1").Error)
	assert.Equal(t, "llm", o.Name)
	require.NotNil(t, o.Output)
	assert.Equal(t, `"hello"`, *o.Output)

	// Orphan observation (trace arrives later) is kept, not rejected.
	orphan := mustParse(t, "span-create", `{"id":"o2","traceId":"t-missing","name":"step"}`)
	require.NoError(t, s.Apply(ctx, pid, orphan))
	assert.Equal(t, int64(2), count(t, s, &models.Observation{}))
}

func TestApplyScoreTypes(t *testing.T) {
	s, pid := testStore(t)
	ctx := context.Background()

	num := mustParse(t, "score-create", `{"id":"s1","traceId":"t1","name":"q","value":0.9}`)
	require.NoError(t, s.Apply(ctx, pid, num))
	require.NoError(t, s.Apply(ctx, pid, num), "score upsert on external id")

	cat := mustParse(t, "score-create", `{"traceId":"t1","name":"label","value":"good"}`)
	require.NoError(t, s.Apply(ctx, pid, cat))

	assert.Equal(t, int64(2), count(t, s, &models.Score{}))
	var sc models.Score
	require.NoError(t, s.db.First(&sc, "external_id = ?", "s1").Error)
	assert.Equal(t, "NUMERIC", sc.DataType)
	require.NotNil(t, sc.ValueNum)
}
