package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/rybkr/totally/internal/pricing"
)

func TestFormatCostEstimatePrintsCacheWriteUncertainty(t *testing.T) {
	amount, uncertainty := "0.321601", "0.018893"
	got := formatCostEstimate(pricing.Estimate{AmountUSD: &amount, UncertaintyUSD: &uncertainty, Status: "partial"})
	want := "~$0.321601 ± $0.018893 USD estimated (cache-write uncertainty)"
	if got != want {
		t.Fatalf("formatCostEstimate() = %q, want %q", got, want)
	}
}

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
		"CWD         /tmp/project",
		"Provider    openai",
		"Prompt      Explain this session",
		"Time        2026-07-09T03:20:44Z -> 2026-07-09T03:20:48Z (4s)",
		"Activity    2 turns, 1 messages, 1 tool calls",
		"Tokens      125 total; 100 input (40 cached); 25 output (incl. 5 reasoning)",
		"Cost        $0.000066 USD estimated (API-equivalent)",
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

func TestShowCommandLatestFilters(t *testing.T) {
	root := t.TempDir()
	matchingID := "019f44e4-5c01-7d22-9805-50cecaefde49"
	newerID := "019f44e4-5c01-7d22-9805-50cecaefde50"
	matching := strings.Replace(inspectFixtureForSessionAt(matchingID, time.Date(2026, 7, 8, 3, 20, 44, 0, time.UTC)), "/tmp/project", root, -1)
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+matchingID+".jsonl", matching)
	newer := strings.Replace(inspectFixtureForSessionAt(newerID, time.Date(2026, 7, 9, 3, 20, 44, 0, time.UTC)), "\"model_provider\":\"openai\"", "\"model_provider\":\"anthropic\"", 1)
	newer = strings.Replace(newer, "\"model\":\"gpt-5\"", "\"model\":\"claude-sonnet-4\"", 1)
	writeRolloutContents(t, root, "sessions/2026/07/09/rollout-2026-07-09T20-20-44-"+newerID+".jsonl", newer)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"show", "--home", root, "--latest", "--cwd", root, "--provider", "OPENAI", "--model", "GPT-5"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Session     "+matchingID) {
		t.Fatalf("expected matching session %q, got:\n%s", matchingID, stdout.String())
	}
}

func TestShowCommandLatestFiltersRequireLatest(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"show", "--provider", "openai", "019f44e4"})

	err := cmd.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "require --latest") {
		t.Fatalf("expected latest requirement error, got %v", err)
	}
}

func TestShowCommandLatestFiltersReportNoMatch(t *testing.T) {
	root := t.TempDir()
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde49.jsonl", inspectFixture())

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"show", "--home", root, "--latest", "--model", "missing-model"})

	err := cmd.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no sessions found matching --latest filters") {
		t.Fatalf("expected no match error, got %v", err)
	}
}

func TestShowLatestSkipsMalformedTranscript(t *testing.T) {
	root := t.TempDir()
	validID := "019f44e4-5c01-7d22-9805-50cecaefde49"
	badID := "019f44e4-5c01-7d22-9805-50cecaefde50"
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+validID+".jsonl", inspectFixtureForSession(validID))
	badPath := writeRolloutContents(t, root, "sessions/2026/07/09/rollout-2026-07-09T20-20-44-"+badID+".jsonl", "not json\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"show", "--home", root, "--latest"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Session     "+validID) {
		t.Fatalf("expected valid session %q, got:\n%s", validID, stdout.String())
	}
	if !strings.Contains(stderr.String(), "warning: skip session transcript "+badPath+": parse rollout line 1:") {
		t.Fatalf("missing transcript warning:\n%s", stderr.String())
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
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &fields); err != nil {
		t.Fatalf("invalid JSON object: %v\n%s", err, stdout.String())
	}
	if _, found := fields["status"]; found {
		t.Fatalf("status must not be present in JSON output: %s", stdout.String())
	}
	if _, found := fields["project"]; found {
		t.Fatalf("project must not be present in JSON output: %s", stdout.String())
	}
	if _, found := fields["cwd"]; !found {
		t.Fatalf("missing cwd in JSON output: %s", stdout.String())
	}
	var tokenUsage map[string]json.RawMessage
	if err := json.Unmarshal(fields["token_usage"], &tokenUsage); err != nil {
		t.Fatalf("invalid token_usage object: %v", err)
	}
	if _, found := tokenUsage["reasoning_output_tokens"]; !found {
		t.Fatalf("missing reasoning_output_tokens: %s", stdout.String())
	}
	if _, found := tokenUsage["reasoning_tokens"]; found {
		t.Fatalf("unexpected legacy reasoning_tokens key: %s", stdout.String())
	}
	if report.SessionID != sessionID {
		t.Fatalf("unexpected session ID: %s", report.SessionID)
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
	if report.CWD == nil || *report.CWD != "/tmp/project" {
		t.Fatalf("unexpected cwd: %+v", report.CWD)
	}
	if report.Provider == nil || *report.Provider != "openai" {
		t.Fatalf("unexpected provider: %+v", report.Provider)
	}
	if report.FirstPrompt == nil || *report.FirstPrompt != "Explain this session" {
		t.Fatalf("unexpected first prompt: %+v", report.FirstPrompt)
	}
	if report.CostUSD != 0.000066 {
		t.Fatalf("unexpected cost: %v", report.CostUSD)
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

func TestShowCommandTruncatesPromptUnlessFull(t *testing.T) {
	root := t.TempDir()
	sessionID := "019f44e4-5c01-7d22-9805-50cecaefde49"
	prompt := "  first\n\tsecond " + strings.Repeat("long ", 20)
	escapedPrompt, err := json.Marshal(prompt)
	if err != nil {
		t.Fatalf("marshal prompt: %v", err)
	}
	contents := strings.Replace(inspectFixtureForSession(sessionID), `"Explain this session"`, string(escapedPrompt), 1)
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+sessionID+".jsonl", contents)

	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "default",
			args: []string{"show", "--home", root, sessionID},
			want: "Prompt      " + formatSessionPrompt(prompt),
		},
		{
			name: "full",
			args: []string{"show", "--home", root, sessionID, "--full"},
			want: "Prompt      " + strings.TrimSpace(prompt),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			cmd := newTestRootCommand(t, &stdout, &stderr)
			cmd.SetArgs(test.args)

			if err := cmd.ExecuteContext(context.Background()); err != nil {
				t.Fatalf("run failed: %v\\nstderr: %s", err, stderr.String())
			}
			if !strings.Contains(stdout.String(), test.want) {
				t.Fatalf("missing %q in output:\\n%s", test.want, stdout.String())
			}
		})
	}
}

func TestShowPromptMaxForTerminalWidth(t *testing.T) {
	if got := showPromptMaxForTerminalWidth(48); got != 36 {
		t.Fatalf("showPromptMaxForTerminalWidth() = %d, want 36", got)
	}
	if got := showPromptMaxForTerminalWidth(20); got != sessionPromptMinRunes {
		t.Fatalf("showPromptMaxForTerminalWidth() = %d, want %d", got, sessionPromptMinRunes)
	}
	if got := showPromptMaxForTerminalWidth(200); got != sessionPromptMaxRunes {
		t.Fatalf("showPromptMaxForTerminalWidth() = %d, want %d", got, sessionPromptMaxRunes)
	}
}

func TestShowCommandJSONKeepsFullPrompt(t *testing.T) {
	root := t.TempDir()
	sessionID := "019f44e4-5c01-7d22-9805-50cecaefde49"
	prompt := "  first\n\tsecond " + strings.Repeat("long ", 20)
	escapedPrompt, err := json.Marshal(prompt)
	if err != nil {
		t.Fatalf("marshal prompt: %v", err)
	}
	contents := strings.Replace(inspectFixtureForSession(sessionID), `"Explain this session"`, string(escapedPrompt), 1)
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+sessionID+".jsonl", contents)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"show", "--home", root, sessionID, "--format", "json"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\\nstderr: %s", err, stderr.String())
	}
	var report showReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON output: %v\\n%s", err, stdout.String())
	}
	if report.FirstPrompt == nil || *report.FirstPrompt != strings.TrimSpace(prompt) {
		t.Fatalf("unexpected first_prompt: %+v", report.FirstPrompt)
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

func TestShowCommandAcceptsUniqueSessionIDPrefix(t *testing.T) {
	root := t.TempDir()
	sessionID := "019f44e4-5c01-7d22-9805-50cecaefde49"
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+sessionID+".jsonl", inspectFixtureForSession(sessionID))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"show", "--home", root, "019f44e4"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Session     "+sessionID) {
		t.Fatalf("expected session %q, got:\n%s", sessionID, stdout.String())
	}
}

func TestShowCommandRejectsAmbiguousSessionIDPrefix(t *testing.T) {
	root := t.TempDir()
	firstID := "019f44e4-5c01-7d22-9805-50cecaefde49"
	secondID := "019f44e4-5c01-7d22-9805-50cecaefde50"
	firstContents := strings.Replace(inspectFixtureForSession(firstID), "/tmp/project", "/tmp/first-project", -1)
	firstContents = strings.Replace(firstContents, "Explain this session", "First matching prompt", 1)
	secondContents := strings.Replace(inspectFixtureForSession(secondID), "/tmp/project", "/tmp/second-project", -1)
	secondContents = strings.Replace(secondContents, "Explain this session", "Second matching prompt", 1)
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+firstID+".jsonl", firstContents)
	writeRolloutContents(t, root, "sessions/2026/07/09/rollout-2026-07-09T20-20-44-"+secondID+".jsonl", secondContents)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"show", "--home", root, "019f44e4"})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected ambiguous prefix to fail")
	}
	for _, want := range []string{
		`multiple sessions found for UUID prefix "019f44e4"`,
		"SESSION ID\tCWD\tPROMPT",
		firstID + "\t/tmp/first-project\tFirst matching prompt",
		secondID + "\t/tmp/second-project\tSecond matching prompt",
		"provide a longer prefix or pass --agent or --home to narrow the search",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in error:\n%v", want, err)
		}
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

func TestCompletionCommandIsNotRegistered(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"completion"})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected completion command to fail")
	}
	if !strings.Contains(err.Error(), `unknown command "completion"`) {
		t.Fatalf("unexpected error: %v", err)
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
{"timestamp":"` + start.Add(2*time.Second).Format(time.RFC3339) + `","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":25,"reasoning_output_tokens":5,"total_tokens":125},"last_token_usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":25,"reasoning_output_tokens":5,"total_tokens":125}}}}
{"timestamp":"` + start.Add(3*time.Second).Format(time.RFC3339) + `","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Explain this session"}]}}
{"timestamp":"` + start.Add(4*time.Second).Format(time.RFC3339) + `","type":"response_item","payload":{"type":"function_call","name":"exec_command"}}
`
}
