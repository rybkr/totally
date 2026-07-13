package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/rybkr/totally/internal/session"
)

// Parser reads Codex rollout transcript files.
type Parser struct{}

type parseState struct {
	activeModel           string
	activeProvider        string
	activeServiceTier     string
	previousTotal         codexTokenUsage
	previousLast          codexTokenUsage
	hasTokenUsagePair     bool
	sawSessionMeta        bool
	hasEmbeddedHistory    bool
	clearedInheritedUsage bool
	canonicalSessionID    string
	canonicalProvider     string
}

func NewParser() Parser {
	return Parser{}
}

func (Parser) Source() session.Source {
	return Source
}

func (Parser) ParseSession(ctx context.Context, file session.FileRef) (session.Record, error) {
	if file.Source != "" && file.Source != Source {
		return session.Record{}, fmt.Errorf("unsupported source %q", file.Source)
	}

	reader, closeReader, err := openRollout(file)
	if err != nil {
		return session.Record{}, err
	}
	defer closeReader()

	record := session.Record{
		Source:    Source,
		SessionID: file.SessionID,
		Path:      file.Path,
		Format:    file.Format,
		CreatedAt: file.CreatedAt,
		UpdatedAt: file.UpdatedAt,
		SizeBytes: file.SizeBytes,
	}

	lineReader := bufio.NewReader(reader)
	sawTimestamp := false
	lineNumber := 0
	state := parseState{}

	for {
		line, err := lineReader.ReadBytes('\n')
		lineNumber++
		if len(bytes.TrimSpace(line)) == 0 && err == io.EOF {
			break
		}
		if err != nil && err != io.EOF {
			return session.Record{}, fmt.Errorf("read rollout line %d: %w", lineNumber, err)
		}
		if err := ctx.Err(); err != nil {
			return session.Record{}, err
		}
		if len(bytes.TrimSpace(line)) > 0 {
			if err := applyRolloutLine(line, &record, &state, &sawTimestamp); err != nil {
				return session.Record{}, fmt.Errorf("parse rollout line %d: %w", lineNumber, err)
			}
		}
		if err == io.EOF {
			break
		}
	}

	if state.hasEmbeddedHistory && !state.clearedInheritedUsage {
		clearInheritedUsage(&record, &state)
	}
	return record, nil
}

func openRollout(file session.FileRef) (io.Reader, func(), error) {
	f, err := os.Open(file.Path)
	if err != nil {
		return nil, func() {}, err
	}

	switch file.Format {
	case "", session.FileFormatJSONL:
		return f, func() { _ = f.Close() }, nil
	case session.FileFormatJSONLZstd:
		decoder, err := zstd.NewReader(f)
		if err != nil {
			_ = f.Close()
			return nil, func() {}, err
		}
		return decoder, func() {
			decoder.Close()
			_ = f.Close()
		}, nil
	default:
		_ = f.Close()
		return nil, func() {}, fmt.Errorf("unsupported Codex rollout format %q", file.Format)
	}
}

func applyRolloutLine(line []byte, record *session.Record, state *parseState, sawTimestamp *bool) error {
	var envelope struct {
		Timestamp time.Time       `json:"timestamp"`
		Type      string          `json:"type"`
		Payload   json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return err
	}
	if !envelope.Timestamp.IsZero() {
		if !*sawTimestamp {
			record.CreatedAt = envelope.Timestamp
		}
		record.UpdatedAt = envelope.Timestamp
		*sawTimestamp = true
	}

	switch envelope.Type {
	case "session_meta":
		return applySessionMeta(envelope.Payload, record, state)
	case "turn_context":
		record.Turns++
		return applyTurnContext(envelope.Payload, record, state)
	case "event_msg":
		return applyEventMsg(envelope.Payload, envelope.Timestamp, record, state)
	case "response_item":
		return applyResponseItem(envelope.Payload, record)
	default:
		return nil
	}
}

func applySessionMeta(payload json.RawMessage, record *session.Record, state *parseState) error {
	var meta struct {
		SessionID     string `json:"session_id"`
		ID            string `json:"id"`
		ForkedFromID  string `json:"forked_from_id"`
		CWD           string `json:"cwd"`
		CLIVersion    string `json:"cli_version"`
		ModelProvider string `json:"model_provider"`
	}
	if err := json.Unmarshal(payload, &meta); err != nil {
		return err
	}

	// Forked rollouts copy their parent's session_meta after the canonical child
	// metadata. The first record owns the file, and current Codex identifies the
	// child in id while session_id still points at the parent.
	if state.sawSessionMeta {
		if meta.ID != "" && meta.ID != record.SessionID {
			state.hasEmbeddedHistory = true
		}
		return nil
	}
	state.sawSessionMeta = true
	state.hasEmbeddedHistory = meta.ForkedFromID != "" || (meta.ID != "" && meta.SessionID != "" && meta.ID != meta.SessionID)

	if meta.ID != "" {
		record.SessionID = meta.ID
	} else if meta.SessionID != "" {
		record.SessionID = meta.SessionID
	}
	state.canonicalSessionID = record.SessionID
	record.CWD = meta.CWD
	record.CLIVersion = meta.CLIVersion
	record.Provider = meta.ModelProvider
	state.canonicalProvider = meta.ModelProvider
	state.activeProvider = meta.ModelProvider
	return nil
}

func applyTurnContext(payload json.RawMessage, record *session.Record, state *parseState) error {
	var turn struct {
		Model string `json:"model"`
		CWD   string `json:"cwd"`
	}
	if err := json.Unmarshal(payload, &turn); err != nil {
		return err
	}

	addModel(record, turn.Model)
	if turn.Model != "" {
		state.activeModel = turn.Model
	}
	if record.CWD == "" {
		record.CWD = turn.CWD
	}
	return nil
}

func addModel(record *session.Record, model string) {
	if model == "" {
		return
	}
	for _, existing := range record.Models {
		if existing == model {
			return
		}
	}
	record.Models = append(record.Models, model)
}

func applyEventMsg(payload json.RawMessage, occurredAt time.Time, record *session.Record, state *parseState) error {
	var event struct {
		Type           string `json:"type"`
		TurnID         string `json:"turn_id"`
		FromModel      string `json:"from_model"`
		ToModel        string `json:"to_model"`
		ThreadSettings *struct {
			Model           string `json:"model"`
			ModelProviderID string `json:"model_provider_id"`
			ServiceTier     string `json:"service_tier"`
		} `json:"thread_settings"`
		Info *struct {
			TotalTokenUsage codexTokenUsage `json:"total_token_usage"`
			LastTokenUsage  codexTokenUsage `json:"last_token_usage"`
		} `json:"info"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return err
	}

	switch event.Type {
	case "task_started", "turn_started":
		// Generic forks do not necessarily carry an inter-agent marker. Codex
		// UUIDv7 thread and turn IDs are timestamp ordered, so the first valid
		// turn at or after the child thread ID marks the end of copied history.
		if state.hasEmbeddedHistory && isUUIDv7(state.canonicalSessionID) && isUUIDv7(event.TurnID) && strings.ToLower(event.TurnID) >= strings.ToLower(state.canonicalSessionID) {
			clearInheritedUsage(record, state)
		}
	case "model_reroute":
		if event.ToModel != "" {
			addModel(record, event.ToModel)
			state.activeModel = event.ToModel
		}
	case "thread_settings_applied":
		if event.ThreadSettings != nil {
			if event.ThreadSettings.Model != "" {
				addModel(record, event.ThreadSettings.Model)
				state.activeModel = event.ThreadSettings.Model
			}
			if event.ThreadSettings.ModelProviderID != "" {
				state.activeProvider = event.ThreadSettings.ModelProviderID
			}
			state.activeServiceTier = event.ThreadSettings.ServiceTier
		}
	case "token_count":
		if event.Info != nil {
			currentTotal := event.Info.TotalTokenUsage
			currentLast := event.Info.LastTokenUsage

			// Codex can repeat its latest counter snapshot when a session is
			// resumed. The repeated last_token_usage is not another request.
			// Compare the complete pair so two real requests with coincidentally
			// identical incremental usage are still retained when the cumulative
			// total advances.
			if state.hasTokenUsagePair && currentTotal == state.previousTotal && currentLast == state.previousLast {
				return nil
			}

			if currentTotal != state.previousTotal {
				// Codex can replace its cumulative counters with a total-only local
				// context-window sentinel. It is not provider usage, and resetting
				// the billable fields makes it look like a cumulative regression.
				if currentTotal.isTotalOnly() && !currentLast.hasBillableBreakdown() {
					expectedDelta := currentTotal.TotalTokens - state.previousTotal.TotalTokens
					if expectedDelta < 0 {
						expectedDelta = 0
					}
					if currentLast.TotalTokens != expectedDelta {
						return fmt.Errorf("cumulative token usage delta does not match last_token_usage")
					}
					state.previousTotal = currentTotal
					state.previousLast = currentLast
					state.hasTokenUsagePair = true
					return nil
				}
				delta, ok := currentTotal.subtract(state.previousTotal)
				if !ok {
					return fmt.Errorf("cumulative token usage regressed")
				}
				if delta != currentLast {
					return fmt.Errorf("cumulative token usage delta does not match last_token_usage")
				}
				provider := state.activeProvider
				if provider == "" {
					provider = record.Provider
				}
				addAcceptedUsage(record, provider, state.activeModel, state.activeServiceTier, occurredAt, currentLast)
			} else if currentLast.hasBillableBreakdown() {
				return fmt.Errorf("last_token_usage changed without cumulative token usage advancing")
			}
			// A total-only last_token_usage is Codex's local estimate of the
			// active context after compaction. It is neither API usage nor billable
			// telemetry, so an unchanged cumulative total contributes nothing.

			state.previousTotal = currentTotal
			state.previousLast = currentLast
			state.hasTokenUsagePair = true
		}
	}
	return nil
}

func clearInheritedUsage(record *session.Record, state *parseState) {
	if !state.hasEmbeddedHistory || state.clearedInheritedUsage {
		return
	}
	record.TokenUsage = session.TokenUsage{}
	record.UsageSegments = nil
	state.activeProvider = state.canonicalProvider
	state.activeServiceTier = ""
	state.clearedInheritedUsage = true
}

func isUUIDv7(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' || value[14] != '7' {
		return false
	}
	for i, r := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func addAcceptedUsage(record *session.Record, provider, model, serviceTier string, occurredAt time.Time, usage codexTokenUsage) {
	if !usage.hasBillableBreakdown() {
		return
	}
	normalized := usage.toSessionUsage()
	record.TokenUsage.InputTokens += normalized.InputTokens
	record.TokenUsage.CachedInputTokens += normalized.CachedInputTokens
	record.TokenUsage.OutputTokens += normalized.OutputTokens
	record.TokenUsage.ReasoningOutputTokens += normalized.ReasoningOutputTokens
	record.TokenUsage.TotalTokens += normalized.TotalTokens
	addUsageSegment(record, provider, model, serviceTier, occurredAt, normalized)
}

func addUsageSegment(record *session.Record, provider, model, serviceTier string, occurredAt time.Time, usage session.TokenUsage) {
	if model == "" || usage == (session.TokenUsage{}) {
		return
	}
	record.UsageSegments = append(record.UsageSegments, session.UsageSegment{Provider: provider, Model: model, ServiceTier: serviceTier, OccurredAt: occurredAt, TokenUsage: usage})
}

func applyResponseItem(payload json.RawMessage, record *session.Record) error {
	var item struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(payload, &item); err != nil {
		return err
	}

	switch item.Type {
	case "message":
		record.Messages++
		if item.Role == "user" && record.FirstPrompt == "" {
			if prompt := firstPromptText(item.Content); prompt != "" {
				record.FirstPrompt = prompt
			}
		}
	case "function_call", "custom_tool_call":
		record.ToolCalls++
	}
	return nil
}

func firstPromptText(content []struct {
	Type string `json:"type"`
	Text string `json:"text"`
}) string {
	var parts []string
	for _, part := range content {
		text := strings.TrimSpace(part.Text)
		if part.Type != "input_text" || text == "" {
			continue
		}
		if strings.HasPrefix(text, "# AGENTS.md instructions") || strings.HasPrefix(text, "<environment_context>") {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n")
}

type codexTokenUsage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
	TotalTokens           int64 `json:"total_tokens"`
}

func (usage codexTokenUsage) subtract(previous codexTokenUsage) (codexTokenUsage, bool) {
	if usage.InputTokens < previous.InputTokens ||
		usage.CachedInputTokens < previous.CachedInputTokens ||
		usage.OutputTokens < previous.OutputTokens ||
		usage.ReasoningOutputTokens < previous.ReasoningOutputTokens ||
		usage.TotalTokens < previous.TotalTokens {
		return codexTokenUsage{}, false
	}
	return codexTokenUsage{
		InputTokens:           usage.InputTokens - previous.InputTokens,
		CachedInputTokens:     usage.CachedInputTokens - previous.CachedInputTokens,
		OutputTokens:          usage.OutputTokens - previous.OutputTokens,
		ReasoningOutputTokens: usage.ReasoningOutputTokens - previous.ReasoningOutputTokens,
		TotalTokens:           usage.TotalTokens - previous.TotalTokens,
	}, true
}

func (usage codexTokenUsage) hasBillableBreakdown() bool {
	return usage.InputTokens != 0 || usage.CachedInputTokens != 0 || usage.OutputTokens != 0 || usage.ReasoningOutputTokens != 0
}

func (usage codexTokenUsage) isTotalOnly() bool {
	return usage.TotalTokens != 0 && !usage.hasBillableBreakdown()
}

func (usage codexTokenUsage) toSessionUsage() session.TokenUsage {
	return session.TokenUsage{
		InputTokens:           usage.InputTokens,
		CachedInputTokens:     usage.CachedInputTokens,
		OutputTokens:          usage.OutputTokens,
		ReasoningOutputTokens: usage.ReasoningOutputTokens,
		TotalTokens:           usage.TotalTokens,
	}
}
