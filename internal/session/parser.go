package session

import (
	"context"
	"time"
)

// Parser reads a discovered session file and extracts normalized metadata.
type Parser interface {
	Source() Source
	ParseSession(context.Context, FileRef) (Record, error)
}

// Record is a provider-neutral summary of a parsed agent session.
type Record struct {
	Source Source `json:"source"`

	SessionID string     `json:"session_id"`
	Path      string     `json:"path"`
	Format    FileFormat `json:"format"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	SizeBytes int64     `json:"size_bytes"`

	CWD         string   `json:"cwd"`
	Models      []string `json:"models"`
	Provider    string   `json:"provider"`
	CLIVersion  string   `json:"cli_version"`
	FirstPrompt string   `json:"first_prompt"`

	Turns     int `json:"turns"`
	Messages  int `json:"messages"`
	ToolCalls int `json:"tool_calls"`

	TokenUsage    TokenUsage     `json:"token_usage"`
	UsageSegments []UsageSegment `json:"usage_segments"`
}

// UsageSegment attributes one request's incremental token usage to a provider and model.
type UsageSegment struct {
	Provider    string     `json:"provider"`
	Model       string     `json:"model"`
	ServiceTier string     `json:"service_tier,omitempty"`
	OccurredAt  time.Time  `json:"occurred_at,omitzero"`
	TokenUsage  TokenUsage `json:"token_usage"`
}

// TokenUsage captures the token counters exposed by an agent session format.
type TokenUsage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
	TotalTokens           int64 `json:"total_tokens"`
}
