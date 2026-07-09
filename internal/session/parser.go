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
	Source Source

	SessionID string
	Path      string
	Format    FileFormat

	CreatedAt time.Time
	UpdatedAt time.Time
	SizeBytes int64

	CWD        string
	Models     []string
	Provider   string
	CLIVersion string

	Turns     int
	Messages  int
	ToolCalls int

	TokenUsage TokenUsage
}

// TokenUsage captures the token counters exposed by an agent session format.
type TokenUsage struct {
	InputTokens           int64
	CachedInputTokens     int64
	OutputTokens          int64
	ReasoningOutputTokens int64
	TotalTokens           int64
}
