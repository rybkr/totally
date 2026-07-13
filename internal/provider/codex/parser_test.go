package codex

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/rybkr/totally/internal/session"
)

func TestParserParseSession(t *testing.T) {
	path := writeRolloutJSONL(t, rolloutFixture())
	created := time.Date(2026, 7, 8, 20, 20, 44, 0, time.UTC)
	updated := time.Date(2026, 7, 9, 8, 0, 0, 0, time.UTC)
	wantCreated := time.Date(2026, 7, 9, 3, 20, 44, 0, time.UTC)
	wantUpdated := time.Date(2026, 7, 9, 3, 20, 50, 0, time.UTC)

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
	if record.Path != path || record.SizeBytes != 123 {
		t.Fatalf("file metadata was not preserved: %+v", record)
	}
	if !record.CreatedAt.Equal(wantCreated) || !record.UpdatedAt.Equal(wantUpdated) {
		t.Fatalf("unexpected transcript timestamps: created=%s updated=%s", record.CreatedAt, record.UpdatedAt)
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
	if record.FirstPrompt != "Explain this session" {
		t.Fatalf("unexpected first prompt: %q", record.FirstPrompt)
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
	if len(record.UsageSegments) != 1 || record.UsageSegments[0].Model != "gpt-5" || record.UsageSegments[0].TokenUsage != wantUsage {
		t.Fatalf("unexpected usage segments: %+v", record.UsageSegments)
	}
	wantOccurredAt := time.Date(2026, 7, 9, 3, 20, 46, 0, time.UTC)
	if !record.UsageSegments[0].OccurredAt.Equal(wantOccurredAt) {
		t.Fatalf("unexpected usage occurrence time: got %s want %s", record.UsageSegments[0].OccurredAt, wantOccurredAt)
	}
}

func TestParserAttributesIncrementalUsageAcrossModels(t *testing.T) {
	path := writeRolloutJSONL(t, `{"timestamp":"2026-07-09T03:20:44Z","type":"session_meta","payload":{"model_provider":"openai"}}
{"timestamp":"2026-07-09T03:20:45Z","type":"turn_context","payload":{"model":"gpt-5"}}
{"timestamp":"2026-07-09T03:20:46Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12},"last_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}}
{"timestamp":"2026-07-09T03:20:47Z","type":"turn_context","payload":{"model":"gpt-5-mini"}}
{"timestamp":"2026-07-09T03:20:48Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":30,"cached_input_tokens":5,"output_tokens":6,"total_tokens":36},"last_token_usage":{"input_tokens":20,"cached_input_tokens":5,"output_tokens":4,"total_tokens":24}}}}
`)
	record, err := NewParser().ParseSession(context.Background(), session.FileRef{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if len(record.UsageSegments) != 2 {
		t.Fatalf("unexpected segments: %+v", record.UsageSegments)
	}
	if record.UsageSegments[0].Model != "gpt-5" || record.UsageSegments[0].TokenUsage.TotalTokens != 12 {
		t.Fatalf("unexpected first segment: %+v", record.UsageSegments[0])
	}
	if record.UsageSegments[1].Model != "gpt-5-mini" || record.UsageSegments[1].TokenUsage.TotalTokens != 24 {
		t.Fatalf("unexpected second segment: %+v", record.UsageSegments[1])
	}
	if !record.UsageSegments[0].OccurredAt.Equal(time.Date(2026, 7, 9, 3, 20, 46, 0, time.UTC)) || !record.UsageSegments[1].OccurredAt.Equal(time.Date(2026, 7, 9, 3, 20, 48, 0, time.UTC)) {
		t.Fatalf("usage occurrence timestamps were not preserved: %+v", record.UsageSegments)
	}
	if record.TokenUsage.TotalTokens != 36 {
		t.Fatalf("unexpected aggregate: %+v", record.TokenUsage)
	}
}

func TestParserAttributesUsageToReroutedModel(t *testing.T) {
	path := writeRolloutJSONL(t, `{"type":"session_meta","payload":{"model_provider":"openai"}}
{"type":"turn_context","payload":{"model":"gpt-5.3-codex"}}
{"type":"event_msg","payload":{"type":"model_reroute","from_model":"gpt-5.3-codex","to_model":"gpt-5.2","reason":"high_risk_cyber_activity"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12},"last_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}}
`)
	record, err := NewParser().ParseSession(context.Background(), session.FileRef{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	wantModels := []string{"gpt-5.3-codex", "gpt-5.2"}
	if !slices.Equal(record.Models, wantModels) {
		t.Fatalf("unexpected models after reroute: got %+v want %+v", record.Models, wantModels)
	}
	if len(record.UsageSegments) != 1 || record.UsageSegments[0].Model != "gpt-5.2" {
		t.Fatalf("usage was not attributed to rerouted model: %+v", record.UsageSegments)
	}
}

func TestParserAttributesUsageToAppliedProviderModelAndServiceTier(t *testing.T) {
	path := writeRolloutJSONL(t, `{"type":"session_meta","payload":{"model_provider":"openai"}}
{"type":"turn_context","payload":{"model":"gpt-5.3-codex"}}
{"type":"event_msg","payload":{"type":"thread_settings_applied","thread_settings":{"model":"gpt-5.3-codex","model_provider_id":"openai","service_tier":"default"}}}
{"type":"event_msg","payload":{"type":"thread_settings_applied","thread_settings":{"model":"custom-model","model_provider_id":"custom-provider","service_tier":"flex"}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12},"last_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}}
{"type":"event_msg","payload":{"type":"thread_settings_applied","thread_settings":{"model":"custom-model","model_provider_id":"custom-provider"}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":30,"output_tokens":5,"total_tokens":35},"last_token_usage":{"input_tokens":20,"output_tokens":3,"total_tokens":23}}}}
`)
	record, err := NewParser().ParseSession(context.Background(), session.FileRef{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if record.Provider != "openai" {
		t.Fatalf("canonical provider changed: %q", record.Provider)
	}
	wantModels := []string{"gpt-5.3-codex", "custom-model"}
	if !slices.Equal(record.Models, wantModels) {
		t.Fatalf("unexpected models after settings change: got %+v want %+v", record.Models, wantModels)
	}
	if len(record.UsageSegments) != 2 {
		t.Fatalf("unexpected usage segments: %+v", record.UsageSegments)
	}
	first, second := record.UsageSegments[0], record.UsageSegments[1]
	if first.Provider != "custom-provider" || first.Model != "custom-model" || first.ServiceTier != "flex" {
		t.Fatalf("usage did not use active settings: %+v", first)
	}
	if second.Provider != "custom-provider" || second.Model != "custom-model" || second.ServiceTier != "" {
		t.Fatalf("full settings snapshot did not clear service tier: %+v", second)
	}
}

func TestParserPreservesRequestsUsingTheSameModel(t *testing.T) {
	path := writeRolloutJSONL(t, `{"type":"session_meta","payload":{"model_provider":"openai"}}
{"type":"turn_context","payload":{"model":"gpt-5"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10},"last_token_usage":{"input_tokens":10}}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":20},"last_token_usage":{"input_tokens":10}}}}
`)
	record, err := NewParser().ParseSession(context.Background(), session.FileRef{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if len(record.UsageSegments) != 2 || record.UsageSegments[0].TokenUsage.InputTokens != 10 || record.UsageSegments[1].TokenUsage.InputTokens != 10 {
		t.Fatalf("requests were not preserved: %+v", record.UsageSegments)
	}
	if !record.UsageSegments[0].OccurredAt.IsZero() || !record.UsageSegments[1].OccurredAt.IsZero() {
		t.Fatalf("timestamp-less usage should preserve zero occurrence times: %+v", record.UsageSegments)
	}
}

func TestParserDeduplicatesRepeatedTokenCountSnapshots(t *testing.T) {
	path := writeRolloutJSONL(t, `{"type":"session_meta","payload":{"model_provider":"openai"}}
{"type":"turn_context","payload":{"model":"gpt-5"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12},"last_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12},"last_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":30,"output_tokens":5,"total_tokens":35},"last_token_usage":{"input_tokens":20,"output_tokens":3,"total_tokens":23}}}}
`)
	record, err := NewParser().ParseSession(context.Background(), session.FileRef{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if len(record.UsageSegments) != 2 {
		t.Fatalf("repeated snapshot was priced as another request: %+v", record.UsageSegments)
	}
	if record.UsageSegments[1].TokenUsage != (session.TokenUsage{InputTokens: 20, OutputTokens: 3, TotalTokens: 23}) {
		t.Fatalf("second request was not derived from the cumulative delta: %+v", record.UsageSegments[1])
	}
}

func TestParserIgnoresTotalOnlyContextEstimateAndDeduplicatesItsRepeat(t *testing.T) {
	path := writeRolloutJSONL(t, `{"type":"session_meta","payload":{"model_provider":"openai"}}
{"type":"turn_context","payload":{"model":"gpt-5"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12},"last_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12},"last_token_usage":{"total_tokens":5003}}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12},"last_token_usage":{"total_tokens":5003}}}}
`)
	record, err := NewParser().ParseSession(context.Background(), session.FileRef{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if record.TokenUsage != (session.TokenUsage{InputTokens: 10, OutputTokens: 2, TotalTokens: 12}) {
		t.Fatalf("context estimate changed aggregate usage: %+v", record.TokenUsage)
	}
	if len(record.UsageSegments) != 1 || record.UsageSegments[0].TokenUsage != record.TokenUsage {
		t.Fatalf("context estimate became a priced segment: %+v", record.UsageSegments)
	}
}

func TestParserIgnoresFullContextTotalOnlySentinel(t *testing.T) {
	path := writeRolloutJSONL(t, `{"type":"session_meta","payload":{"model_provider":"openai"}}
{"type":"turn_context","payload":{"model":"gpt-5"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"cached_input_tokens":4,"output_tokens":2,"reasoning_output_tokens":1,"total_tokens":12},"last_token_usage":{"input_tokens":10,"cached_input_tokens":4,"output_tokens":2,"reasoning_output_tokens":1,"total_tokens":12}}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":100},"last_token_usage":{"total_tokens":88}}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":20,"cached_input_tokens":5,"output_tokens":4,"reasoning_output_tokens":1,"total_tokens":124},"last_token_usage":{"input_tokens":20,"cached_input_tokens":5,"output_tokens":4,"reasoning_output_tokens":1,"total_tokens":24}}}}
`)
	record, err := NewParser().ParseSession(context.Background(), session.FileRef{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	want := session.TokenUsage{InputTokens: 30, CachedInputTokens: 9, OutputTokens: 6, ReasoningOutputTokens: 2, TotalTokens: 36}
	if record.TokenUsage != want {
		t.Fatalf("unexpected billable aggregate across sentinel: got %+v want %+v", record.TokenUsage, want)
	}
	if len(record.UsageSegments) != 2 {
		t.Fatalf("full-context sentinel became a priced segment: %+v", record.UsageSegments)
	}
}

func TestParserRejectsTokenUsageDeltaMismatch(t *testing.T) {
	path := writeRolloutJSONL(t, `{"type":"session_meta","payload":{"model_provider":"openai"}}
{"type":"turn_context","payload":{"model":"gpt-5"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10},"last_token_usage":{"input_tokens":9}}}}
`)
	_, err := NewParser().ParseSession(context.Background(), session.FileRef{Path: path})
	if err == nil || !strings.Contains(err.Error(), "delta does not match") {
		t.Fatalf("expected cumulative delta mismatch, got %v", err)
	}
}

func TestParserIgnoresContextEstimateWithoutBillableBreakdown(t *testing.T) {
	path := writeRolloutJSONL(t, `{"type":"session_meta","payload":{"model_provider":"openai"}}
{"type":"turn_context","payload":{"model":"gpt-5"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"total_tokens":5003}}}}
`)
	record, err := NewParser().ParseSession(context.Background(), session.FileRef{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if record.TokenUsage != (session.TokenUsage{}) || len(record.UsageSegments) != 0 {
		t.Fatalf("context estimate was counted as usage: aggregate=%+v segments=%+v", record.TokenUsage, record.UsageSegments)
	}
}

func TestParserCurrentForkShapeKeepsChildMetadataAndChildOnlyUsage(t *testing.T) {
	const (
		parentID = "019f587e-f771-7f43-97db-74bb1b781ce8"
		childID  = "019f587f-e759-7e51-9ac9-5ecba7ac3cb6"
	)
	path := writeRolloutJSONL(t, `{"timestamp":"2026-07-12T22:43:25.244Z","type":"session_meta","payload":{"id":"`+childID+`","session_id":"`+parentID+`","forked_from_id":"`+parentID+`","cwd":"/child","cli_version":"0.144.1","model_provider":"openai"}}
{"timestamp":"2026-07-12T22:43:25.244Z","type":"session_meta","payload":{"id":"`+parentID+`","session_id":"`+parentID+`","cwd":"/parent","cli_version":"0.143.0","model_provider":"openai"}}
{"type":"event_msg","payload":{"type":"task_started","turn_id":"019f587f-a0ed-7dd3-bdb4-b8ef06cc896c"}}
{"type":"turn_context","payload":{"model":"gpt-parent"}}
{"type":"event_msg","payload":{"type":"thread_settings_applied","thread_settings":{"model":"gpt-parent","model_provider_id":"parent-provider","service_tier":"priority"}}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12},"last_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}}
{"type":"inter_agent_communication_metadata","payload":{"trigger_turn":true}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":30,"output_tokens":5,"total_tokens":35},"last_token_usage":{"input_tokens":20,"output_tokens":3,"total_tokens":23}}}}
{"type":"event_msg","payload":{"type":"task_started","turn_id":"019f587f-e7c0-7873-914d-865a3bbb115b"}}
{"type":"turn_context","payload":{"model":"gpt-child"}}
{"type":"inter_agent_communication_metadata","payload":{"trigger_turn":true}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":50,"cached_input_tokens":4,"output_tokens":9,"reasoning_output_tokens":1,"total_tokens":59},"last_token_usage":{"input_tokens":20,"cached_input_tokens":4,"output_tokens":4,"reasoning_output_tokens":1,"total_tokens":24}}}}
`)
	record, err := NewParser().ParseSession(context.Background(), session.FileRef{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if record.SessionID != childID || record.CWD != "/child" || record.CLIVersion != "0.144.1" || record.Provider != "openai" {
		t.Fatalf("embedded parent metadata replaced child metadata: %+v", record)
	}
	want := session.TokenUsage{InputTokens: 20, CachedInputTokens: 4, OutputTokens: 4, ReasoningOutputTokens: 1, TotalTokens: 24}
	if record.TokenUsage != want {
		t.Fatalf("copied parent usage was counted: got %+v want %+v", record.TokenUsage, want)
	}
	if len(record.UsageSegments) != 1 || record.UsageSegments[0].Provider != "openai" || record.UsageSegments[0].Model != "gpt-child" || record.UsageSegments[0].ServiceTier != "" || record.UsageSegments[0].TokenUsage != want {
		t.Fatalf("unexpected child usage segments: %+v", record.UsageSegments)
	}
}

func TestParserGenericForkUsesUUIDv7ChildTurnBoundary(t *testing.T) {
	path := writeRolloutJSONL(t, `{"type":"session_meta","payload":{"id":"019f587f-e759-7e51-9ac9-5ecba7ac3cb6","session_id":"019f587e-f771-7f43-97db-74bb1b781ce8","forked_from_id":"019f587e-f771-7f43-97db-74bb1b781ce8","model_provider":"openai"}}
{"type":"session_meta","payload":{"id":"019f587e-f771-7f43-97db-74bb1b781ce8","model_provider":"openai"}}
{"type":"turn_context","payload":{"model":"gpt-parent"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12},"last_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}}
{"type":"event_msg","payload":{"type":"turn_started","turn_id":"019f587f-e7c0-7873-914d-865a3bbb115b"}}
{"type":"turn_context","payload":{"model":"gpt-child"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":30,"output_tokens":5,"total_tokens":35},"last_token_usage":{"input_tokens":20,"output_tokens":3,"total_tokens":23}}}}
`)
	record, err := NewParser().ParseSession(context.Background(), session.FileRef{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	want := session.TokenUsage{InputTokens: 20, OutputTokens: 3, TotalTokens: 23}
	if record.TokenUsage != want || len(record.UsageSegments) != 1 || record.UsageSegments[0].Model != "gpt-child" {
		t.Fatalf("generic fork retained copied usage: aggregate=%+v segments=%+v", record.TokenUsage, record.UsageSegments)
	}
}

func TestParserForkEndingBeforeChildTurnReportsZeroUsage(t *testing.T) {
	const childID = "019f587f-e759-7e51-9ac9-5ecba7ac3cb6"
	path := writeRolloutJSONL(t, `{"type":"session_meta","payload":{"id":"`+childID+`","session_id":"019f587e-f771-7f43-97db-74bb1b781ce8","forked_from_id":"019f587e-f771-7f43-97db-74bb1b781ce8","model_provider":"openai"}}
{"type":"session_meta","payload":{"id":"019f587e-f771-7f43-97db-74bb1b781ce8","model_provider":"openai"}}
{"type":"turn_context","payload":{"model":"gpt-parent"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12},"last_token_usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}}
`)
	record, err := NewParser().ParseSession(context.Background(), session.FileRef{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if record.SessionID != childID {
		t.Fatalf("unexpected child session ID: %q", record.SessionID)
	}
	if record.TokenUsage != (session.TokenUsage{}) || len(record.UsageSegments) != 0 {
		t.Fatalf("EOF fork retained copied parent usage: aggregate=%+v segments=%+v", record.TokenUsage, record.UsageSegments)
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

func TestParserReportsMalformedJSONLLine(t *testing.T) {
	path := writeRolloutJSONL(t, "not json\n")
	_, err := NewParser().ParseSession(context.Background(), session.FileRef{Path: path})
	if err == nil || !strings.Contains(err.Error(), "parse rollout line 1:") {
		t.Fatalf("expected line-specific parse error, got %v", err)
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
{"timestamp":"2026-07-09T03:20:47Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Explain this session"}]}}
{"timestamp":"2026-07-09T03:20:48Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[]}}
{"timestamp":"2026-07-09T03:20:49Z","type":"response_item","payload":{"type":"function_call","name":"exec_command"}}
{"timestamp":"2026-07-09T03:20:50Z","type":"response_item","payload":{"type":"custom_tool_call","name":"apply_patch"}}
`
}
