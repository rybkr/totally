package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestShowCommandPrintsSingleSessionReport(t *testing.T) {
	root := t.TempDir()
	sessionID := "019f44e4-5c01-7d22-9805-50cecaefde49"
	path := writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+sessionID+".jsonl", inspectFixtureForSession(sessionID))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"show", "--home", root, sessionID})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"Session     " + sessionID,
		"Source      codex",
		"Models      gpt-5, gpt-5-mini",
		"Project     /tmp/project",
		"Time        2026-07-09T03:20:44Z -> 2026-07-09T03:20:48Z (4s)",
		"Activity    2 turns, 1 messages, 1 tool calls",
		"Tokens      125 total; 100 input (40 cached); 25 output (incl. 5 reasoning)",
		"Transcript  " + path,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q in output:\n%s", want, output)
		}
	}
}

func TestShowCommandPrintsMostRecentlyUpdatedSession(t *testing.T) {
	root := t.TempDir()
	olderID := "019f44e4-5c01-7d22-9805-50cecaefde49"
	latestID := "019f44e4-5c01-7d22-9805-50cecaefde50"
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+olderID+".jsonl", inspectFixtureForSessionAt(olderID, time.Date(2026, 7, 8, 3, 20, 44, 0, time.UTC)))
	writeRolloutContents(t, root, "sessions/2026/07/09/rollout-2026-07-09T20-20-44-"+latestID+".jsonl", inspectFixtureForSessionAt(latestID, time.Date(2026, 7, 9, 3, 20, 44, 0, time.UTC)))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"show", "--home", root, "--latest"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Session     "+latestID) {
		t.Fatalf("expected latest session %q, got:\n%s", latestID, stdout.String())
	}
}

func TestShowCommandPrintsJSONReport(t *testing.T) {
	root := t.TempDir()
	sessionID := "019f44e4-5c01-7d22-9805-50cecaefde49"
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+sessionID+".jsonl", inspectFixtureForSession(sessionID))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"show", "--home", root, sessionID, "--format", "json"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	var report showReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	if report.SessionID != sessionID {
		t.Fatalf("unexpected session ID: %s", report.SessionID)
	}
	if report.Status != nil {
		t.Fatalf("expected unknown status to be null, got %q", *report.Status)
	}
	if report.CreatedAt == nil || *report.CreatedAt != "2026-07-09T03:20:44Z" {
		t.Fatalf("unexpected created_at: %+v", report.CreatedAt)
	}
	if report.UpdatedAt == nil || *report.UpdatedAt != "2026-07-09T03:20:48Z" {
		t.Fatalf("unexpected updated_at: %+v", report.UpdatedAt)
	}
	if report.DurationSeconds == nil || *report.DurationSeconds != 4 {
		t.Fatalf("unexpected duration_seconds: %+v", report.DurationSeconds)
	}
	if report.Project == nil || *report.Project != "/tmp/project" {
		t.Fatalf("unexpected project: %+v", report.Project)
	}
	if len(report.Models) != 2 || report.Models[0] != "gpt-5" || report.Models[1] != "gpt-5-mini" {
		t.Fatalf("unexpected models: %+v", report.Models)
	}
	if report.Turns != 2 || report.Messages != 1 || report.ToolCalls != 1 {
		t.Fatalf("unexpected activity: %+v", report)
	}
	if report.TokenUsage.TotalTokens != 125 || report.TokenUsage.ReasoningTokens != 5 {
		t.Fatalf("unexpected token usage: %+v", report.TokenUsage)
	}
}

func TestFormatShowTokenUsageMakesSubsetRelationshipsExplicit(t *testing.T) {
	got := formatShowTokenUsage(showTokenUsageReport{
		InputTokens:       1_043_777,
		CachedInputTokens: 936_704,
		OutputTokens:      12_788,
		ReasoningTokens:   3_756,
		TotalTokens:       1_056_565,
	})
	want := "1.06M total; 1.04M input (936.7K cached); 12.8K output (incl. 3.8K reasoning)"
	if got != want {
		t.Fatalf("formatShowTokenUsage() = %q, want %q", got, want)
	}
}

func TestShowCommandRejectsMissingSessionID(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"show"})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected missing session ID to fail")
	}
	if ExitCode(err) != 1 {
		t.Fatalf("expected exit code 1, got %d", ExitCode(err))
	}
}

func TestShowCommandRejectsMalformedSessionID(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"show", "missing-session"})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected malformed session ID to fail")
	}
	if !strings.Contains(err.Error(), `malformed session ID "missing-session"`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if ExitCode(err) != 1 {
		t.Fatalf("expected exit code 1, got %d", ExitCode(err))
	}
}

func TestShowCommandRejectsPathTarget(t *testing.T) {
	root := t.TempDir()
	path := writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde49.jsonl", inspectFixture())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"show", path})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected path target to fail")
	}
	if !strings.Contains(err.Error(), "malformed session ID") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShowCommandErrorsForUnknownSessionID(t *testing.T) {
	root := t.TempDir()
	sessionID := "019f44e4-5c01-7d22-9805-50cecaefde49"
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"show", "--home", root, sessionID})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected missing session to fail")
	}
	if !strings.Contains(err.Error(), `no session found for "`+sessionID+`"`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if ExitCode(err) != 1 {
		t.Fatalf("expected exit code 1, got %d", ExitCode(err))
	}
}

func TestShowCommandInvalidFormatIsUsageError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"show", "019f44e4-5c01-7d22-9805-50cecaefde49", "--format", "xml"})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected invalid format to fail")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %d", ExitCode(err))
	}
}

func TestInspectCommandIsNotRegistered(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"inspect"})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected retired inspect command to fail")
	}
	if !strings.Contains(err.Error(), `unknown command "inspect"`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if ExitCode(err) != 1 {
		t.Fatalf("expected exit code 1, got %d", ExitCode(err))
	}
}

func inspectFixture() string {
	return inspectFixtureForSession("019f44e4-5c01-7d22-9805-50cecaefde49")
}

func inspectFixtureForSession(sessionID string) string {
	return inspectFixtureForSessionAt(sessionID, time.Date(2026, 7, 9, 3, 20, 44, 0, time.UTC))
}

func inspectFixtureForSessionAt(sessionID string, start time.Time) string {
	return `{"timestamp":"` + start.Format(time.RFC3339) + `","type":"session_meta","payload":{"session_id":"` + sessionID + `","cwd":"/tmp/project","cli_version":"0.142.5","model_provider":"openai"}}
{"timestamp":"` + start.Add(time.Second).Format(time.RFC3339) + `","type":"turn_context","payload":{"cwd":"/tmp/project","model":"gpt-5"}}
{"timestamp":"` + start.Add(time.Second).Format(time.RFC3339) + `","type":"turn_context","payload":{"cwd":"/tmp/project","model":"gpt-5-mini"}}
{"timestamp":"` + start.Add(2*time.Second).Format(time.RFC3339) + `","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":25,"reasoning_output_tokens":5,"total_tokens":125}}}}
{"timestamp":"` + start.Add(3*time.Second).Format(time.RFC3339) + `","type":"response_item","payload":{"type":"message","role":"user","content":[]}}
{"timestamp":"` + start.Add(4*time.Second).Format(time.RFC3339) + `","type":"response_item","payload":{"type":"function_call","name":"exec_command"}}
`
}
