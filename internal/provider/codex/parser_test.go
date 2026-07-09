package codex

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/rybkr/totally/internal/session"
)

func TestParserParseSession(t *testing.T) {
	path := writeRolloutJSONL(t, rolloutFixture())
	created := time.Date(2026, 7, 8, 20, 20, 44, 0, time.UTC)
	updated := time.Date(2026, 7, 9, 8, 0, 0, 0, time.UTC)

	record, err := NewParser().ParseSession(context.Background(), session.FileRef{
		Source:    Source,
		Role:      session.FileRoleTranscript,
		Format:    session.FileFormatJSONL,
		Path:      path,
		SessionID: "from-file-name",
		CreatedAt: created,
		UpdatedAt: updated,
		SizeBytes: 123,
	})
	if err != nil {
		t.Fatal(err)
	}

	if record.Source != Source {
		t.Fatalf("unexpected source: %s", record.Source)
	}
	if record.SessionID != "019f44e4-5c01-7d22-9805-50cecaefde49" {
		t.Fatalf("unexpected session ID: %s", record.SessionID)
	}
	if record.Path != path || record.CreatedAt != created || record.UpdatedAt != updated || record.SizeBytes != 123 {
		t.Fatalf("file metadata was not preserved: %+v", record)
	}
	if record.CWD != "/tmp/project" {
		t.Fatalf("unexpected cwd: %s", record.CWD)
	}
	if record.Provider != "openai" {
		t.Fatalf("unexpected provider: %s", record.Provider)
	}
	if record.CLIVersion != "0.142.5" {
		t.Fatalf("unexpected cli version: %s", record.CLIVersion)
	}
	wantModels := []string{"gpt-5", "gpt-5-mini"}
	if !slices.Equal(record.Models, wantModels) {
		t.Fatalf("unexpected models: %+v", record.Models)
	}
	if record.Turns != 3 {
		t.Fatalf("unexpected turns: %d", record.Turns)
	}
	if record.Messages != 2 {
		t.Fatalf("unexpected messages: %d", record.Messages)
	}
	if record.ToolCalls != 2 {
		t.Fatalf("unexpected tool calls: %d", record.ToolCalls)
	}

	wantUsage := session.TokenUsage{
		InputTokens:           100,
		CachedInputTokens:     40,
		OutputTokens:          25,
		ReasoningOutputTokens: 5,
		TotalTokens:           125,
	}
	if record.TokenUsage != wantUsage {
		t.Fatalf("unexpected token usage: %+v", record.TokenUsage)
	}
}

func TestParserParseCompressedSession(t *testing.T) {
	path := writeCompressedRollout(t, rolloutFixture())

	record, err := NewParser().ParseSession(context.Background(), session.FileRef{
		Source: Source,
		Format: session.FileFormatJSONLZstd,
		Path:   path,
	})
	if err != nil {
		t.Fatal(err)
	}

	if record.SessionID != "019f44e4-5c01-7d22-9805-50cecaefde49" {
		t.Fatalf("unexpected session ID: %s", record.SessionID)
	}
	if record.TokenUsage.TotalTokens != 125 {
		t.Fatalf("unexpected total tokens: %d", record.TokenUsage.TotalTokens)
	}
}

func TestParserRejectsOtherSources(t *testing.T) {
	_, err := NewParser().ParseSession(context.Background(), session.FileRef{
		Source: "other",
		Path:   "unused",
	})
	if err == nil {
		t.Fatal("expected source mismatch to fail")
	}
}

func writeRolloutJSONL(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeCompressedRollout(t *testing.T, contents string) string {
	t.Helper()

	var buf bytes.Buffer
	encoder, err := zstd.NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encoder.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "rollout.jsonl.zst")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func rolloutFixture() string {
	return `{"timestamp":"2026-07-09T03:20:44Z","type":"session_meta","payload":{"session_id":"019f44e4-5c01-7d22-9805-50cecaefde49","id":"019f44e4-5c01-7d22-9805-50cecaefde49","cwd":"/tmp/project","cli_version":"0.142.5","model_provider":"openai"}}
{"timestamp":"2026-07-09T03:20:45Z","type":"turn_context","payload":{"turn_id":"turn-1","cwd":"/tmp/project","model":"gpt-5"}}
{"timestamp":"2026-07-09T03:20:45Z","type":"turn_context","payload":{"turn_id":"turn-2","cwd":"/tmp/project","model":"gpt-5-mini"}}
{"timestamp":"2026-07-09T03:20:45Z","type":"turn_context","payload":{"turn_id":"turn-3","cwd":"/tmp/project","model":"gpt-5"}}
{"timestamp":"2026-07-09T03:20:46Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":25,"reasoning_output_tokens":5,"total_tokens":125},"last_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":25,"reasoning_output_tokens":5,"total_tokens":125},"model_context_window":258400}}}
{"timestamp":"2026-07-09T03:20:47Z","type":"response_item","payload":{"type":"message","role":"user","content":[]}}
{"timestamp":"2026-07-09T03:20:48Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[]}}
{"timestamp":"2026-07-09T03:20:49Z","type":"response_item","payload":{"type":"function_call","name":"exec_command"}}
{"timestamp":"2026-07-09T03:20:50Z","type":"response_item","payload":{"type":"custom_tool_call","name":"apply_patch"}}
`
}
