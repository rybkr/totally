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
	activeModel       string
	previousTotal     codexTokenUsage
	previousLast      codexTokenUsage
	hasTokenUsagePair bool
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
		return applySessionMeta(envelope.Payload, record)
	case "turn_context":
		record.Turns++
		return applyTurnContext(envelope.Payload, record, state)
	case "event_msg":
		return applyEventMsg(envelope.Payload, record, state)
	case "response_item":
		return applyResponseItem(envelope.Payload, record)
	default:
		return nil
	}
}

func applySessionMeta(payload json.RawMessage, record *session.Record) error {
	var meta struct {
		SessionID     string `json:"session_id"`
		ID            string `json:"id"`
		CWD           string `json:"cwd"`
		CLIVersion    string `json:"cli_version"`
		ModelProvider string `json:"model_provider"`
	}
	if err := json.Unmarshal(payload, &meta); err != nil {
		return err
	}

	if meta.SessionID != "" {
		record.SessionID = meta.SessionID
	} else if meta.ID != "" {
		record.SessionID = meta.ID
	}
	record.CWD = meta.CWD
	record.CLIVersion = meta.CLIVersion
	record.Provider = meta.ModelProvider
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

func applyEventMsg(payload json.RawMessage, record *session.Record, state *parseState) error {
	var event struct {
		Type string `json:"type"`
		Info *struct {
			TotalTokenUsage codexTokenUsage `json:"total_token_usage"`
			LastTokenUsage  codexTokenUsage `json:"last_token_usage"`
		} `json:"info"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return err
	}

	switch event.Type {
	case "token_count":
		if event.Info != nil {
			currentTotal := event.Info.TotalTokenUsage
			currentLast := event.Info.LastTokenUsage
			record.TokenUsage = currentTotal.toSessionUsage()

			// Codex can repeat its latest counter snapshot when a session is
			// resumed. The repeated last_token_usage is not another request.
			// Compare the complete pair so two real requests with coincidentally
			// identical incremental usage are still retained when the cumulative
			// total advances.
			if state.hasTokenUsagePair && currentTotal == state.previousTotal && currentLast == state.previousLast {
				return nil
			}

			if currentTotal != state.previousTotal {
				delta, ok := currentTotal.subtract(state.previousTotal)
				if !ok {
					return fmt.Errorf("cumulative token usage regressed")
				}
				if delta != currentLast {
					return fmt.Errorf("cumulative token usage delta does not match last_token_usage")
				}
				addUsageSegment(record, record.Provider, state.activeModel, delta.toSessionUsage())
			} else if currentLast.hasBillableBreakdown() {
				return fmt.Errorf("last_token_usage changed without cumulative token usage advancing")
			} else {
				// Compaction events can report only total_tokens and do not advance
				// Codex's cumulative billable input/output counters.
				addUsageSegment(record, record.Provider, state.activeModel, currentLast.toSessionUsage())
			}

			state.previousTotal = currentTotal
			state.previousLast = currentLast
			state.hasTokenUsagePair = true
		}
	}
	return nil
}

func addUsageSegment(record *session.Record, provider, model string, usage session.TokenUsage) {
	if model == "" || usage == (session.TokenUsage{}) {
		return
	}
	record.UsageSegments = append(record.UsageSegments, session.UsageSegment{Provider: provider, Model: model, TokenUsage: usage})
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

func (usage codexTokenUsage) toSessionUsage() session.TokenUsage {
	return session.TokenUsage{
		InputTokens:           usage.InputTokens,
		CachedInputTokens:     usage.CachedInputTokens,
		OutputTokens:          usage.OutputTokens,
		ReasoningOutputTokens: usage.ReasoningOutputTokens,
		TotalTokens:           usage.TotalTokens,
	}
}
