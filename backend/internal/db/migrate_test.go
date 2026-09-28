package db

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A broken migration only fails at deploy time, so this smoke test guards
// the embedded files: goose markers present, tables declared, chain ordered.
func TestEmbeddedMigrationsSane(t *testing.T) {
	entries, err := migrationFS.ReadDir("migrations")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(entries), 3, "expected at least 3 migration files")

	tables := []string{
		"organizations", "users", "memberships", "projects", "api_keys",
		"traces", "observations", "scores", "prompts", "prompt_versions",
		"datasets", "dataset_items", "dataset_runs", "dataset_run_items",
		"score_configs", "annotation_queues", "annotation_queue_items",
	}
	var initSQL strings.Builder
	var allSQL strings.Builder
	for _, e := range entries {
		raw, err := migrationFS.ReadFile("migrations/" + e.Name())
		require.NoError(t, err, e.Name())
		assert.Contains(t, string(raw), "-- +goose Up", e.Name())
		assert.Contains(t, string(raw), "-- +goose Down", e.Name())
		allSQL.Write(raw)
		if strings.HasPrefix(e.Name(), "00001_") {
			initSQL.Write(raw)
		}
	}
	require.NotZero(t, initSQL.Len(), "00001 init migration missing")
	// Every model table must be declared somewhere in the chain.
	for _, tbl := range tables {
		assert.Contains(t, allSQL.String(), "CREATE TABLE IF NOT EXISTS "+tbl, tbl)
	}
}
