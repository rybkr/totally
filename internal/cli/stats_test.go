package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
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

	groups := groupStatsRecords([]session.Record{record}, []string{"model"})
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

func TestGroupStatsRecordsByProviderAndModelUsesUsageSegmentsAdditively(t *testing.T) {
	created := time.Date(2026, time.July, 8, 12, 0, 0, 0, time.UTC)
	record := session.Record{
		SessionID: "multi-provider-model",
		CreatedAt: created, UpdatedAt: created.Add(2 * time.Minute),
		Provider: "provider-a", Models: []string{"model-a", "model-b"}, FirstPrompt: "help",
		Turns: 3, Messages: 5, ToolCalls: 2,
		TokenUsage: session.TokenUsage{InputTokens: 350, TotalTokens: 350},
		UsageSegments: []session.UsageSegment{
			{Provider: "provider-a", Model: "model-a", TokenUsage: session.TokenUsage{InputTokens: 100, TotalTokens: 100}},
			{Provider: "provider-b", Model: "model-a", TokenUsage: session.TokenUsage{InputTokens: 200, TotalTokens: 200}},
			{Provider: "provider-b", Model: "model-b", TokenUsage: session.TokenUsage{InputTokens: 50, TotalTokens: 50}},
		},
	}
	catalog := pricing.DefaultCatalog()
	for _, rate := range []pricing.Rate{
		{Provider: "provider-a", Model: "model-a", InputPerMillionUSD: "1", CachedInputPerMillionUSD: "1", OutputPerMillionUSD: "1", EffectiveFrom: "2020-01-01"},
		{Provider: "provider-b", Model: "model-a", InputPerMillionUSD: "2", CachedInputPerMillionUSD: "2", OutputPerMillionUSD: "2", EffectiveFrom: "2020-01-01"},
		{Provider: "provider-b", Model: "model-b", InputPerMillionUSD: "3", CachedInputPerMillionUSD: "3", OutputPerMillionUSD: "3", EffectiveFrom: "2020-01-01"},
	} {
		if err := catalog.Overlay(rate); err != nil {
			t.Fatal(err)
		}
	}

	providerGroups := groupStatsRecords([]session.Record{record}, []string{"provider"})
	providerA := summarizeGroupedStats(providerGroups["provider-a"], catalog)
	providerB := summarizeGroupedStats(providerGroups["provider-b"], catalog)
	if providerA.TokenUsage.TotalTokens != 100 || providerB.TokenUsage.TotalTokens != 250 {
		t.Fatalf("provider usage was not split by segments: a=%+v b=%+v", providerA.TokenUsage, providerB.TokenUsage)
	}
	if providerA.Sessions != 0 || providerB.Sessions != 1 || providerB.Turns != 3 || providerB.DurationSeconds != 120 {
		t.Fatalf("session stats were not assigned once to primary provider: a=%+v b=%+v", providerA, providerB)
	}

	combined := groupStatsRecords([]session.Record{record}, []string{"model", "provider"})
	wantTokens := map[string]int64{
		"model-a\x1fprovider-a": 100,
		"model-a\x1fprovider-b": 200,
		"model-b\x1fprovider-b": 50,
	}
	var tokens, sessions, prompts, duration int64
	var turns, messages, tools int
	groupCost := new(big.Rat)
	for key, want := range wantTokens {
		report := summarizeGroupedStats(combined[key], catalog)
		if report.TokenUsage.TotalTokens != want {
			t.Fatalf("unexpected usage for %q: got %d want %d", key, report.TokenUsage.TotalTokens, want)
		}
		tokens += report.TokenUsage.TotalTokens
		sessions += int64(report.Sessions)
		prompts += int64(report.Prompts)
		duration += report.DurationSeconds
		turns += report.Turns
		messages += report.Messages
		tools += report.ToolCalls
		if report.Cost.AmountUSD == nil {
			t.Fatalf("missing cost for %q: %+v", key, report.Cost)
		}
		amount, ok := new(big.Rat).SetString(*report.Cost.AmountUSD)
		if !ok {
			t.Fatalf("invalid cost for %q: %q", key, *report.Cost.AmountUSD)
		}
		groupCost.Add(groupCost, amount)
	}
	if len(combined) != len(wantTokens) {
		t.Fatalf("unexpected combined groups: %+v", combined)
	}
	if tokens != 350 || sessions != 1 || prompts != 1 || duration != 120 || turns != 3 || messages != 5 || tools != 2 {
		t.Fatalf("combined groups were not additive: tokens=%d sessions=%d prompts=%d duration=%d turns=%d messages=%d tools=%d", tokens, sessions, prompts, duration, turns, messages, tools)
	}
	overall := summarizeStats([]session.Record{record}, catalog)
	if overall.Cost.AmountUSD == nil {
		t.Fatalf("missing overall cost: %+v", overall.Cost)
	}
	overallCost, ok := new(big.Rat).SetString(*overall.Cost.AmountUSD)
	if !ok || groupCost.Cmp(overallCost) != 0 {
		t.Fatalf("grouped cost was not additive: groups=%s overall=%v", groupCost.RatString(), overall.Cost.AmountUSD)
	}

	filteredProvider := filterStatsRecords([]session.Record{record}, statsOptions{provider: "PROVIDER-B"})
	if len(filteredProvider) != 1 {
		t.Fatalf("provider filter did not match usage segment: %+v", filteredProvider)
	}
	providerFilteredStats := summarizeStats(filteredProvider, catalog)
	if providerFilteredStats.TokenUsage.TotalTokens != 250 || providerFilteredStats.Sessions != 1 || providerFilteredStats.Turns != 3 || len(providerFilteredStats.Cost.Components) != 2 || providerFilteredStats.Cost.AmountUSD == nil || *providerFilteredStats.Cost.AmountUSD != "0.00055" {
		t.Fatalf("provider filter leaked other-provider attribution: %+v", providerFilteredStats)
	}
	filteredGroups := groupStatsRecords(filteredProvider, []string{"model", "provider"})
	var filteredGroupTokens int64
	var filteredGroupSessions int
	for _, records := range filteredGroups {
		report := summarizeGroupedStats(records, catalog)
		filteredGroupTokens += report.TokenUsage.TotalTokens
		filteredGroupSessions += report.Sessions
	}
	if filteredGroupTokens != 250 || filteredGroupSessions != 1 {
		t.Fatalf("grouping after provider filter was not additive: tokens=%d sessions=%d groups=%+v", filteredGroupTokens, filteredGroupSessions, filteredGroups)
	}

	filteredModel := filterStatsRecords([]session.Record{record}, statsOptions{model: "MODEL-A"})
	modelFilteredStats := summarizeStats(filteredModel, catalog)
	if modelFilteredStats.TokenUsage.TotalTokens != 300 || len(modelFilteredStats.Cost.Components) != 2 || modelFilteredStats.Cost.AmountUSD == nil || *modelFilteredStats.Cost.AmountUSD != "0.0005" {
		t.Fatalf("model filter leaked other-model attribution: %+v", modelFilteredStats)
	}

	intersection := filterStatsRecords([]session.Record{record}, statsOptions{provider: "provider-b", model: "model-a"})
	intersectionStats := summarizeStats(intersection, catalog)
	if intersectionStats.TokenUsage.TotalTokens != 200 || len(intersectionStats.Cost.Components) != 1 || intersectionStats.Cost.AmountUSD == nil || *intersectionStats.Cost.AmountUSD != "0.0004" {
		t.Fatalf("combined provider/model filters did not intersect: %+v", intersectionStats)
	}
	if got := filterStatsRecords([]session.Record{record}, statsOptions{provider: "PROVIDER-A"}); len(got) != 1 || got[0].TokenUsage.TotalTokens != 100 {
		t.Fatalf("provider filter stopped matching canonical-provider segment: %+v", got)
	}
	if got := filterStatsRecords([]session.Record{record}, statsOptions{provider: "provider-c"}); len(got) != 0 {
		t.Fatalf("provider filter matched unrelated provider: %+v", got)
	}
}

func TestSummarizeStatsKeepsAllUnpricedServiceTiersUnavailable(t *testing.T) {
	created := time.Date(2026, time.July, 8, 12, 0, 0, 0, time.UTC)
	usage := session.TokenUsage{InputTokens: 100, TotalTokens: 100}
	record := session.Record{
		CreatedAt:     created,
		TokenUsage:    usage,
		UsageSegments: []session.UsageSegment{{Provider: "openai", Model: "gpt-5", ServiceTier: "flex", TokenUsage: usage}},
	}
	report := summarizeStats([]session.Record{record}, pricing.DefaultCatalog())
	if report.Cost.Status != "unavailable" || report.Cost.AmountUSD != nil || len(report.Cost.Components) != 0 || len(report.Cost.Missing) != 1 || len(report.Cost.Limitations) != 1 {
		t.Fatalf("all-unpriced aggregate should remain unavailable: %+v", report.Cost)
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
	cmd.SetArgs([]string{"stats", "--home", root, "--cwd", root, "--provider", "OPENAI", "--model", "GPT-5-MINI", "--by", "provider", "--format", "json"})
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

func TestStatsCommandGroupsByCWD(t *testing.T) {
	root := t.TempDir()
	sessionID := "019f44e4-5c01-7d22-9805-50cecaefde49"
	contents := strings.Replace(inspectFixtureForSession(sessionID), "/tmp/project", root, -1)
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+sessionID+".jsonl", contents)

	var stdout, stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"stats", "--home", root, "--by", "cwd", "--format", "json"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run --by cwd: %v\\nstderr: %s", err, stderr.String())
	}
	var report groupedStatsReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode --by cwd report: %v\\n%s", err, stdout.String())
	}
	if report.By != "cwd" || len(report.Groups) != 1 || report.Groups[0].Group != root {
		t.Fatalf("unexpected --by cwd report: %+v", report)
	}
}

func TestStatsCommandGroupsByCompositeDimensions(t *testing.T) {
	useTestPacificTime(t)

	root := t.TempDir()
	first := "019f44e4-5c01-7d22-9805-50cecaefde49"
	second := "019f44e4-5c01-7d22-9805-50cecaefde50"
	writeRolloutContents(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+first+".jsonl", inspectFixtureForSession(first))
	contents := strings.ReplaceAll(inspectFixtureForSession(second), "gpt-5-mini", "gpt-5")
	writeRolloutContents(t, root, "sessions/2026/07/09/rollout-2026-07-09T20-20-44-"+second+".jsonl", contents)

	var stdout, stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"stats", "--home", root, "--by", "day", "--by", "model", "--format", "json"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\\nstderr: %s", err, stderr.String())
	}
	var report groupedStatsReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v\\n%s", err, stdout.String())
	}
	if report.By != "day,model" || strings.Join(report.ByDimensions, ",") != "day,model" {
		t.Fatalf("unexpected dimensions: by=%q dimensions=%v", report.By, report.ByDimensions)
	}
	if len(report.Groups) != 2 || report.Groups[0].Group != "2026-07-08 / gpt-5" || report.Groups[1].Group != "2026-07-08 / gpt-5-mini" || strings.Join(report.Groups[0].Values, ",") != "2026-07-08,gpt-5" {
		t.Fatalf("unexpected composite groups: %+v", report.Groups)
	}
}

func TestStatsCommandRejectsUnknownGrouping(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"stats", "--by", "project"})
	if err := cmd.ExecuteContext(context.Background()); err == nil || !strings.Contains(err.Error(), "unknown --by value") {
		t.Fatalf("expected project grouping to be rejected, got %v", err)
	}
}
