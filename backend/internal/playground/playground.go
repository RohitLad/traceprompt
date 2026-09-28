// Package playground runs prompt templates against OpenAI-compatible
// chat APIs without persisting provider keys.
//
// Design: provider credentials travel per-request from the browser and are
// never stored server-side (no KMS liability). The proxy adds timeouts,
// response-shape validation, and optional trace logging via ingest.Store.
package playground

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// varPattern matches {{name}} with optional whitespace.
var varPattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.]+)\s*\}\}`)

// Render substitutes {{variables}} in a template. Unknown variables render
// empty (mirrors lenient prompt-preview behavior); use Missing to pre-check.
func Render(template string, vars map[string]string) string {
	return varPattern.ReplaceAllStringFunc(template, func(m string) string {
		key := strings.TrimSpace(m[2 : len(m)-2])
		return vars[key]
	})
}

// Missing returns template variables with no supplied value.
func Missing(template string, vars map[string]string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range varPattern.FindAllStringSubmatch(template, -1) {
		key := strings.TrimSpace(m[1])
		if _, ok := vars[key]; !ok && !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	}
	return out
}

// ChatRequest is the subset of OpenAI chat completions we send.
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

// Message is one chat turn.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse is the subset we parse back.
type ChatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
}

// Result is a completed playground run.
type Result struct {
	Output       string `json:"output"`
	Model        string `json:"model"`
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
	TotalTokens  int    `json:"totalTokens"`
	LatencyMs    int64  `json:"latencyMs"`
}

// Client calls OpenAI-compatible endpoints with a fixed timeout.
type Client struct {
	http *http.Client
}

// NewClient builds a client with 60s timeout (LLM calls are slow).
func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 60 * time.Second}}
}

// Run posts messages to baseURL/chat/completions with a bearer key.
func (c *Client) Run(ctx context.Context, baseURL, apiKey, model string, messages []Message) (Result, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" || apiKey == "" || model == "" {
		return Result{}, fmt.Errorf("baseUrl, apiKey, and model are required")
	}
	body, _ := json.Marshal(ChatRequest{Model: model, Messages: messages})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("provider: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("provider returned %d", resp.StatusCode)
	}
	var out ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Result{}, fmt.Errorf("provider response: %w", err)
	}
	if len(out.Choices) == 0 {
		return Result{}, fmt.Errorf("provider returned no choices")
	}
	res := Result{
		Output:    out.Choices[0].Message.Content,
		Model:     out.Model,
		LatencyMs: time.Since(start).Milliseconds(),
	}
	if res.Model == "" {
		res.Model = model
	}
	if out.Usage != nil {
		res.InputTokens = out.Usage.PromptTokens
		res.OutputTokens = out.Usage.CompletionTokens
		res.TotalTokens = out.Usage.TotalTokens
	}
	return res, nil
}
