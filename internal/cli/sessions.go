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

	"github.com/rybkr/totally/internal/session"
	"github.com/spf13/cobra"
)

type sessionsOptions struct {
	limit   int
	latest  bool
	summary bool
	ids     bool
	paths   bool
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
		if shouldPageSessions(stdout, globals.noPager) {
			var table bytes.Buffer
			if err := printSessionsTable(&table, records); err != nil {
				return err
			}
			return pageSessionsOutput(cmd, stdout, table.Bytes())
		}
		return printSessionsTable(stdout, records)
	case outputFormatJSON:
		return json.NewEncoder(stdout).Encode(records)
	default:
		return fmt.Errorf("unknown format %q", globals.format)
	}
}

func shouldPageSessions(stdout io.Writer, noPager bool) bool {
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

func pageSessionsOutput(cmd *cobra.Command, stdout io.Writer, output []byte) error {
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
			return nil, fmt.Errorf("parse %s: %w", file.Path, err)
		}
		records = append(records, record)
	}
	return records, nil
}

func printSessionsTable(w io.Writer, records []session.Record) error {
	if _, err := fmt.Fprintln(w, "SESSION ID\tCWD\tPROMPT"); err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	for _, record := range records {
		if _, err := fmt.Fprintf(
			w,
			"%s\t%s\t%s\n",
			record.SessionID,
			shortenSessionCWD(record.CWD, home),
			formatSessionPrompt(record.FirstPrompt),
		); err != nil {
			return err
		}
	}
	return nil
}

const sessionPromptMaxRunes = 80

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
	prompt = strings.Join(strings.Fields(prompt), " ")
	if prompt == "" {
		return "-"
	}

	runes := []rune(prompt)
	if len(runes) <= sessionPromptMaxRunes {
		return prompt
	}
	return string(runes[:sessionPromptMaxRunes-3]) + "..."
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
