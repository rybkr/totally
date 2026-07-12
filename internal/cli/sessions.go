package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rybkr/totally/internal/pricing"
	"github.com/rybkr/totally/internal/session"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"golang.org/x/text/width"
)

type sessionsOptions struct {
	limit   int
	latest  bool
	summary bool
	ids     bool
	paths   bool
	full    bool
}

// sessionListReport adds derived, comparison-oriented fields to the normalized
// session record without changing the underlying parser contract.
type sessionListReport struct {
	session.Record
	DurationSeconds *int64           `json:"duration_seconds,omitempty"`
	CostUSD         float64          `json:"cost_usd"`
	Cost            pricing.Estimate `json:"cost"`
}

func newSessionsCommand(stdout io.Writer, globals *globalOptions) *cobra.Command {
	var opts sessionsOptions

	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "List parsed agent sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessions(cmd, stdout, *globals, opts)
		},
	}

	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of sessions to print")
	cmd.Flags().BoolVar(&opts.latest, "latest", false, "sort by most recently updated session and default to one result")
	cmd.Flags().BoolVar(&opts.summary, "summary", false, "print an aggregate session summary")
	cmd.Flags().BoolVar(&opts.ids, "ids", false, "print only session IDs")
	cmd.Flags().BoolVar(&opts.paths, "paths", false, "print only backing transcript paths")
	cmd.Flags().BoolVar(&opts.full, "full", false, "do not truncate display values in table output")

	return cmd
}

func runSessions(cmd *cobra.Command, stdout io.Writer, globals globalOptions, opts sessionsOptions) error {
	if err := validateSessionsOptions(opts); err != nil {
		return err
	}

	files, err := discoverSessionFiles(cmd, globals)
	if err != nil {
		return err
	}

	records, err := parseSessionFiles(cmd, globals, files)
	if err != nil {
		return err
	}

	sortRecordsByCreated(records)
	if opts.latest {
		sortRecordsByUpdated(records)
		if opts.limit == 0 {
			opts.limit = 1
		}
	}
	if opts.limit > 0 && len(records) > opts.limit {
		records = records[:opts.limit]
	}

	if opts.ids {
		return printSessionIDs(stdout, records)
	}

	if opts.paths {
		return printSessionPaths(stdout, records)
	}

	if opts.summary {
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

	switch globals.format {
	case outputFormatTable:
		terminalWidth, _ := outputTerminalWidth(stdout)
		if shouldPageTable(stdout, globals.noPager) {
			var table bytes.Buffer
			if err := printSessionsTableWithWidth(&table, records, globals.prices, opts.full, terminalWidth); err != nil {
				return err
			}
			return pageTableOutput(cmd, stdout, table.Bytes())
		}
		return printSessionsTableWithWidth(stdout, records, globals.prices, opts.full, terminalWidth)
	case outputFormatJSON:
		return json.NewEncoder(stdout).Encode(newSessionListReports(records, globals.prices))
	default:
		return fmt.Errorf("unknown format %q", globals.format)
	}
}

func outputTerminalWidth(w io.Writer) (int, bool) {
	file, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return 0, false
	}
	columns, _, err := term.GetSize(int(file.Fd()))
	if err != nil || columns <= 0 {
		return 0, false
	}
	return columns, true
}

func shouldPageTable(stdout io.Writer, noPager bool) bool {
	if noPager {
		return false
	}
	file, ok := stdout.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func pageTableOutput(cmd *cobra.Command, stdout io.Writer, output []byte) error {
	args := strings.Fields(os.Getenv("PAGER"))
	if len(args) == 0 {
		args = []string{"less", "-FRX"}
	}

	pagerPath, err := exec.LookPath(args[0])
	if err != nil {
		_, writeErr := stdout.Write(output)
		return writeErr
	}

	pager := exec.CommandContext(cmd.Context(), pagerPath, args[1:]...)
	pager.Stdin = bytes.NewReader(output)
	pager.Stdout = stdout
	pager.Stderr = cmd.ErrOrStderr()
	if err := pager.Run(); err != nil {
		return fmt.Errorf("run pager %q: %w", args[0], err)
	}
	return nil
}

func validateSessionsOptions(opts sessionsOptions) error {
	modes := 0
	for _, enabled := range []bool{opts.summary, opts.ids, opts.paths} {
		if enabled {
			modes++
		}
	}
	if modes > 1 {
		return fmt.Errorf("--summary, --ids, and --paths are mutually exclusive")
	}
	return nil
}

func parseSessionFiles(cmd *cobra.Command, globals globalOptions, files []session.FileRef) ([]session.Record, error) {
	parsers, err := globals.parsers()
	if err != nil {
		return nil, err
	}
	return parseSessionFilesWithParsers(cmd, parsers, files)
}

func parseSessionFilesWithParsers(cmd *cobra.Command, parsers []session.Parser, files []session.FileRef) ([]session.Record, error) {
	records := make([]session.Record, 0, len(files))
	for _, file := range files {
		parser, err := parserForSource(parsers, file.Source)
		if err != nil {
			return nil, err
		}
		record, err := parser.ParseSession(cmd.Context(), file)
		if err != nil {
			// A transcript is external, append-only data and can be truncated or
			// otherwise malformed independently of every other discovered file.
			// Keep machine-readable command output on stdout and report the
			// individual failure on stderr instead of losing usable sessions.
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: skip session transcript %s: %v\n", file.Path, err)
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

func printSessionsTable(w io.Writer, records []session.Record, catalog pricing.Catalog, full bool) error {
	return printSessionsTableWithWidth(w, records, catalog, full, 0)
}

func printSessionsTableWithWidth(w io.Writer, records []session.Record, catalog pricing.Catalog, full bool, terminalWidth int) error {
	if _, err := fmt.Fprintln(w, "SESSION ID\tSTARTED\tCWD\tMODEL\tTOKENS\tCOST\tDURATION\tPROMPT"); err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	for _, record := range records {
		sessionID := formatSessionID(record.SessionID)
		started := formatTime(record.CreatedAt)
		cwd := shortenSessionCWD(record.CWD, home)
		model := fallback(strings.Join(record.Models, ","))
		tokens := formatCompactNumber(record.TokenUsage.TotalTokens)
		cost := formatSessionListCost(catalog.Estimate(record.UsageSegments, record.CreatedAt))
		duration := fallback(formatDurationSeconds(sessionDurationSeconds(record)))
		promptMaxWidth := sessionPromptMaxRunes
		if terminalWidth > 0 {
			promptMaxWidth = sessionPromptMaxForTerminalWidth(terminalWidth, sessionID, started, cwd, model, tokens, cost, duration)
		}
		prompt := formatSessionPromptToWidth(record.FirstPrompt, promptMaxWidth)
		if full {
			sessionID = record.SessionID
			prompt = record.FirstPrompt
		}
		if _, err := fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			sessionID,
			started,
			cwd,
			model,
			tokens,
			cost,
			duration,
			prompt,
		); err != nil {
			return err
		}
	}
	return nil
}

func newSessionListReports(records []session.Record, catalog pricing.Catalog) []sessionListReport {
	reports := make([]sessionListReport, 0, len(records))
	for _, record := range records {
		estimate := catalog.Estimate(record.UsageSegments, record.CreatedAt)
		reports = append(reports, sessionListReport{
			Record:          record,
			DurationSeconds: sessionDurationSeconds(record),
			CostUSD:         pricing.FloatAmount(estimate),
			Cost:            estimate,
		})
	}
	return reports
}

func sessionDurationSeconds(record session.Record) *int64 {
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() || record.UpdatedAt.Before(record.CreatedAt) {
		return nil
	}
	duration := int64(record.UpdatedAt.Sub(record.CreatedAt).Seconds())
	return &duration
}

func formatSessionListCost(cost pricing.Estimate) string {
	if cost.AmountUSD == nil {
		return "-"
	}
	prefix := "$"
	if cost.Status == "partial" || cost.UncertaintyUSD != nil {
		prefix = "~$"
	}
	return prefix + *cost.AmountUSD
}

const (
	sessionIDPrefixRunes = 13
	// Leave room for eight preview characters and the truncation ellipsis.
	sessionPromptMinRunes = 11
	sessionPromptMaxRunes = 160
)

func formatSessionID(sessionID string) string {
	runes := []rune(sessionID)
	if len(runes) <= sessionIDPrefixRunes {
		return sessionID
	}
	return string(runes[:sessionIDPrefixRunes])
}

func shortenSessionCWD(cwd string, home string) string {
	if cwd == "" || home == "" {
		return fallback(cwd)
	}
	if cwd == home {
		return "~"
	}
	prefix := home + string(filepath.Separator)
	if strings.HasPrefix(cwd, prefix) {
		return "~" + strings.TrimPrefix(cwd, home)
	}
	return cwd
}

func formatSessionPrompt(prompt string) string {
	return formatSessionPromptToWidth(prompt, sessionPromptMaxRunes)
}

func sessionPromptMaxForTerminalWidth(terminalWidth int, columns ...string) int {
	promptStart := 0
	for _, column := range columns {
		promptStart = nextTabStop(promptStart + displayWidth(column))
	}
	available := terminalWidth - promptStart
	if available < sessionPromptMinRunes {
		return sessionPromptMinRunes
	}
	if available > sessionPromptMaxRunes {
		return sessionPromptMaxRunes
	}
	return available
}

func nextTabStop(column int) int {
	const tabWidth = 8
	return ((column / tabWidth) + 1) * tabWidth
}

func formatSessionPromptToWidth(prompt string, maxWidth int) string {
	prompt = strings.Join(strings.Fields(prompt), " ")
	if maxWidth <= 0 {
		return ""
	}
	if prompt == "" {
		return "-"
	}

	if displayWidth(prompt) <= maxWidth {
		return prompt
	}
	if maxWidth <= 3 {
		return strings.Repeat(".", maxWidth)
	}

	limit := maxWidth - 3
	used := 0
	var preview strings.Builder
	for _, r := range prompt {
		runeWidth := displayWidth(string(r))
		if used+runeWidth > limit {
			break
		}
		preview.WriteRune(r)
		used += runeWidth
	}
	return preview.String() + "..."
}

func displayWidth(value string) int {
	columns := 0
	for _, r := range value {
		switch width.LookupRune(r).Kind() {
		case width.EastAsianWide, width.EastAsianFullwidth:
			columns += 2
		default:
			columns++
		}
	}
	return columns
}

func printSessionIDs(w io.Writer, records []session.Record) error {
	for _, record := range records {
		if _, err := fmt.Fprintln(w, record.SessionID); err != nil {
			return err
		}
	}
	return nil
}

func printSessionPaths(w io.Writer, records []session.Record) error {
	for _, record := range records {
		if _, err := fmt.Fprintln(w, record.Path); err != nil {
			return err
		}
	}
	return nil
}

func sortRecordsByCreated(records []session.Record) {
	sort.Slice(records, func(i, j int) bool {
		if !records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].CreatedAt.After(records[j].CreatedAt)
		}
		return records[i].Path > records[j].Path
	})
}

func sortRecordsByUpdated(records []session.Record) {
	sort.Slice(records, func(i, j int) bool {
		left := latestRecordActivityTime(records[i])
		right := latestRecordActivityTime(records[j])
		if !left.Equal(right) {
			return left.After(right)
		}
		return records[i].Path > records[j].Path
	})
}

func latestRecordActivityTime(record session.Record) time.Time {
	if !record.UpdatedAt.IsZero() {
		return record.UpdatedAt
	}
	return record.CreatedAt
}
