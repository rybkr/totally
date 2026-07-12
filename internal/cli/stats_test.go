package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestStatsCommandAggregatesSessionsAndEmitsJSON(t *testing.T) {
	root := t.TempDir()
	first := "019f44e4-5c01-7d22-9805-50cecaefde49"
	second := "019f44e4-5c01-7d22-9805-50cecaefde50"
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+first+".jsonl", inspectFixtureForSession(first))
	writeRolloutContents(t, root, "sessions/2026/07/09/rollout-2026-07-09T20-20-44-"+second+".jsonl", inspectFixtureForSession(second))

	var stdout, stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"stats", "--home", root, "--format", "json"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}
	var report statsReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if report.Sessions != 2 || report.Prompts != 2 || report.DurationSeconds != 8 {
		t.Fatalf("unexpected aggregate: %+v", report)
	}
	if report.TokenUsage.TotalTokens != 250 || report.Cost.AmountUSD == nil || *report.Cost.AmountUSD != "0.000132" {
		t.Fatalf("unexpected usage or cost: %+v", report)
	}
}

func TestStatsCommandGroupsAndFiltersLikeShow(t *testing.T) {
	root := t.TempDir()
	matching := "019f44e4-5c01-7d22-9805-50cecaefde49"
	nonMatching := "019f44e4-5c01-7d22-9805-50cecaefde50"
	contents := strings.Replace(inspectFixtureForSession(matching), "/tmp/project", root, -1)
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+matching+".jsonl", contents)
	other := strings.Replace(inspectFixtureForSession(nonMatching), "\"model_provider\":\"openai\"", "\"model_provider\":\"anthropic\"", 1)
	writeRolloutContents(t, root, "sessions/2026/07/09/rollout-2026-07-09T20-20-44-"+nonMatching+".jsonl", other)

	var stdout, stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"stats", "--home", root, "--cwd", root, "--provider", "OPENAI", "--model", "GPT-5", "--by", "provider", "--format", "json"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}
	var report groupedStatsReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if report.By != "provider" || report.Total.Sessions != 1 || len(report.Groups) != 1 || report.Groups[0].Group != "openai" {
		t.Fatalf("unexpected grouped report: %+v", report)
	}
}

func TestStatsCommandRejectsUnknownGrouping(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"stats", "--by", "cost"})
	if err := cmd.ExecuteContext(context.Background()); err == nil || !strings.Contains(err.Error(), "unknown --by value") {
		t.Fatalf("expected invalid --by error, got %v", err)
	}
}
