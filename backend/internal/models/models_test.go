package models

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAllModelsMigrate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Migrator().DropTable(AllModels()...))
	require.NoError(t, db.AutoMigrate(AllModels()...))
	// Every model must have a backing table after migrate.
	for _, m := range AllModels() {
		assert.True(t, db.Migrator().HasTable(m), "missing table for %T", m)
	}
}

func TestBaseFillsUUID(t *testing.T) {
	b := Base{}
	require.NoError(t, b.BeforeCreate(nil))
	assert.NotEqual(t, uuid.Nil, b.ID)

	fixed := Base{ID: uuid.New()}
	require.NoError(t, fixed.BeforeCreate(nil))
	assert.Equal(t, fixed.ID, fixed.ID, "existing IDs are preserved")
}

func TestJSONShapesAreLangfuseCompatible(t *testing.T) {
	tr := Trace{TraceID: "t1", Name: "chat", Tags: []string{"web"}}
	raw, err := json.Marshal(tr)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	assert.Contains(t, m, "traceId")
	assert.Contains(t, m, "projectId")
	assert.Contains(t, m, "userId")
	assert.NotContains(t, m, "trace_id", "JSON must be camelCase, not snake_case")

	var sc Score
	raw, err = json.Marshal(sc)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &m))
	assert.Contains(t, m, "dataType")
	assert.Contains(t, m, "observationId")
}
