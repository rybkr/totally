package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rybkr/totally/internal/session"
	"github.com/spf13/cobra"
)

type inspectOptions struct {
	latest bool
}

type inspectSummary struct {
	Sessions int
	Sources  []string

	CreatedStart string
	CreatedEnd   string
	UpdatedStart string
	UpdatedEnd   string

	CWDs        []string
	Models      []string
	Providers   []string
	CLIVersions []string

	Turns     int
	Messages  int
	ToolCalls int

	TokenUsage session.TokenUsage
}

func newInspectCommand(stdout io.Writer, globals *globalOptions) *cobra.Command {
	var opts inspectOptions

	cmd := &cobra.Command{
		Use:   "inspect [session-id-or-path]",
		Short: "Inspect local agent session usage",
		Args: func(cmd *cobra.Command, args []string) error {
			if opts.latest {
				return cobra.NoArgs(cmd, args)
			}
			return cobra.MaximumNArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInspect(cmd, stdout, *globals, opts, args)
		},
	}

	cmd.Flags().BoolVar(&opts.latest, "latest", false, "inspect the most recently updated session file")

	return cmd
}

func runInspect(cmd *cobra.Command, stdout io.Writer, globals globalOptions, opts inspectOptions, args []string) error {
	parsers, err := globals.parsers()
	if err != nil {
		return err
	}

	if len(args) == 0 && !opts.latest {
		records, err := parseDiscoveredSessions(cmd, globals, parsers)
		if err != nil {
			return err
		}
		summary := summarizeRecords(records)
		switch globals.format {
		case outputFormatTable:
			return printInspectSummary(stdout, summary)
		case outputFormatJSON:
			return json.NewEncoder(stdout).Encode(summary)
		default:
			return fmt.Errorf("unknown format %q", globals.format)
		}
	}

	file, err := resolveInspectTarget(cmd, globals, opts, args)
	if err != nil {
		return err
	}

	parser, err := parserForSource(parsers, file.Source)
	if err != nil {
		return err
	}

	record, err := parser.ParseSession(cmd.Context(), file)
	if err != nil {
		return err
	}

	switch globals.format {
	case outputFormatTable:
		return printInspect(stdout, record)
	case outputFormatJSON:
		return json.NewEncoder(stdout).Encode(record)
	default:
		return fmt.Errorf("unknown format %q", globals.format)
	}
}

func parseDiscoveredSessions(cmd *cobra.Command, globals globalOptions, parsers []session.Parser) ([]session.Record, error) {
	files, err := discoverSessionFiles(cmd, globals)
	if err != nil {
		return nil, err
	}
	sortFilesByCreated(files)
	return parseSessionFilesWithParsers(cmd, parsers, files)
}

func resolveInspectTarget(cmd *cobra.Command, globals globalOptions, opts inspectOptions, args []string) (session.FileRef, error) {
	if opts.latest {
		return latestSessionFile(cmd, globals)
	}

	target := args[0]
	if file, ok, err := fileRefFromPath(target); ok || err != nil {
		return file, err
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
			if file.SessionID == target {
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
		return session.FileRef{}, fmt.Errorf("multiple sessions found for %q; pass a file path or narrow --agent", target)
	}
}

func fileRefFromPath(target string) (session.FileRef, bool, error) {
	path := expandHomePath(target)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return session.FileRef{}, false, nil
		}
		return session.FileRef{}, true, err
	}
	if info.IsDir() {
		return session.FileRef{}, true, fmt.Errorf("%q is a directory, expected a session file", target)
	}

	format := session.FileFormatJSONL
	compressed := false
	switch {
	case strings.HasSuffix(path, ".jsonl.zst"):
		format = session.FileFormatJSONLZstd
		compressed = true
	case strings.HasSuffix(path, ".jsonl"):
		format = session.FileFormatJSONL
	default:
		return session.FileRef{}, true, fmt.Errorf("unsupported session file extension %q", filepath.Ext(path))
	}

	return session.FileRef{
		Source:     sourceForPath(path),
		Role:       session.FileRoleTranscript,
		Format:     format,
		Path:       path,
		Compressed: compressed,
		UpdatedAt:  info.ModTime(),
		SizeBytes:  info.Size(),
	}, true, nil
}

func sourceForPath(path string) session.Source {
	if strings.Contains(path, string(filepath.Separator)+".codex"+string(filepath.Separator)) ||
		strings.Contains(filepath.Base(path), "rollout-") {
		return "codex"
	}
	return "codex"
}

func parserForSource(parsers []session.Parser, source session.Source) (session.Parser, error) {
	for _, parser := range parsers {
		if parser.Source() == source {
			return parser, nil
		}
	}
	return nil, fmt.Errorf("no parser registered for source %q", source)
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

func printInspect(w io.Writer, record session.Record) error {
	lines := []struct {
		label string
		value string
	}{
		{"Session", record.SessionID},
		{"Source", string(record.Source)},
		{"Created", formatTime(record.CreatedAt)},
		{"Updated", formatTime(record.UpdatedAt)},
		{"Path", record.Path},
		{"CWD", record.CWD},
		{"Models", strings.Join(record.Models, ", ")},
		{"Provider", record.Provider},
		{"CLI", record.CLIVersion},
		{"Turns", formatCount(record.Turns)},
		{"Messages", formatCount(record.Messages)},
		{"Tool calls", formatCount(record.ToolCalls)},
	}

	for _, line := range lines {
		if _, err := fmt.Fprintf(w, "%-11s %s\n", line.label+":", fallback(line.value)); err != nil {
			return err
		}
	}

	return printTokenUsage(w, record.TokenUsage)
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
