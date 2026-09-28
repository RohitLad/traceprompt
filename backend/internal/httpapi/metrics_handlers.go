package httpapi

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/traceprompt/traceprompt/backend/internal/models"
)

// maxMetricsRows bounds full-table scans; responses flag truncation so
// callers know aggregates are partial until a warehouse export lands.
const maxMetricsRows = 20000

type dayBucket struct {
	Date         string `json:"date"`
	Traces       int    `json:"traces"`
	Observations int    `json:"observations"`
}

type modelBucket struct {
	Model        string `json:"model"`
	Observations int    `json:"observations"`
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
}

// GET /api/v1/projects/:id/metrics/overview?days=30
// Dialect-free Go aggregation (SQLite + Postgres identical). Full accuracy
// to maxMetricsRows; beyond that the response is marked truncated.
func (h *Handler) MetricsOverview(c *fiber.Ctx) error {
	p := currentProject(c)
	if p == nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	days := clampInt(queryInt(c, "days", 30), 1, 365)
	since := time.Now().UTC().AddDate(0, 0, -days)

	var traces []models.Trace
	if err := h.db.Select("trace_id", "created_at").
		Where("project_id = ? AND created_at >= ?", p.ID, since).
		Limit(maxMetricsRows).Find(&traces).Error; err != nil {
		return err
	}
	var obs []models.Observation
	if err := h.db.Select("model", "usage_input", "usage_output", "start_time", "end_time").
		Where("project_id = ? AND start_time >= ?", p.ID, since).
		Limit(maxMetricsRows).Find(&obs).Error; err != nil {
		return err
	}

	byDay := map[string]*dayBucket{}
	day := func(t time.Time) *dayBucket {
		k := t.UTC().Format("2006-01-02")
		b, ok := byDay[k]
		if !ok {
			b = &dayBucket{Date: k}
			byDay[k] = b
		}
		return b
	}
	for _, t := range traces {
		day(t.CreatedAt).Traces++
	}
	var inputTokens, outputTokens int
	var latencySum int64
	var latencyN int64
	byModel := map[string]*modelBucket{}
	for _, o := range obs {
		day(o.StartTime).Observations++
		if o.UsageInput != nil {
			inputTokens += *o.UsageInput
		}
		if o.UsageOutput != nil {
			outputTokens += *o.UsageOutput
		}
		if o.EndTime != nil && o.EndTime.After(o.StartTime) {
			latencySum += o.EndTime.Sub(o.StartTime).Milliseconds()
			latencyN++
		}
		model := "unknown"
		if o.Model != nil && *o.Model != "" {
			model = *o.Model
		}
		b, ok := byModel[model]
		if !ok {
			b = &modelBucket{Model: model}
			byModel[model] = b
		}
		b.Observations++
		if o.UsageInput != nil {
			b.InputTokens += *o.UsageInput
		}
		if o.UsageOutput != nil {
			b.OutputTokens += *o.UsageOutput
		}
	}

	days_out := make([]dayBucket, 0, len(byDay))
	for _, b := range byDay {
		days_out = append(days_out, *b)
	}
	// Sort ascending by date (insertion order is random from map).
	for i := 1; i < len(days_out); i++ {
		for j := i; j > 0 && days_out[j-1].Date > days_out[j].Date; j-- {
			days_out[j-1], days_out[j] = days_out[j], days_out[j-1]
		}
	}
	models_out := make([]modelBucket, 0, len(byModel))
	for _, b := range byModel {
		models_out = append(models_out, *b)
	}
	var avgLatency *float64
	if latencyN > 0 {
		v := float64(latencySum) / float64(latencyN)
		avgLatency = &v
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"days":         days,
		"traces":       len(traces),
		"observations": len(obs),
		"inputTokens":  inputTokens,
		"outputTokens": outputTokens,
		"avgLatencyMs": avgLatency,
		"perDay":       days_out,
		"byModel":      models_out,
		"truncated":    len(traces) == maxMetricsRows || len(obs) == maxMetricsRows,
	}})
}
