package worker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/traceprompt/traceprompt/backend/internal/ingest"
	"github.com/traceprompt/traceprompt/backend/internal/models"
	"github.com/traceprompt/traceprompt/backend/internal/queue"
)

func testSetup(t *testing.T) (*queue.Memory, *ingest.Store, *gorm.DB, uuid.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Migrator().DropTable(models.AllModels()...))
	require.NoError(t, db.AutoMigrate(models.AllModels()...))
	mem := queue.NewMemory()
	return mem, ingest.NewStore(db), db, uuid.New()
}

func mustItem(t *testing.T, pid uuid.UUID, typ, body string) queue.Item {
	t.Helper()
	parsed, errs := ingest.Parse(json.RawMessage(
		`{"batch":[{"id":"e1","type":"` + typ + `","timestamp":"2026-01-01T00:00:00Z","body":` + body + `}]}`))
	require.Empty(t, errs)
	require.Len(t, parsed, 1)
	return queue.FromParsed(pid, parsed[0])
}

func TestDrainMemoryAppliesEvents(t *testing.T) {
	mem, store, db, pid := testSetup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go DrainMemory(ctx, mem, store, 10*time.Millisecond)

	require.NoError(t, mem.Enqueue(ctx, []queue.Item{
		mustItem(t, pid, "trace-create", `{"id":"w-t1","name":"w"}`),
		mustItem(t, pid, "span-create", `{"id":"w-o1","traceId":"w-t1","name":"s"}`),
		mustItem(t, pid, "score-create", `{"traceId":"w-t1","name":"q","value":1}`),
	}))

	require.Eventually(t, func() bool {
		var n int64
		db.Model(&models.Trace{}).Count(&n)
		return n == 1
	}, 5*time.Second, 10*time.Millisecond)
	var on, sn int64
	db.Model(&models.Observation{}).Count(&on)
	db.Model(&models.Score{}).Count(&sn)
	assert.Equal(t, int64(1), on)
	assert.Equal(t, int64(1), sn)
}

func TestDrainMemoryDropsBadProject(t *testing.T) {
	mem, store, db, _ := testSetup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go DrainMemory(ctx, mem, store, 10*time.Millisecond)

	bad := queue.Item{ProjectID: "not-a-uuid", EventID: "e1", Type: ingest.TypeTraceCreate, Body: []byte(`{"id":"t"}`)}
	require.NoError(t, mem.Enqueue(ctx, []queue.Item{bad}))
	// Drained (not stuck) and nothing persisted.
	require.Eventually(t, func() bool { return mem.Len() == 0 }, 5*time.Second, 10*time.Millisecond)
	var n int64
	db.Model(&models.Trace{}).Count(&n)
	assert.Equal(t, int64(0), n)
}

func TestDrainMemoryToleratesSchemaViolations(t *testing.T) {
	// A schema-violating score is a permanent failure: the worker logs it,
	// drains past it, and never stores it — the queue must not wedge.
	mem, store, db, pid := testSetup(t)
	zero, one := 0.0, 1.0
	require.NoError(t, db.Create(&models.ScoreConfig{
		ProjectID: pid, Name: "q", DataType: "NUMERIC", MinValue: &zero, MaxValue: &one,
	}).Error)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go DrainMemory(ctx, mem, store, 10*time.Millisecond)

	require.NoError(t, mem.Enqueue(ctx, []queue.Item{
		mustItem(t, pid, "score-create", `{"traceId":"t","name":"q","value":99}`),
		mustItem(t, pid, "trace-create", `{"id":"after","name":"kept"}`),
	}))
	// Wait on persistence, not queue depth: Len()==0 fires the moment items
	// are drained, potentially before Apply commits.
	require.Eventually(t, func() bool {
		var n int64
		db.Model(&models.Trace{}).Where("trace_id = ?", "after").Count(&n)
		return n == 1
	}, 5*time.Second, 10*time.Millisecond)

	var scores int64
	db.Model(&models.Score{}).Count(&scores)
	assert.Equal(t, int64(0), scores, "violating score never stored")
}
