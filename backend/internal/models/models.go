package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Base provides UUID primary key + timestamps for all tables.
// IDs are UUIDs internally; Langfuse-compatible external IDs
// (traceId, observationId, pk-lf-...) live on dedicated columns.
type Base struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// BeforeCreate fills UUIDs automatically.
func (b *Base) BeforeCreate(_ *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

// Organization groups users and projects.
type Organization struct {
	Base
	Name string `gorm:"not null" json:"name"`
}

// User is a human login. Passwords are bcrypt hashes, never returned.
type User struct {
	Base
	Email        string `gorm:"uniqueIndex;not null" json:"email"`
	Name         string `gorm:"not null" json:"name"`
	PasswordHash string `gorm:"not null" json:"-"`
}

// Membership links a user to an organization with a role.
type Membership struct {
	Base
	UserID         uuid.UUID `gorm:"uniqueIndex:idx_membership;not null" json:"userId"`
	OrganizationID uuid.UUID `gorm:"uniqueIndex:idx_membership;not null" json:"organizationId"`
	Role           string    `gorm:"not null;default:member" json:"role"` // owner|member
}

// Project scopes all observability data. Mirrors Langfuse project.
type Project struct {
	Base
	OrganizationID uuid.UUID `gorm:"index;not null" json:"organizationId"`
	Name           string    `gorm:"not null" json:"name"`
}

// ApiKey authenticates SDK/public-API callers via BasicAuth pk:sk.
// Only a SHA-256 hash of the secret is stored; the raw secret is
// shown once at creation time (Langfuse behavior).
type ApiKey struct {
	Base
	ProjectID  uuid.UUID  `gorm:"index;not null" json:"projectId"`
	Name       string     `gorm:"not null" json:"name"`
	PublicKey  string     `gorm:"uniqueIndex;not null" json:"publicKey"`
	SecretHash string     `gorm:"not null" json:"-"`
	CreatedBy  *uuid.UUID `json:"createdBy"`
	RevokedAt  *time.Time `gorm:"index" json:"revokedAt"`
}

// Revoked reports whether the key can no longer authenticate.
func (k *ApiKey) Revoked() bool {
	return k.RevokedAt != nil
}

// Trace is a single request/operation grouping observations.
// Field names follow Langfuse public API (camelCase JSON) for SDK compat.
//
// NOTE: Metadata/Tags use GORM serializer:json so the same models run on
// Postgres (jsonb) and SQLite (text) in tests. If tag filtering needs GIN
// indexes at scale, migrate Tags to text[] in a versioned migration.
type Trace struct {
	Base
	ProjectID uuid.UUID      `gorm:"index:idx_traces_project_time,priority:1;not null" json:"projectId"`
	TraceID   string         `gorm:"uniqueIndex;not null" json:"traceId"`
	Name      string         `gorm:"index" json:"name"`
	UserID    *string        `gorm:"index" json:"userId"`
	SessionID *string        `gorm:"index" json:"sessionId"`
	Metadata  map[string]any `gorm:"serializer:json;type:jsonb" json:"metadata"`
	Tags      []string       `gorm:"serializer:json" json:"tags"`
}

// ObservationType mirrors Langfuse observation types.
type ObservationType string

const (
	ObsSpan       ObservationType = "SPAN"
	ObsGeneration ObservationType = "GENERATION"
	ObsEvent      ObservationType = "EVENT"
	ObsTool       ObservationType = "TOOL"
	ObsAgent      ObservationType = "AGENT"
	ObsChain      ObservationType = "CHAIN"
	ObsRetriever  ObservationType = "RETRIEVER"
	ObsEvaluator  ObservationType = "EVALUATOR"
	ObsEmbedding  ObservationType = "EMBEDDING"
	ObsGuardrail  ObservationType = "GUARDRAIL"
)

// Observation is one step inside a trace (LLM call, tool, retrieval...).
type Observation struct {
	Base
	ProjectID       uuid.UUID       `gorm:"index:idx_obs_project_time,priority:1;not null" json:"projectId"`
	TraceID         string          `gorm:"index:idx_obs_trace,priority:1;not null" json:"traceId"`
	ObservationID   string          `gorm:"uniqueIndex;not null" json:"observationId"`
	ParentID        *string         `gorm:"index" json:"parentObservationId"`
	Type            ObservationType `gorm:"index;not null" json:"type"`
	Name            string          `gorm:"index" json:"name"`
	StartTime       time.Time       `gorm:"index:idx_obs_project_time,priority:2;not null" json:"startTime"`
	EndTime         *time.Time      `json:"endTime"`
	Input           *string         `gorm:"type:text" json:"input"`
	Output          *string         `gorm:"type:text" json:"output"`
	Metadata        map[string]any  `gorm:"serializer:json;type:jsonb" json:"metadata"`
	Model           *string         `gorm:"index" json:"model"`
	ModelParameters map[string]any  `gorm:"serializer:json;type:jsonb" json:"modelParameters"`
	UsageInput      *int            `json:"inputUsage"`
	UsageOutput     *int            `json:"outputUsage"`
	UsageTotal      *int            `json:"totalUsage"`
	Level           string          `gorm:"default:DEFAULT" json:"level"`
	StatusMessage   *string         `json:"statusMessage"`
	Environment     string          `gorm:"default:default;index" json:"environment"`
	PromptName      *string         `gorm:"index" json:"promptName"`
	PromptVersion   *int            `json:"promptVersion"`
}

// Score attaches evaluation/human feedback to a trace or observation.
// ExternalID carries the SDK-provided score id for idempotent upserts.
type Score struct {
	Base
	ProjectID     uuid.UUID `gorm:"index;not null" json:"projectId"`
	ExternalID    *string   `gorm:"uniqueIndex" json:"externalId"`
	TraceID       string    `gorm:"index;not null" json:"traceId"`
	ObservationID *string   `gorm:"index" json:"observationId"`
	SessionID     *string   `gorm:"index" json:"sessionId"`
	Name          string    `gorm:"index;not null" json:"name"`
	ValueNum      *float64  `json:"valueNum"`
	ValueStr      *string   `json:"valueStr"`
	DataType      string    `gorm:"not null" json:"dataType"` // NUMERIC|CATEGORICAL|BOOLEAN|TEXT|CORRECTION
	Source        string    `gorm:"not null;default:API" json:"source"`
	Comment       *string   `json:"comment"`
}

// Prompt centralizes a versioned template (text or chat) outside app code.
type Prompt struct {
	Base
	ProjectID uuid.UUID `gorm:"uniqueIndex:idx_prompts_project_name,priority:1;not null" json:"projectId"`
	Name      string    `gorm:"uniqueIndex:idx_prompts_project_name,priority:2;not null" json:"name"`
	Type      string    `gorm:"not null;default:text" json:"type"` // text|chat
}

// PromptVersion is one immutable template revision. Labels (e.g.
// "production", "latest") move between versions to control rollouts.
type PromptVersion struct {
	Base
	PromptID      uuid.UUID       `gorm:"uniqueIndex:idx_prompt_versions,priority:1;not null" json:"promptId"`
	Version       int             `gorm:"uniqueIndex:idx_prompt_versions,priority:2;not null" json:"version"`
	Template      *string         `gorm:"type:text" json:"template"`                 // text prompts
	Messages      []PromptMessage `gorm:"serializer:json" json:"messages,omitempty"` // chat prompts
	Config        map[string]any  `gorm:"serializer:json;type:jsonb" json:"config"`
	Labels        []string        `gorm:"serializer:json" json:"labels"`
	CommitMessage *string         `json:"commitMessage"`
}

// PromptMessage is one chat turn with mustache-style {{variables}}.
type PromptMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Dataset is a versioned test set for experiments (prompts/models eval).
type Dataset struct {
	Base
	ProjectID   uuid.UUID `gorm:"uniqueIndex:idx_datasets_project_name,priority:1;not null" json:"projectId"`
	Name        string    `gorm:"uniqueIndex:idx_datasets_project_name,priority:2;not null" json:"name"`
	Description *string   `json:"description"`
}

// DatasetItem is one test case: input, expected output, and metadata.
type DatasetItem struct {
	Base
	DatasetID      uuid.UUID      `gorm:"index;not null" json:"datasetId"`
	Input          *string        `gorm:"type:text" json:"input"`
	ExpectedOutput *string        `gorm:"type:text" json:"expectedOutput"`
	Metadata       map[string]any `gorm:"serializer:json;type:jsonb" json:"metadata"`
}

// DatasetRun is one execution of an app against dataset items.
type DatasetRun struct {
	Base
	DatasetID   uuid.UUID      `gorm:"index;not null" json:"datasetId"`
	Name        string         `gorm:"index;not null" json:"name"`
	Description *string        `json:"description"`
	Metadata    map[string]any `gorm:"serializer:json;type:jsonb" json:"metadata"`
}

// DatasetRunItem links one item to the trace it produced in a run.
// Unique per (run, item): re-running an item updates its trace link.
type DatasetRunItem struct {
	Base
	RunID   uuid.UUID `gorm:"uniqueIndex:idx_run_items,priority:1;not null" json:"runId"`
	ItemID  uuid.UUID `gorm:"uniqueIndex:idx_run_items,priority:2;not null" json:"itemId"`
	TraceID *string   `gorm:"index" json:"traceId"`
}

// AuthModels are migrated first and work on SQLite (tests) and Postgres.
func AuthModels() []any {
	return []any{&Organization{}, &User{}, &Membership{}, &Project{}, &ApiKey{}}
}

// AllModels lists every GORM model for AutoMigrate in dev/test.
// Production should use versioned SQL migrations (see migrations/).
func AllModels() []any {
	return append(AuthModels(),
		&Trace{}, &Observation{}, &Score{},
		&Prompt{}, &PromptVersion{},
		&Dataset{}, &DatasetItem{}, &DatasetRun{}, &DatasetRunItem{},
	)
}
