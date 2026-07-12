package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/rybkr/totally/internal/pricing"
	"github.com/rybkr/totally/internal/session"
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

func TestGroupStatsRecordsByModelAttributesUsageAndAssignsSessionStatsOnce(t *testing.T) {
	created := time.Date(2026, time.July, 8, 12, 0, 0, 0, time.UTC)
	record := session.Record{
		SessionID: "multi-model",
		CreatedAt: created, UpdatedAt: created.Add(2 * time.Minute),
		Models: []string{"gpt-5", "gpt-5-mini"}, FirstPrompt: "help",
		Turns: 3, Messages: 5, ToolCalls: 2,
		TokenUsage: session.TokenUsage{TotalTokens: 150},
		UsageSegments: []session.UsageSegment{
			{Provider: "openai", Model: "gpt-5", TokenUsage: session.TokenUsage{InputTokens: 80, TotalTokens: 100}},
			{Provider: "openai", Model: "gpt-5-mini", TokenUsage: session.TokenUsage{InputTokens: 40, TotalTokens: 50}},
		},
	}

	groups := groupStatsRecords([]session.Record{record}, "model")
	primary := summarizeGroupedStats(groups["gpt-5"], pricing.DefaultCatalog())
	secondary := summarizeGroupedStats(groups["gpt-5-mini"], pricing.DefaultCatalog())
	if primary.Sessions != 1 || primary.Prompts != 1 || primary.DurationSeconds != 120 || primary.Turns != 3 || primary.Messages != 5 || primary.ToolCalls != 2 || primary.TokenUsage.TotalTokens != 100 {
		t.Fatalf("unexpected primary model stats: %+v", primary)
	}
	if secondary.Sessions != 0 || secondary.Prompts != 0 || secondary.DurationSeconds != 0 || secondary.Turns != 0 || secondary.Messages != 0 || secondary.ToolCalls != 0 || secondary.TokenUsage.TotalTokens != 50 {
		t.Fatalf("unexpected secondary model stats: %+v", secondary)
	}
	if len(primary.Cost.Components) != 1 || len(secondary.Cost.Components) != 1 || primary.Cost.Components[0].TokenUsage.TotalTokens != 100 || secondary.Cost.Components[0].TokenUsage.TotalTokens != 50 {
		t.Fatalf("cost components were not attributed by model: primary=%+v secondary=%+v", primary.Cost.Components, secondary.Cost.Components)
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
