package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rybkr/totally/internal/pricing"
	"github.com/rybkr/totally/internal/session"
	"github.com/spf13/cobra"
)

// statsOptions deliberately mirrors the qualifiers accepted by `show --latest`,
// except that they select every matching session instead of one session.
type statsOptions struct {
	cwd      string
	provider string
	model    string
	by       []string
	pretty   bool
}

type statsReport struct {
	Sessions        int                `json:"sessions"`
	Prompts         int                `json:"prompts"`
	DurationSeconds int64              `json:"duration_seconds"`
	Turns           int                `json:"turns"`
	Messages        int                `json:"messages"`
	ToolCalls       int                `json:"tool_calls"`
	TokenUsage      session.TokenUsage `json:"token_usage"`
	CostUSD         float64            `json:"cost_usd"`
	Cost            pricing.Estimate   `json:"cost"`
}

type groupedStatsReport struct {
	By           string           `json:"by"`
	ByDimensions []string         `json:"by_dimensions,omitempty"`
	Total        statsReport      `json:"total"`
	Groups       []statsReportRow `json:"groups"`
}

type statsReportRow struct {
	Group  string      `json:"group"`
	Values []string    `json:"values,omitempty"`
	Stats  statsReport `json:"stats"`
}

func newStatsCommand(stdout io.Writer, globals *globalOptions) *cobra.Command {
	var opts statsOptions
	cmd := &cobra.Command{
		Use:   "stats [--cwd PATH] [--provider NAME] [--model NAME] [--by DIMENSION]...",
		Short: "Aggregate session activity, token use, and estimated cost",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStats(cmd, stdout, *globals, opts)
		},
	}
	cmd.Flags().StringVar(&opts.cwd, "cwd", "", "limit to sessions in this working directory")
	cmd.Flags().StringVar(&opts.provider, "provider", "", "limit to sessions from this provider")
	cmd.Flags().StringVar(&opts.model, "model", "", "limit to sessions using this model")
	cmd.Flags().StringArrayVar(&opts.by, "by", nil, "group by cwd, model, provider, day, week, month, or session; may be repeated")
	cmd.Flags().BoolVar(&opts.pretty, "pretty", false, "use terminal-oriented table output")
	return cmd
}

func runStats(cmd *cobra.Command, stdout io.Writer, globals globalOptions, opts statsOptions) error {
	if opts.cwd != "" {
		cwd, err := filepath.Abs(filepath.Clean(opts.cwd))
		if err != nil {
			return fmt.Errorf("resolve --cwd: %w", err)
		}
		opts.cwd = cwd
	}
	if err := validateStatsBy(opts.by); err != nil {
		return usageError{err: err}
	}
	files, err := discoverSessionFiles(cmd, globals)
	if err != nil {
		return err
	}
	records, err := parseSessionFiles(cmd, globals, files)
	if err != nil {
		return err
	}
	records = filterStatsRecords(records, opts)
	total := summarizeStats(records, globals.prices)
	if len(opts.by) == 0 {
		if globals.format == outputFormatJSON {
			return json.NewEncoder(stdout).Encode(total)
		}
		return printStatsReport(stdout, total)
	}
	groups := groupStatsRecords(records, opts.by)
	report := groupedStatsReport{By: strings.Join(opts.by, ","), Total: total, Groups: make([]statsReportRow, 0, len(groups))}
	if len(opts.by) > 1 {
		report.ByDimensions = opts.by
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		values := strings.Split(key, "\x1f")
		group := values[0]
		if len(values) > 1 {
			group = strings.Join(values, " / ")
		}
		row := statsReportRow{Group: group, Stats: summarizeGroupedStats(groups[key], globals.prices)}
		if len(values) > 1 {
			row.Values = values
		}
		report.Groups = append(report.Groups, row)
	}
	if globals.format == outputFormatJSON {
		return json.NewEncoder(stdout).Encode(report)
	}
	return printGroupedStatsReport(stdout, report)
}

func validateStatsBy(by []string) error {
	seen := make(map[string]struct{}, len(by))
	for _, dimension := range by {
		switch dimension {
		case "cwd", "model", "provider", "day", "week", "month", "session":
		default:
			return fmt.Errorf("unknown --by value %q: expected cwd, model, provider, day, week, month, or session", dimension)
		}
		if _, ok := seen[dimension]; ok {
			return fmt.Errorf("duplicate --by value %q", dimension)
		}
		seen[dimension] = struct{}{}
	}
	return nil
}

func filterStatsRecords(records []session.Record, opts statsOptions) []session.Record {
	filtered := records[:0]
	for _, record := range records {
		if opts.cwd != "" {
			cwd, err := filepath.Abs(filepath.Clean(record.CWD))
			if err != nil || cwd != opts.cwd {
				continue
			}
		}
		if opts.provider != "" && !strings.EqualFold(record.Provider, opts.provider) {
			continue
		}
		if opts.model != "" && !hasShowModel(record.Models, opts.model) {
			continue
		}
		filtered = append(filtered, record)
	}
	return filtered
}

type groupedStatsRecord struct {
	Record              session.Record
	IncludeSessionStats bool
}

func groupStatsRecords(records []session.Record, by []string) map[string][]groupedStatsRecord {
	groups := make(map[string][]groupedStatsRecord)
	for _, record := range records {
		attributed := []groupedStatsRecord{{Record: record, IncludeSessionStats: true}}
		if containsStatsDimension(by, "model") {
			attributed = groupStatsRecordsByModel(record)
		}
		for _, grouped := range attributed {
			values := make([]string, 0, len(by))
			for _, dimension := range by {
				keys := statsGroupKeys(grouped.Record, dimension)
				values = append(values, keys[0])
			}
			key := strings.Join(values, "\x1f")
			groups[key] = append(groups[key], grouped)
		}
	}
	return groups
}

func containsStatsDimension(dimensions []string, target string) bool {
	for _, dimension := range dimensions {
		if dimension == target {
			return true
		}
	}
	return false
}

// groupStatsRecordByModel attributes tokens and pricing from each usage segment
// to its model. Session-level fields cannot be split reliably, so they are
// assigned to the model with the most attributed tokens (first seen wins ties).
// This keeps model groups additive without inventing fractional sessions or
// activity counts.
func groupStatsRecordByModel(groups map[string][]groupedStatsRecord, record session.Record) {
	for _, attributed := range groupStatsRecordsByModel(record) {
		groups[attributed.Record.Models[0]] = append(groups[attributed.Record.Models[0]], attributed)
	}
}

func groupStatsRecordsByModel(record session.Record) []groupedStatsRecord {
	segmentsByModel := make(map[string][]session.UsageSegment)
	models := make([]string, 0, len(record.UsageSegments))
	for _, segment := range record.UsageSegments {
		if segment.Model == "" {
			continue
		}
		if _, ok := segmentsByModel[segment.Model]; !ok {
			models = append(models, segment.Model)
		}
		segmentsByModel[segment.Model] = append(segmentsByModel[segment.Model], segment)
	}
	if len(models) == 0 {
		key := "(unknown)"
		if len(record.Models) > 0 && record.Models[0] != "" {
			key = record.Models[0]
		}
		record.Models = []string{key}
		return []groupedStatsRecord{{Record: record, IncludeSessionStats: true}}
	}

	primary := models[0]
	var primaryTokens int64 = -1
	result := make([]groupedStatsRecord, 0, len(models))
	for _, model := range models {
		var tokens int64
		for _, segment := range segmentsByModel[model] {
			tokens += segment.TokenUsage.TotalTokens
		}
		if tokens > primaryTokens {
			primary, primaryTokens = model, tokens
		}
	}
	for _, model := range models {
		attributed := record
		attributed.Models = []string{model}
		attributed.UsageSegments = segmentsByModel[model]
		attributed.TokenUsage = session.TokenUsage{}
		for _, segment := range attributed.UsageSegments {
			addTokenUsage(&attributed.TokenUsage, segment.TokenUsage)
		}
		result = append(result, groupedStatsRecord{Record: attributed, IncludeSessionStats: model == primary})
	}
	return result
}

func statsGroupKeys(record session.Record, by string) []string {
	unknown := "(unknown)"
	switch by {
	case "cwd":
		if record.CWD != "" {
			return []string{record.CWD}
		}
	case "provider":
		if record.Provider != "" {
			return []string{record.Provider}
		}
	case "model":
		if len(record.Models) > 0 {
			return record.Models
		}
	case "session":
		return []string{record.SessionID}
	case "day":
		if !record.CreatedAt.IsZero() {
			return []string{record.CreatedAt.Local().Format(time.DateOnly)}
		}
	case "week":
		if !record.CreatedAt.IsZero() {
			year, week := record.CreatedAt.Local().ISOWeek()
			return []string{fmt.Sprintf("%04d-W%02d", year, week)}
		}
	case "month":
		if !record.CreatedAt.IsZero() {
			return []string{record.CreatedAt.Local().Format("2006-01")}
		}
	}
	return []string{unknown}
}

func summarizeStats(records []session.Record, catalog pricing.Catalog) statsReport {
	result := statsReport{Cost: pricing.Estimate{Currency: "USD", Status: "unavailable", Basis: "api_equivalent", PricingVersion: pricing.CatalogVersion}}
	var amount big.Rat
	for _, record := range records {
		addStatsRecord(&result, &amount, record, catalog, true)
	}
	return finalizeStatsReport(result, amount)
}

func summarizeGroupedStats(records []groupedStatsRecord, catalog pricing.Catalog) statsReport {
	result := statsReport{Cost: pricing.Estimate{Currency: "USD", Status: "unavailable", Basis: "api_equivalent", PricingVersion: pricing.CatalogVersion}}
	var amount big.Rat
	for _, record := range records {
		addStatsRecord(&result, &amount, record.Record, catalog, record.IncludeSessionStats)
	}
	return finalizeStatsReport(result, amount)
}

func addStatsRecord(result *statsReport, amount *big.Rat, record session.Record, catalog pricing.Catalog, includeSessionStats bool) {
	if includeSessionStats {
		result.Sessions++
		if record.FirstPrompt != "" {
			result.Prompts++
		}
		if !record.CreatedAt.IsZero() && !record.UpdatedAt.IsZero() && !record.UpdatedAt.Before(record.CreatedAt) {
			result.DurationSeconds += int64(record.UpdatedAt.Sub(record.CreatedAt).Seconds())
		}
		result.Turns += record.Turns
		result.Messages += record.Messages
		result.ToolCalls += record.ToolCalls
	}
	addTokenUsage(&result.TokenUsage, record.TokenUsage)
	estimate := catalog.Estimate(record.UsageSegments, record.CreatedAt)
	result.Cost.Components = append(result.Cost.Components, estimate.Components...)
	for _, missing := range estimate.Missing {
		addMissingRate(&result.Cost.Missing, missing)
	}
	for _, limitation := range estimate.Limitations {
		addUniqueString(&result.Cost.Limitations, limitation)
	}
	if estimate.AmountUSD != nil {
		value, ok := new(big.Rat).SetString(*estimate.AmountUSD)
		if ok {
			amount.Add(amount, value)
		}
	}
}

func finalizeStatsReport(result statsReport, amount big.Rat) statsReport {
	if len(result.Cost.Components) > 0 {
		value := strings.TrimRight(strings.TrimRight(amount.FloatString(9), "0"), ".")
		if value == "" {
			value = "0"
		}
		result.Cost.AmountUSD = &value
		result.CostUSD, _ = amount.Float64()
		result.Cost.Status = "complete"
		if len(result.Cost.Missing) > 0 || len(result.Cost.Limitations) > 0 {
			result.Cost.Status = "partial"
		}
	} else if len(result.Cost.Limitations) > 0 {
		result.Cost.Status = "partial"
	}
	return result
}

func addTokenUsage(total *session.TokenUsage, usage session.TokenUsage) {
	total.InputTokens += usage.InputTokens
	total.CachedInputTokens += usage.CachedInputTokens
	total.OutputTokens += usage.OutputTokens
	total.ReasoningOutputTokens += usage.ReasoningOutputTokens
	total.TotalTokens += usage.TotalTokens
}
func addMissingRate(values *[]pricing.MissingRate, value pricing.MissingRate) {
	for _, existing := range *values {
		if existing == value {
			return
		}
	}
	*values = append(*values, value)
}
func addUniqueString(values *[]string, value string) {
	for _, existing := range *values {
		if existing == value {
			return
		}
	}
	*values = append(*values, value)
}

func printStatsReport(w io.Writer, report statsReport) error {
	for _, line := range []struct{ label, value string }{
		{"Sessions", formatCount(report.Sessions)}, {"Prompts", formatCount(report.Prompts)}, {"Duration", formatDurationSeconds(&report.DurationSeconds)},
		{"Activity", fmt.Sprintf("%s turns, %s messages, %s tool calls", formatNumber(int64(report.Turns)), formatNumber(int64(report.Messages)), formatNumber(int64(report.ToolCalls)))},
		{"Tokens", formatShowTokenUsage(showTokenUsageReport{InputTokens: report.TokenUsage.InputTokens, CachedInputTokens: report.TokenUsage.CachedInputTokens, OutputTokens: report.TokenUsage.OutputTokens, ReasoningTokens: report.TokenUsage.ReasoningOutputTokens, TotalTokens: report.TokenUsage.TotalTokens})},
		{"Cost", formatCostEstimate(report.Cost)},
	} {
		if _, err := fmt.Fprintf(w, "%-11s %s\n", line.label, fallback(line.value)); err != nil {
			return err
		}
	}
	return nil
}

func printGroupedStatsReport(w io.Writer, report groupedStatsReport) error {
	dimensions := report.ByDimensions
	if len(dimensions) == 0 {
		dimensions = []string{report.By}
	}
	groupColumns := make([]string, len(dimensions))
	for i, dimension := range dimensions {
		groupColumns[i] = strings.ToUpper(dimension)
	}
	if _, err := fmt.Fprintf(w, "%s\tSESSIONS\tPROMPTS\tDURATION\tTOKENS\tCOST\n", strings.Join(groupColumns, "\t")); err != nil {
		return err
	}
	for _, row := range report.Groups {
		values := row.Values
		if len(values) == 0 {
			values = []string{row.Group}
		}
		if _, err := fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%s\t%s\n", strings.Join(values, "\t"), row.Stats.Sessions, row.Stats.Prompts, fallback(formatDurationSeconds(&row.Stats.DurationSeconds)), formatCompactNumber(row.Stats.TokenUsage.TotalTokens), formatCostEstimate(row.Stats.Cost)); err != nil {
			return err
		}
	}
	return nil
}
