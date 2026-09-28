package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/traceprompt/traceprompt/backend/internal/models"
)

// Store persists parsed ingestion events with idempotent upserts on
// external IDs (traceId, observationId, score id). Orphan observations
// and scores are kept — their trace may arrive in a later batch.
type Store struct {
	db *gorm.DB
}

// NewStore wraps a *gorm.DB for ingestion writes.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Apply writes one parsed event for a project. It is idempotent:
// re-delivery (Redis redrive, SDK retry) converges instead of duplicating.
func (s *Store) Apply(ctx context.Context, projectID uuid.UUID, p Parsed) error {
	switch p.Type {
	case TypeTraceCreate:
		return s.applyTrace(ctx, projectID, p)
	case TypeObservationCreate, TypeObservationUpdate:
		return s.applyObservation(ctx, projectID, p)
	case TypeScoreCreate:
		return s.applyScore(ctx, projectID, p)
	default:
		return nil
	}
}

type traceBody struct {
	ID        string         `json:"id"`
	Name      *string        `json:"name"`
	UserID    *string        `json:"userId"`
	SessionID *string        `json:"sessionId"`
	Metadata  map[string]any `json:"metadata"`
	Tags      []string       `json:"tags"`
}

func (s *Store) applyTrace(ctx context.Context, projectID uuid.UUID, p Parsed) error {
	var b traceBody
	if err := json.Unmarshal(p.Body, &b); err != nil {
		return err
	}
	t := models.Trace{
		ProjectID: projectID,
		TraceID:   b.ID,
		Metadata:  orEmptyMap(b.Metadata),
		Tags:      orEmptySlice(b.Tags),
	}
	if b.Name != nil {
		t.Name = *b.Name
	}
	t.UserID = emptyToNil(b.UserID)
	t.SessionID = emptyToNil(b.SessionID)
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "trace_id"}},
		UpdateAll: true,
	}).Create(&t).Error
}

type usageBlock struct {
	Input  *int `json:"input"`
	Output *int `json:"output"`
	Total  *int `json:"total"`
}

type observationBody struct {
	ID                  string          `json:"id"`
	TraceID             *string         `json:"traceId"`
	ParentObservationID *string         `json:"parentObservationId"`
	Type                *string         `json:"type"`
	Name                *string         `json:"name"`
	StartTime           *string         `json:"startTime"`
	EndTime             *string         `json:"endTime"`
	Input               json.RawMessage `json:"input"`
	Output              json.RawMessage `json:"output"`
	Metadata            map[string]any  `json:"metadata"`
	Model               *string         `json:"model"`
	ModelParameters     map[string]any  `json:"modelParameters"`
	Usage               *usageBlock     `json:"usage"`
	PromptTokens        *int            `json:"promptTokens"`
	CompletionTokens    *int            `json:"completionTokens"`
	TotalTokens         *int            `json:"totalTokens"`
	Level               *string         `json:"level"`
	StatusMessage       *string         `json:"statusMessage"`
	Environment         *string         `json:"environment"`
	PromptName          *string         `json:"promptName"`
	PromptVersion       *int            `json:"promptVersion"`
}

func (s *Store) applyObservation(ctx context.Context, projectID uuid.UUID, p Parsed) error {
	var b observationBody
	if err := json.Unmarshal(p.Body, &b); err != nil {
		return err
	}
	o := models.Observation{
		ProjectID:       projectID,
		ObservationID:   b.ID,
		Metadata:        orEmptyMap(b.Metadata),
		ModelParameters: orEmptyMap(b.ModelParameters),
	}
	if b.TraceID != nil {
		o.TraceID = *b.TraceID
	}
	o.ParentID = emptyToNil(b.ParentObservationID)
	obsType := p.ObsType
	if b.Type != nil && *b.Type != "" {
		obsType = strings.ToUpper(*b.Type)
	}
	if obsType == "" {
		obsType = string(models.ObsSpan)
	}
	o.Type = models.ObservationType(obsType)
	if b.Name != nil {
		o.Name = *b.Name
	}
	if b.StartTime != nil {
		if t, err := parseTime(*b.StartTime); err == nil {
			o.StartTime = t
		}
	}
	if o.StartTime.IsZero() {
		o.StartTime = p.Timestamp
	}
	if b.EndTime != nil {
		if t, err := parseTime(*b.EndTime); err == nil {
			o.EndTime = &t
		}
	}
	if len(b.Input) > 0 {
		s := string(b.Input)
		o.Input = &s
	}
	if len(b.Output) > 0 {
		s := string(b.Output)
		o.Output = &s
	}
	o.Model = emptyToNil(b.Model)
	o.UsageInput = firstNonNil(usageIn(b, true), b.PromptTokens)
	o.UsageOutput = firstNonNil(usageIn(b, false), b.CompletionTokens)
	if b.Usage != nil {
		o.UsageTotal = b.Usage.Total
	} else {
		o.UsageTotal = b.TotalTokens
	}
	if b.Level != nil && *b.Level != "" {
		o.Level = strings.ToUpper(*b.Level)
	} else if p.Type == TypeObservationCreate {
		o.Level = "DEFAULT"
	}
	o.StatusMessage = emptyToNil(b.StatusMessage)
	if b.Environment != nil && *b.Environment != "" {
		o.Environment = *b.Environment
	} else if p.Type == TypeObservationCreate {
		o.Environment = "default"
	}
	o.PromptName = emptyToNil(b.PromptName)
	o.PromptVersion = b.PromptVersion

	// Update events must not clobber existing columns with zero values,
	// so they use a targeted update; creates upsert fully.
	if p.Type == TypeObservationUpdate {
		existing := models.Observation{}
		err := s.db.WithContext(ctx).
			First(&existing, "project_id = ? AND observation_id = ?", projectID, b.ID).Error
		if err == nil {
			updates := map[string]any{}
			if b.TraceID != nil {
				updates["trace_id"] = *b.TraceID
			}
			if b.ParentObservationID != nil {
				updates["parent_id"] = nullIfEmpty(*b.ParentObservationID)
			}
			if b.Name != nil {
				updates["name"] = *b.Name
			}
			if b.Type != nil {
				updates["type"] = strings.ToUpper(*b.Type)
			}
			if o.EndTime != nil {
				updates["end_time"] = *o.EndTime
			}
			if o.Input != nil {
				updates["input"] = *o.Input
			}
			if o.Output != nil {
				updates["output"] = *o.Output
			}
			if b.Metadata != nil {
				updates["metadata"] = o.Metadata
			}
			if b.Model != nil {
				updates["model"] = nullIfEmpty(*b.Model)
			}
			if b.Level != nil {
				updates["level"] = o.Level
			}
			if b.StatusMessage != nil {
				updates["status_message"] = nullIfEmpty(*b.StatusMessage)
			}
			if len(updates) == 0 {
				return nil
			}
			return s.db.WithContext(ctx).Model(&models.Observation{}).
				Where("project_id = ? AND observation_id = ?", projectID, b.ID).
				Updates(updates).Error
		}
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "observation_id"}},
		UpdateAll: true,
	}).Create(&o).Error
}

type scoreBody struct {
	ID            *string         `json:"id"`
	TraceID       *string         `json:"traceId"`
	ObservationID *string         `json:"observationId"`
	SessionID     *string         `json:"sessionId"`
	Name          string          `json:"name"`
	Value         json.RawMessage `json:"value"`
	DataType      *string         `json:"dataType"`
	Comment       *string         `json:"comment"`
	ConfigID      *string         `json:"configId"`
}

func (s *Store) applyScore(ctx context.Context, projectID uuid.UUID, p Parsed) error {
	var b scoreBody
	if err := json.Unmarshal(p.Body, &b); err != nil {
		return err
	}
	sc := models.Score{
		ProjectID: projectID,
		Name:      b.Name,
		Source:    "API",
		DataType:  "CATEGORICAL",
	}
	if b.ID != nil && *b.ID != "" {
		sc.ExternalID = b.ID
	}
	if b.TraceID != nil {
		sc.TraceID = *b.TraceID
	}
	sc.ObservationID = emptyToNil(b.ObservationID)
	sc.SessionID = emptyToNil(b.SessionID)
	sc.Comment = emptyToNil(b.Comment)

	var num float64
	var str string
	var boolean bool
	switch {
	case json.Unmarshal(b.Value, &num) == nil:
		sc.ValueNum = &num
		sc.DataType = "NUMERIC"
	case json.Unmarshal(b.Value, &boolean) == nil:
		if boolean {
			num = 1
		}
		sc.ValueNum = &num
		sc.DataType = "BOOLEAN"
	case json.Unmarshal(b.Value, &str) == nil:
		sc.ValueStr = &str
		sc.DataType = "CATEGORICAL"
	}
	if b.DataType != nil && *b.DataType != "" {
		sc.DataType = strings.ToUpper(*b.DataType)
	}

	if err := s.checkScoreConfig(ctx, projectID, &b, &sc); err != nil {
		return err
	}

	if sc.ExternalID != nil {
		return s.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "external_id"}},
			UpdateAll: true,
		}).Create(&sc).Error
	}
	return s.db.WithContext(ctx).Create(&sc).Error
}

// checkScoreConfig enforces the project's score schema when the score names
// (or references) a configured schema. Violations are permanent: the value
// can never become valid by redelivery.
func (s *Store) checkScoreConfig(ctx context.Context, projectID uuid.UUID, b *scoreBody, sc *models.Score) error {
	var cfg models.ScoreConfig
	if b.ConfigID != nil && *b.ConfigID != "" {
		id, err := uuid.Parse(*b.ConfigID)
		if err != nil {
			return Permanent("invalid configId")
		}
		if err := s.db.WithContext(ctx).First(&cfg, "id = ? AND project_id = ?", id, projectID).Error; err != nil {
			return Permanent("score config not found")
		}
		if cfg.Name != b.Name {
			return Permanent("score name must equal its config name")
		}
	} else {
		if err := s.db.WithContext(ctx).First(&cfg, "project_id = ? AND name = ?", projectID, b.Name).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil // no schema declared → anything goes
			}
			return err
		}
	}
	if sc.DataType != cfg.DataType {
		return Permanent("score dataType " + sc.DataType + " does not match config " + cfg.DataType)
	}
	switch cfg.DataType {
	case "NUMERIC":
		if sc.ValueNum == nil {
			return Permanent("numeric score requires a numeric value")
		}
		if cfg.MinValue != nil && *sc.ValueNum < *cfg.MinValue {
			return Permanent("score below config minimum")
		}
		if cfg.MaxValue != nil && *sc.ValueNum > *cfg.MaxValue {
			return Permanent("score above config maximum")
		}
	case "CATEGORICAL":
		if sc.ValueStr == nil {
			return Permanent("categorical score requires a string value")
		}
		if len(cfg.Categories) > 0 && !containsStr(cfg.Categories, *sc.ValueStr) {
			return Permanent("score category not in config")
		}
	}
	return nil
}

func containsStr(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}

func usageIn(b observationBody, input bool) *int {
	if b.Usage == nil {
		return nil
	}
	if input {
		return b.Usage.Input
	}
	return b.Usage.Output
}

func firstNonNil[T any](vals ...*T) *T {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

func orEmptyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func orEmptySlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func emptyToNil(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}

func nullIfEmpty(s string) any {
	if s == "" {
		return gorm.Expr("NULL")
	}
	return s
}
