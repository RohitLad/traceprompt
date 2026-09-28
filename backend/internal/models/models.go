package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Base provides UUID primary key + timestamps for all tables.
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

// Project scopes all observability data. Mirrors Langfuse project.
type Project struct {
	Base
	Name string `gorm:"not null" json:"name"`
}

// Trace is a single request/operation grouping observations.
// Field names follow Langfuse public API (camelCase JSON) for SDK compat.
type Trace struct {
	Base
	ProjectID string    `gorm:"index:idx_traces_project_time,priority:1;not null" json:"projectId"`
	TraceID   string    `gorm:"uniqueIndex;not null" json:"traceId"`
	Name      string    `gorm:"index" json:"name"`
	UserID    *string   `gorm:"index" json:"userId"`
	SessionID *string   `gorm:"index" json:"sessionId"`
	Metadata  JSONMap   `gorm:"type:jsonb;default:'{}'" json:"metadata"`
	Tags      StringArr `gorm:"type:text[]" json:"tags"`
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
	ProjectID       string          `gorm:"index:idx_obs_project_time,priority:1;not null" json:"projectId"`
	TraceID         string          `gorm:"index:idx_obs_trace,priority:1;not null" json:"traceId"`
	ObservationID   string          `gorm:"uniqueIndex;not null" json:"observationId"`
	ParentID        *string         `gorm:"index" json:"parentObservationId"`
	Type            ObservationType `gorm:"index;not null" json:"type"`
	Name            string          `gorm:"index" json:"name"`
	StartTime       time.Time       `gorm:"index:idx_obs_project_time,priority:2;not null" json:"startTime"`
	EndTime         *time.Time      `json:"endTime"`
	Input           *string         `gorm:"type:jsonb" json:"input"`
	Output          *string         `gorm:"type:jsonb" json:"output"`
	Metadata        JSONMap         `gorm:"type:jsonb;default:'{}'" json:"metadata"`
	Model           *string         `gorm:"index" json:"model"`
	ModelParameters JSONMap         `gorm:"type:jsonb;default:'{}'" json:"modelParameters"`
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
type Score struct {
	Base
	ProjectID     string   `gorm:"index;not null" json:"projectId"`
	TraceID       string   `gorm:"index;not null" json:"traceId"`
	ObservationID *string  `gorm:"index" json:"observationId"`
	SessionID     *string  `gorm:"index" json:"sessionId"`
	Name          string   `gorm:"index;not null" json:"name"`
	ValueNum      *float64 `json:"valueNum"`
	ValueStr      *string  `json:"valueStr"`
	DataType      string   `gorm:"not null" json:"dataType"` // NUMERIC|CATEGORICAL|BOOLEAN|TEXT|CORRECTION
	Source        string   `gorm:"not null;default:API" json:"source"`
	Comment       *string  `json:"comment"`
}

// AllModels lists every GORM model for AutoMigrate in dev/test.
// Production should use versioned SQL migrations (see migrations/).
func AllModels() []any {
	return []any{&Project{}, &Trace{}, &Observation{}, &Score{}}
}
