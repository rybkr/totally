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

func TestSessionsCommandPrintsTable(t *testing.T) {
	root := t.TempDir()
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde49.jsonl", inspectFixture())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"sessions", "--home", root})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"SESSION\tCREATED\tUPDATED\tMODELS\tTURNS\tMESSAGES\tTOOLS\tTOKENS\tCWD",
		"019f44e4-5c01-7d22-9805-50cecaefde49",
		"gpt-5, gpt-5-mini",
		"\t2\t1\t1\t125\t/tmp/project",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q in output:\n%s", want, output)
		}
	}
}

func TestSessionsCommandPrintsJSON(t *testing.T) {
	root := t.TempDir()
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde49.jsonl", inspectFixture())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"--format", "json", "sessions", "--home", root})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	var records []session.Record
	if err := json.Unmarshal(stdout.Bytes(), &records); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 session, got %d", len(records))
	}
	if records[0].SessionID != "019f44e4-5c01-7d22-9805-50cecaefde49" {
		t.Fatalf("unexpected session ID: %s", records[0].SessionID)
	}
	if records[0].TokenUsage.TotalTokens != 125 {
		t.Fatalf("unexpected tokens: %d", records[0].TokenUsage.TotalTokens)
	}
}

func TestSessionsCommandPrintsIDs(t *testing.T) {
	root := t.TempDir()
	firstID := "019f44e4-5c01-7d22-9805-50cecaefde49"
	secondID := "019f44e4-5c01-7d22-9805-50cecaefde50"
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+firstID+".jsonl", inspectFixtureForSession(firstID))
	writeRolloutContents(t, root, "sessions/2026/07/09/rollout-2026-07-09T20-20-44-"+secondID+".jsonl", inspectFixtureForSession(secondID))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"sessions", "--home", root, "--ids"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	got := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(got) != 2 || got[0] != secondID || got[1] != firstID {
		t.Fatalf("unexpected IDs: %+v", got)
	}
}

func TestSessionsCommandPrintsPaths(t *testing.T) {
	root := t.TempDir()
	path := writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde49.jsonl", inspectFixture())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"sessions", "--home", root, "--paths"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	if got := strings.TrimSpace(stdout.String()); got != path {
		t.Fatalf("expected path %q, got %q", path, got)
	}
}

func TestSessionsCommandPrintsSummary(t *testing.T) {
	root := t.TempDir()
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde49.jsonl", inspectFixtureForSession("019f44e4-5c01-7d22-9805-50cecaefde49"))
	writeRolloutContents(t, root, "sessions/2026/07/09/rollout-2026-07-09T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde50.jsonl", inspectFixtureForSession("019f44e4-5c01-7d22-9805-50cecaefde50"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"sessions", "--home", root, "--summary"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"Sessions:   2",
		"Sources:    codex",
		"Models:     gpt-5, gpt-5-mini",
		"Tool calls: 2",
		"Total:     250",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q in output:\n%s", want, output)
		}
	}
}

func TestSessionsCommandPrintsSummaryJSON(t *testing.T) {
	root := t.TempDir()
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde49.jsonl", inspectFixture())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"--format", "json", "sessions", "--home", root, "--summary"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	var summary inspectSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	if summary.Sessions != 1 {
		t.Fatalf("unexpected session count: %d", summary.Sessions)
	}
	if summary.TokenUsage.TotalTokens != 125 {
		t.Fatalf("unexpected total tokens: %d", summary.TokenUsage.TotalTokens)
	}
}

func TestSessionsCommandLatestLimitPrintsLatestSessions(t *testing.T) {
	root := t.TempDir()
	oldestID := "019f44e4-5c01-7d22-9805-50cecaefde49"
	latestID := "019f44e4-5c01-7d22-9805-50cecaefde50"
	secondLatestID := "019f44e4-5c01-7d22-9805-50cecaefde51"
	oldest := writeRolloutContents(t, root, "sessions/2026/07/07/rollout-2026-07-07T20-20-44-"+oldestID+".jsonl", inspectFixtureForSession(oldestID))
	latest := writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+latestID+".jsonl", inspectFixtureForSession(latestID))
	secondLatest := writeRolloutContents(t, root, "sessions/2026/07/09/rollout-2026-07-09T20-20-44-"+secondLatestID+".jsonl", inspectFixtureForSession(secondLatestID))
	setFileTimes(t, oldest, time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC))
	setFileTimes(t, secondLatest, time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC))
	setFileTimes(t, latest, time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"sessions", "--home", root, "--latest", "--limit", "2", "--ids"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	got := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(got) != 2 || got[0] != latestID || got[1] != secondLatestID {
		t.Fatalf("unexpected latest IDs: %+v", got)
	}
}

func TestSessionsCommandRejectsMultiplePlainOutputModes(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"sessions", "--summary", "--ids"})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected multiple output modes to fail")
	}
	if !strings.Contains(err.Error(), "--summary, --ids, and --paths are mutually exclusive") {
		t.Fatalf("unexpected error: %v", err)
	}
}
