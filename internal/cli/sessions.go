package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

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

	sortFilesByCreated(files)
	if opts.latest {
		sortFilesByUpdated(files)
		if opts.limit == 0 {
			opts.limit = 1
		}
	}
	if opts.limit > 0 && len(files) > opts.limit {
		files = files[:opts.limit]
	}

	records, err := parseSessionFiles(cmd, globals, files)
	if err != nil {
		return err
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
		return printSessionsTable(stdout, records)
	case outputFormatJSON:
		return json.NewEncoder(stdout).Encode(records)
	default:
		return fmt.Errorf("unknown format %q", globals.format)
	}
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
	if _, err := fmt.Fprintln(w, "SESSION\tCREATED\tUPDATED\tMODELS\tTURNS\tMESSAGES\tTOOLS\tTOKENS\tCWD"); err != nil {
		return err
	}
	for _, record := range records {
		if _, err := fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%d\t%d\t%d\t%d\t%s\n",
			record.SessionID,
			formatTime(record.CreatedAt),
			formatTime(record.UpdatedAt),
			strings.Join(record.Models, ", "),
			record.Turns,
			record.Messages,
			record.ToolCalls,
			record.TokenUsage.TotalTokens,
			record.CWD,
		); err != nil {
			return err
		}
	}
	return nil
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
