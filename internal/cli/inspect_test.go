package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/rybkr/totally/internal/session"
)

func TestInspectCommandPrintsSessionSummaryForPath(t *testing.T) {
	root := t.TempDir()
	path := writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde49.jsonl", inspectFixture())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"inspect", path})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"Session:    019f44e4-5c01-7d22-9805-50cecaefde49",
		"Source:     codex",
		"Path:       " + path,
		"Models:     gpt-5, gpt-5-mini",
		"Tool calls: 1",
		"Tokens:",
		"Total:     125",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q in output:\n%s", want, output)
		}
	}
}

func TestInspectCommandFindsSessionByID(t *testing.T) {
	root := t.TempDir()
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde49.jsonl", inspectFixture())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"inspect", "--home", root, "019f44e4-5c01-7d22-9805-50cecaefde49"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "Session:    019f44e4-5c01-7d22-9805-50cecaefde49") {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestInspectCommandPrintsJSON(t *testing.T) {
	root := t.TempDir()
	path := writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde49.jsonl", inspectFixture())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"--format", "json", "inspect", path})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	var record session.Record
	if err := json.Unmarshal(stdout.Bytes(), &record); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	if record.SessionID != "019f44e4-5c01-7d22-9805-50cecaefde49" {
		t.Fatalf("unexpected session ID: %s", record.SessionID)
	}
	if record.TokenUsage.TotalTokens != 125 {
		t.Fatalf("unexpected total tokens: %d", record.TokenUsage.TotalTokens)
	}
	if len(record.Models) != 2 || record.Models[0] != "gpt-5" || record.Models[1] != "gpt-5-mini" {
		t.Fatalf("unexpected models: %+v", record.Models)
	}
}

func TestInspectCommandLatestUsesUpdatedTime(t *testing.T) {
	root := t.TempDir()
	newerCreatedID := "019f44e4-5c01-7d22-9805-50cecaefde49"
	newerUpdatedID := "019f44e4-5c01-7d22-9805-50cecaefde50"
	createdNewerUpdatedOlder := writeRolloutContents(t, root, "sessions/2026/07/09/rollout-2026-07-09T20-20-44-"+newerCreatedID+".jsonl", inspectFixtureForSession(newerCreatedID))
	createdOlderUpdatedNewer := writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+newerUpdatedID+".jsonl", inspectFixtureForSession(newerUpdatedID))
	setFileTimes(t, createdNewerUpdatedOlder, time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC))
	setFileTimes(t, createdOlderUpdatedNewer, time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"inspect", "--home", root, "--latest"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "Session:    "+newerUpdatedID) {
		t.Fatalf("expected updated-newer session, got:\n%s", stdout.String())
	}
}

func TestInspectCommandErrorsForUnknownSessionID(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"inspect", "missing-session"})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected missing session to fail")
	}
	if !strings.Contains(err.Error(), `no session found for "missing-session"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func inspectFixture() string {
	return inspectFixtureForSession("019f44e4-5c01-7d22-9805-50cecaefde49")
}

func inspectFixtureForSession(sessionID string) string {
	return `{"timestamp":"2026-07-09T03:20:44Z","type":"session_meta","payload":{"session_id":"` + sessionID + `","cwd":"/tmp/project","cli_version":"0.142.5","model_provider":"openai"}}
{"timestamp":"2026-07-09T03:20:45Z","type":"turn_context","payload":{"cwd":"/tmp/project","model":"gpt-5"}}
{"timestamp":"2026-07-09T03:20:45Z","type":"turn_context","payload":{"cwd":"/tmp/project","model":"gpt-5-mini"}}
{"timestamp":"2026-07-09T03:20:46Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":25,"reasoning_output_tokens":5,"total_tokens":125}}}}
{"timestamp":"2026-07-09T03:20:47Z","type":"response_item","payload":{"type":"message","role":"user","content":[]}}
{"timestamp":"2026-07-09T03:20:48Z","type":"response_item","payload":{"type":"function_call","name":"exec_command"}}
`
}
