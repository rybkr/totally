package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rybkr/totally/internal/pricing"
	"github.com/rybkr/totally/internal/session"
	"github.com/spf13/cobra"
)

var sessionIDPrefixPattern = regexp.MustCompile(`^[0-9a-fA-F-]+$`)

type inspectSummary struct {
	Sessions int      `json:"sessions"`
	Sources  []string `json:"sources"`

	CreatedStart string `json:"created_start"`
	CreatedEnd   string `json:"created_end"`
	UpdatedStart string `json:"updated_start"`
	UpdatedEnd   string `json:"updated_end"`

	CWDs        []string `json:"cwds"`
	Models      []string `json:"models"`
	Providers   []string `json:"providers"`
	CLIVersions []string `json:"cli_versions"`

	Turns     int `json:"turns"`
	Messages  int `json:"messages"`
	ToolCalls int `json:"tool_calls"`

	TokenUsage session.TokenUsage `json:"token_usage"`
}

type showOptions struct {
	latest bool
	full   bool
}

func newShowCommand(stdout io.Writer, globals *globalOptions) *cobra.Command {
	var opts showOptions

	cmd := &cobra.Command{
		Use:   "show <session-id> | --latest",
		Short: "Show a detailed, single-session report",
		Args: func(cmd *cobra.Command, args []string) error {
			if opts.latest {
				if len(args) != 0 {
					return fmt.Errorf("--latest does not accept a session ID")
				}
				return nil
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShow(cmd, stdout, *globals, opts, args)
		},
	}
	cmd.Flags().BoolVar(&opts.latest, "latest", false, "show the most recently updated session")
	cmd.Flags().BoolVar(&opts.full, "full", false, "do not truncate display values in table output")

	return cmd
}

func runShow(cmd *cobra.Command, stdout io.Writer, globals globalOptions, opts showOptions, args []string) error {
	parsers, err := globals.parsers()
	if err != nil {
		return err
	}

	var record session.Record
	if opts.latest {
		files, err := discoverSessionFiles(cmd, globals)
		if err != nil {
			return err
		}
		records, err := parseSessionFilesWithParsers(cmd, parsers, files)
		if err != nil {
			return err
		}
		if len(records) == 0 {
			return fmt.Errorf("no sessions found")
		}
		sortRecordsByUpdated(records)
		record = records[0]
	} else {
		file, err := resolveShowSessionID(cmd, globals, args[0])
		if err != nil {
			return err
		}

		parser, err := parserForSource(parsers, file.Source)
		if err != nil {
			return err
		}

		record, err = parser.ParseSession(cmd.Context(), file)
		if err != nil {
			return err
		}
	}

	report := newShowReport(record, globals.prices)
	switch globals.format {
	case outputFormatTable:
		return printShowReport(stdout, report, opts.full)
	case outputFormatJSON:
		return json.NewEncoder(stdout).Encode(report)
	default:
		return fmt.Errorf("unknown format %q", globals.format)
	}
}

func parseDiscoveredSessions(cmd *cobra.Command, globals globalOptions, parsers []session.Parser) ([]session.Record, error) {
	files, err := discoverSessionFiles(cmd, globals)
	if err != nil {
		return nil, err
	}
	records, err := parseSessionFilesWithParsers(cmd, parsers, files)
	if err != nil {
		return nil, err
	}
	sortRecordsByCreated(records)
	return records, nil
}

func parserForSource(parsers []session.Parser, source session.Source) (session.Parser, error) {
	for _, parser := range parsers {
		if parser.Source() == source {
			return parser, nil
		}
	}
	return nil, fmt.Errorf("no parser registered for source %q", source)
}

func resolveShowSessionID(cmd *cobra.Command, globals globalOptions, target string) (session.FileRef, error) {
	if !sessionIDPrefixPattern.MatchString(target) {
		return session.FileRef{}, fmt.Errorf("malformed session ID %q: expected a UUID prefix", target)
	}

	finders, err := globals.finders()
	if err != nil {
		return session.FileRef{}, err
	}

	var matches []session.FileRef
	for _, finder := range finders {
		files, err := finder.FindSessionFiles(cmd.Context(), session.FindOptions{
			Roots:           globals.homes,
			IncludeArchived: globals.archived,
		})
		if err != nil {
			return session.FileRef{}, err
		}
		for _, file := range files {
			if strings.HasPrefix(strings.ToLower(file.SessionID), strings.ToLower(target)) {
				matches = append(matches, file)
			}
		}
	}

	switch len(matches) {
	case 0:
		return session.FileRef{}, fmt.Errorf("no session found for %q", target)
	case 1:
		return matches[0], nil
	default:
		return session.FileRef{}, fmt.Errorf("multiple sessions found for UUID prefix %q; provide a longer prefix or pass --agent or --home to narrow the search", target)
	}
}

type showReport struct {
	SessionID       string               `json:"session_id"`
	Source          string               `json:"source"`
	CreatedAt       *string              `json:"created_at"`
	UpdatedAt       *string              `json:"updated_at"`
	DurationSeconds *int64               `json:"duration_seconds"`
	Project         *string              `json:"project"`
	Provider        *string              `json:"provider"`
	FirstPrompt     *string              `json:"first_prompt"`
	Path            string               `json:"path"`
	Models          []string             `json:"models"`
	Turns           int                  `json:"turns"`
	Messages        int                  `json:"messages"`
	ToolCalls       int                  `json:"tool_calls"`
	TokenUsage      showTokenUsageReport `json:"token_usage"`
	CostUSD         float64              `json:"cost_usd"`
	Cost            pricing.Estimate     `json:"cost"`
}

type showTokenUsageReport struct {
	InputTokens       int64 `json:"input_tokens"`
	CachedInputTokens int64 `json:"cached_input_tokens"`
	OutputTokens      int64 `json:"output_tokens"`
	ReasoningTokens   int64 `json:"reasoning_output_tokens"`
	TotalTokens       int64 `json:"total_tokens"`
}

func newShowReport(record session.Record, catalog pricing.Catalog) showReport {
	estimate := catalog.Estimate(record.UsageSegments, record.CreatedAt)
	report := showReport{
		SessionID: record.SessionID,
		Source:    string(record.Source),
		Path:      record.Path,
		Models:    record.Models,
		Turns:     record.Turns,
		Messages:  record.Messages,
		ToolCalls: record.ToolCalls,
		Cost:      estimate,
		CostUSD:   pricing.FloatAmount(estimate),
		TokenUsage: showTokenUsageReport{
			InputTokens:       record.TokenUsage.InputTokens,
			CachedInputTokens: record.TokenUsage.CachedInputTokens,
			OutputTokens:      record.TokenUsage.OutputTokens,
			ReasoningTokens:   record.TokenUsage.ReasoningOutputTokens,
			TotalTokens:       record.TokenUsage.TotalTokens,
		},
	}
	if !record.CreatedAt.IsZero() {
		created := formatTime(record.CreatedAt)
		report.CreatedAt = &created
	}
	if !record.UpdatedAt.IsZero() {
		updated := formatTime(record.UpdatedAt)
		report.UpdatedAt = &updated
	}
	if !record.CreatedAt.IsZero() && !record.UpdatedAt.IsZero() && !record.UpdatedAt.Before(record.CreatedAt) {
		duration := int64(record.UpdatedAt.Sub(record.CreatedAt).Seconds())
		report.DurationSeconds = &duration
	}
	if record.CWD != "" {
		report.Project = &record.CWD
	}
	if record.Provider != "" {
		provider := record.Provider
		report.Provider = &provider
	}
	if record.FirstPrompt != "" {
		prompt := record.FirstPrompt
		report.FirstPrompt = &prompt
	}
	return report
}

func summarizeRecords(records []session.Record) inspectSummary {
	var summary inspectSummary
	for _, record := range records {
		summary.Sessions++
		addUnique(&summary.Sources, string(record.Source))
		addUnique(&summary.CWDs, record.CWD)
		addUnique(&summary.Providers, record.Provider)
		addUnique(&summary.CLIVersions, record.CLIVersion)
		for _, model := range record.Models {
			addUnique(&summary.Models, model)
		}

		summary.CreatedStart = earliestFormattedTime(summary.CreatedStart, record.CreatedAt)
		summary.CreatedEnd = latestFormattedTime(summary.CreatedEnd, record.CreatedAt)
		summary.UpdatedStart = earliestFormattedTime(summary.UpdatedStart, record.UpdatedAt)
		summary.UpdatedEnd = latestFormattedTime(summary.UpdatedEnd, record.UpdatedAt)

		summary.Turns += record.Turns
		summary.Messages += record.Messages
		summary.ToolCalls += record.ToolCalls
		summary.TokenUsage.InputTokens += record.TokenUsage.InputTokens
		summary.TokenUsage.CachedInputTokens += record.TokenUsage.CachedInputTokens
		summary.TokenUsage.OutputTokens += record.TokenUsage.OutputTokens
		summary.TokenUsage.ReasoningOutputTokens += record.TokenUsage.ReasoningOutputTokens
		summary.TokenUsage.TotalTokens += record.TokenUsage.TotalTokens
	}
	return summary
}

func printInspectSummary(w io.Writer, summary inspectSummary) error {
	lines := []struct {
		label string
		value string
	}{
		{"Sessions", formatCount(summary.Sessions)},
		{"Sources", strings.Join(summary.Sources, ", ")},
		{"Created", formatRange(summary.CreatedStart, summary.CreatedEnd)},
		{"Updated", formatRange(summary.UpdatedStart, summary.UpdatedEnd)},
		{"CWDs", strings.Join(summary.CWDs, ", ")},
		{"Models", strings.Join(summary.Models, ", ")},
		{"Providers", strings.Join(summary.Providers, ", ")},
		{"CLIs", strings.Join(summary.CLIVersions, ", ")},
		{"Turns", formatCount(summary.Turns)},
		{"Messages", formatCount(summary.Messages)},
		{"Tool calls", formatCount(summary.ToolCalls)},
	}

	for _, line := range lines {
		if _, err := fmt.Fprintf(w, "%-11s %s\n", line.label+":", fallback(line.value)); err != nil {
			return err
		}
	}

	return printTokenUsage(w, summary.TokenUsage)
}

func printShowReport(w io.Writer, report showReport, full bool) error {
	lines := []struct {
		label string
		value string
	}{
		{"Session", report.SessionID},
		{"Source", report.Source},
	}
	if models := strings.Join(report.Models, ", "); models != "" {
		lines = append(lines, struct{ label, value string }{"Models", models})
	}
	if project := stringPtrValue(report.Project); project != "" {
		lines = append(lines, struct{ label, value string }{"Project", project})
	}
	if provider := stringPtrValue(report.Provider); provider != "" {
		lines = append(lines, struct{ label, value string }{"Provider", provider})
	}
	if prompt := stringPtrValue(report.FirstPrompt); prompt != "" {
		if !full {
			prompt = formatSessionPrompt(prompt)
		}
		lines = append(lines, struct{ label, value string }{"Prompt", prompt})
	}
	lines = append(lines,
		struct{ label, value string }{"Time", formatShowTime(report)},
		struct{ label, value string }{"Activity", fmt.Sprintf("%s turns, %s messages, %s tool calls", formatNumber(int64(report.Turns)), formatNumber(int64(report.Messages)), formatNumber(int64(report.ToolCalls)))},
		struct{ label, value string }{"Tokens", formatShowTokenUsage(report.TokenUsage)},
		struct{ label, value string }{"Cost", formatCostEstimate(report.Cost)},
		struct{ label, value string }{"Transcript", report.Path},
	)

	for _, line := range lines {
		if _, err := fmt.Fprintf(w, "%-11s %s\n", line.label, fallback(line.value)); err != nil {
			return err
		}
	}
	return nil
}

func formatCostEstimate(cost pricing.Estimate) string {
	if cost.AmountUSD == nil {
		if len(cost.Missing) > 0 {
			return fmt.Sprintf("unavailable (no price for %s/%s)", cost.Missing[0].Provider, cost.Missing[0].Model)
		}
		return "unavailable (token usage is not attributed to a model)"
	}
	value := "$" + *cost.AmountUSD + " USD estimated (API-equivalent)"
	if cost.Status == "partial" {
		value += "; partial"
	}
	return value
}

func formatShowTime(report showReport) string {
	created := stringPtrValue(report.CreatedAt)
	updated := stringPtrValue(report.UpdatedAt)
	if created == "" {
		return updated
	}
	if updated == "" || updated == created {
		return created
	}

	value := created + " -> " + updated
	if duration := formatDurationSeconds(report.DurationSeconds); duration != "" {
		value += " (" + duration + ")"
	}
	return value
}

func formatShowTokenUsage(usage showTokenUsageReport) string {
	return fmt.Sprintf(
		"%s total; %s input (%s cached); %s output (incl. %s reasoning)",
		formatCompactNumber(usage.TotalTokens),
		formatCompactNumber(usage.InputTokens),
		formatCompactNumber(usage.CachedInputTokens),
		formatCompactNumber(usage.OutputTokens),
		formatCompactNumber(usage.ReasoningTokens),
	)
}

func printTokenUsage(w io.Writer, usage session.TokenUsage) error {
	_, err := fmt.Fprintf(
		w,
		"\nTokens:\n  Input:     %d\n  Cached:    %d\n  Output:    %d\n  Reasoning: %d\n  Total:     %d\n",
		usage.InputTokens,
		usage.CachedInputTokens,
		usage.OutputTokens,
		usage.ReasoningOutputTokens,
		usage.TotalTokens,
	)
	return err
}

func addUnique(values *[]string, value string) {
	if value == "" {
		return
	}
	for _, existing := range *values {
		if existing == value {
			return
		}
	}
	*values = append(*values, value)
}

func earliestFormattedTime(current string, next time.Time) string {
	if next.IsZero() {
		return current
	}
	if current == "" {
		return formatTime(next)
	}
	parsed, err := time.Parse(time.RFC3339, current)
	if err != nil || next.Before(parsed) {
		return formatTime(next)
	}
	return current
}

func latestFormattedTime(current string, next time.Time) string {
	if next.IsZero() {
		return current
	}
	if current == "" {
		return formatTime(next)
	}
	parsed, err := time.Parse(time.RFC3339, current)
	if err != nil || next.After(parsed) {
		return formatTime(next)
	}
	return current
}

func formatRange(start string, end string) string {
	if start == "" && end == "" {
		return ""
	}
	if start == "" {
		return end
	}
	if end == "" || start == end {
		return start
	}
	return start + " to " + end
}

func fallback(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func formatCount(value int) string {
	return fmt.Sprintf("%d", value)
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func formatDurationSeconds(seconds *int64) string {
	if seconds == nil {
		return ""
	}
	if *seconds == 0 {
		return "0s"
	}
	remaining := *seconds
	hours := remaining / 3600
	remaining %= 3600
	minutes := remaining / 60
	remaining %= 60

	var parts []string
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if remaining > 0 {
		parts = append(parts, fmt.Sprintf("%ds", remaining))
	}
	return strings.Join(parts, " ")
}

func formatNumber(value int64) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	digits := strconv.FormatInt(value, 10)
	if len(digits) <= 3 {
		return sign + digits
	}

	var b strings.Builder
	b.WriteString(sign)
	head := len(digits) % 3
	if head == 0 {
		head = 3
	}
	b.WriteString(digits[:head])
	for i := head; i < len(digits); i += 3 {
		b.WriteByte(',')
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

func formatCompactNumber(value int64) string {
	abs := value
	if abs < 0 {
		abs = -abs
	}

	if abs < 1_000 {
		return formatNumber(value)
	}

	divisor := float64(1_000)
	unit := "K"
	precision := 1
	if abs >= 1_000_000 {
		divisor = 1_000_000
		unit = "M"
		precision = 2
	}

	formatted := strconv.FormatFloat(float64(value)/divisor, 'f', precision, 64)
	formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	return formatted + unit
}
