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
		"SESSION ID\tSTARTED\tCWD\tMODEL\tTOKENS\tCOST\tDURATION\tPROMPT",
		"019f44e4-5c01\t2026-07-09T03:20:44Z\t/tmp/project\tgpt-5,gpt-5-mini\t125\t$",
		"\t4s\tExplain this session",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q in output:\n%s", want, output)
		}
	}
}

func TestSessionsCommandFullPrintsFullSessionID(t *testing.T) {
	root := t.TempDir()
	sessionID := "019f44e4-5c01-7d22-9805-50cecaefde49"
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+sessionID+".jsonl", inspectFixtureForSession(sessionID))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"sessions", "--home", root, "--full"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), sessionID+"\t2026-07-09T03:20:44Z\t/tmp/project") {
		t.Fatalf("expected full session ID in output:\\n%s", stdout.String())
	}
}

func TestFormatSessionIDTruncatesToPrefix(t *testing.T) {
	if got := formatSessionID("019f44e4-5c01-7d22-9805-50cecaefde49"); got != "019f44e4-5c01" {
		t.Fatalf("formatSessionID() = %q, want UUID prefix", got)
	}
}

func TestSessionsCommandNoPagerPrintsTableDirectly(t *testing.T) {
	root := t.TempDir()
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde49.jsonl", inspectFixture())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"--no-pager", "sessions", "--home", root})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "SESSION ID\tSTARTED\tCWD\tMODEL\tTOKENS\tCOST\tDURATION\tPROMPT") {
		t.Fatalf("expected direct table output, got:\n%s", stdout.String())
	}
}

func TestFormatSessionPromptNormalizesAndTruncates(t *testing.T) {
	prompt := "  first\n\tsecond " + strings.Repeat("long ", 40)
	got := formatSessionPrompt(prompt)
	if len([]rune(got)) != sessionPromptMaxRunes || !strings.HasSuffix(got, "...") {
		t.Fatalf("formatSessionPrompt() = %q, want %d runes ending in ellipsis", got, sessionPromptMaxRunes)
	}
	if strings.ContainsAny(got, "\n\t") {
		t.Fatalf("formatSessionPrompt() retained whitespace: %q", got)
	}
}

func TestSessionPromptMaxForTerminalWidth(t *testing.T) {
	const (
		sessionID = "019f44e4-5c01"
		cwd       = "/tmp/project"
	)
	if got := sessionPromptMaxForTerminalWidth(48, sessionID, cwd); got != 16 {
		t.Fatalf("sessionPromptMaxForTerminalWidth() = %d, want 16", got)
	}
	if got := sessionPromptMaxForTerminalWidth(35, sessionID, cwd); got != sessionPromptMinRunes {
		t.Fatalf("sessionPromptMaxForTerminalWidth() = %d, want %d", got, sessionPromptMinRunes)
	}
	if got := sessionPromptMaxForTerminalWidth(200, sessionID, cwd); got != sessionPromptMaxRunes {
		t.Fatalf("sessionPromptMaxForTerminalWidth() = %d, want %d", got, sessionPromptMaxRunes)
	}
}

func TestFormatSessionPromptToWidthUsesTerminalDisplayWidth(t *testing.T) {
	if got := formatSessionPromptToWidth("界界界界", 7); got != "界界..." {
		t.Fatalf("formatSessionPromptToWidth() = %q, want %q", got, "界界...")
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
	var document []map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("invalid JSON document: %v", err)
	}
	if _, ok := document[0]["SessionID"]; ok {
		t.Fatalf("unexpected Go-style JSON key: %s", stdout.String())
	}
	if _, ok := document[0]["session_id"]; !ok {
		t.Fatalf("missing session_id: %s", stdout.String())
	}
	var usage map[string]json.RawMessage
	if err := json.Unmarshal(document[0]["token_usage"], &usage); err != nil {
		t.Fatalf("invalid token_usage: %v", err)
	}
	if _, ok := usage["reasoning_output_tokens"]; !ok {
		t.Fatalf("missing reasoning_output_tokens: %s", stdout.String())
	}
	for _, key := range []string{"duration_seconds", "cost_usd", "cost"} {
		if _, ok := document[0][key]; !ok {
			t.Fatalf("missing %s: %s", key, stdout.String())
		}
	}
}

func TestSessionsCommandSkipsMalformedTranscript(t *testing.T) {
	root := t.TempDir()
	validID := "019f44e4-5c01-7d22-9805-50cecaefde49"
	badID := "019f44e4-5c01-7d22-9805-50cecaefde50"
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+validID+".jsonl", inspectFixtureForSession(validID))
	badPath := writeRolloutContents(t, root, "sessions/2026/07/09/rollout-2026-07-09T20-20-44-"+badID+".jsonl", "not json\n")

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
	if len(records) != 1 || records[0].SessionID != validID {
		t.Fatalf("unexpected records: %+v", records)
	}
	if !strings.Contains(stderr.String(), "warning: skip session transcript "+badPath+": parse rollout line 1:") {
		t.Fatalf("missing transcript warning:\n%s", stderr.String())
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
	var document map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("invalid JSON document: %v", err)
	}
	if _, ok := document["TokenUsage"]; ok {
		t.Fatalf("unexpected Go-style JSON key: %s", stdout.String())
	}
	if _, ok := document["token_usage"]; !ok {
		t.Fatalf("missing token_usage: %s", stdout.String())
	}
}

func TestSessionsCommandLatestLimitPrintsLatestSessions(t *testing.T) {
	root := t.TempDir()
	oldestID := "019f44e4-5c01-7d22-9805-50cecaefde49"
	latestID := "019f44e4-5c01-7d22-9805-50cecaefde50"
	secondLatestID := "019f44e4-5c01-7d22-9805-50cecaefde51"
	oldest := writeRolloutContents(t, root, "sessions/2026/07/07/rollout-2026-07-07T20-20-44-"+oldestID+".jsonl", inspectFixtureForSessionAt(oldestID, time.Date(2026, 7, 7, 3, 20, 44, 0, time.UTC)))
	latest := writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+latestID+".jsonl", inspectFixtureForSessionAt(latestID, time.Date(2026, 7, 10, 3, 20, 44, 0, time.UTC)))
	secondLatest := writeRolloutContents(t, root, "sessions/2026/07/09/rollout-2026-07-09T20-20-44-"+secondLatestID+".jsonl", inspectFixtureForSessionAt(secondLatestID, time.Date(2026, 7, 9, 3, 20, 44, 0, time.UTC)))
	setFileTimes(t, oldest, time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC))
	setFileTimes(t, secondLatest, time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC))
	setFileTimes(t, latest, time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC))

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
