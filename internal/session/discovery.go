package session

import (
	"context"
	"time"
)

// Source identifies the agent or tool that produced a session file.
type Source string

// FileRole describes what kind of session-adjacent file was found.
type FileRole string

const (
	FileRoleTranscript FileRole = "transcript"
	FileRoleHistory    FileRole = "history"
)

// FileFormat describes the on-disk encoding of a discovered file.
type FileFormat string

const (
	FileFormatJSONL     FileFormat = "jsonl"
	FileFormatJSONLZstd FileFormat = "jsonl.zst"
)

// FindOptions configures a provider-specific file discovery pass.
type FindOptions struct {
	// Roots are provider-specific roots, e.g. $CODEX_HOME
	Roots []string

	IncludeArchived bool
	Limit           int
}

// FileRef is a provider-neutral reference to a local session file.
type FileRef struct {
	Source Source     `json:"source"`
	Role   FileRole   `json:"role"`
	Format FileFormat `json:"format"`

	Path       string `json:"path"`
	Compressed bool   `json:"compressed"`

	SessionID string    `json:"session_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	SizeBytes int64     `json:"size_bytes"`
}

// Finder discovers local files that may contain agent session data.
type Finder interface {
	Source() Source
	FindSessionFiles(context.Context, FindOptions) ([]FileRef, error)
}
