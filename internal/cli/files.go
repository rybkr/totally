package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rybkr/totally/internal/provider/codex"
	"github.com/rybkr/totally/internal/session"
	"github.com/spf13/cobra"
)

const allAgents = "all"

type filesOptions struct {
	agent    string
	homes    []string
	archived bool
	format   string
	limit    int
}

func newFilesCommand(stdout io.Writer) *cobra.Command {
	var opts filesOptions

	cmd := &cobra.Command{
		Use:   "files",
		Short: "Find local agent session files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFiles(cmd, stdout, opts)
		},
	}

	cmd.Flags().StringVar(&opts.agent, "agent", allAgents, "agent session format to discover: all, codex")
	cmd.Flags().StringArrayVar(&opts.homes, "home", nil, "agent home directory; may be repeated")
	cmd.Flags().BoolVar(&opts.archived, "archived", false, "include archived sessions")
	cmd.Flags().StringVar(&opts.format, "format", "table", "output format: table, json")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of files to print")

	return cmd
}

func runFiles(cmd *cobra.Command, stdout io.Writer, opts filesOptions) error {
	finders, err := findersForAgent(opts.agent)
	if err != nil {
		return err
	}

	var files []session.FileRef
	for _, finder := range finders {
		found, err := finder.FindSessionFiles(cmd.Context(), session.FindOptions{
			Roots:           opts.homes,
			IncludeArchived: opts.archived,
		})
		if err != nil {
			return err
		}
		files = append(files, found...)
	}

	sort.Slice(files, func(i, j int) bool {
		if !files[i].CreatedAt.Equal(files[j].CreatedAt) {
			return files[i].CreatedAt.After(files[j].CreatedAt)
		}
		return files[i].Path > files[j].Path
	})

	if opts.limit > 0 && len(files) > opts.limit {
		files = files[:opts.limit]
	}

	switch opts.format {
	case "table":
		return printFilesTable(stdout, files)
	case "json":
		return json.NewEncoder(stdout).Encode(files)
	default:
		return fmt.Errorf("unknown format %q", opts.format)
	}
}

func findersForAgent(agent string) ([]session.Finder, error) {
	switch strings.ToLower(agent) {
	case "", allAgents:
		return []session.Finder{codex.NewFinder()}, nil
	case string(codex.Source):
		return []session.Finder{codex.NewFinder()}, nil
	default:
		return nil, fmt.Errorf("unknown agent %q", agent)
	}
}

func printFilesTable(w io.Writer, files []session.FileRef) error {
	if _, err := fmt.Fprintln(w, "SOURCE\tFORMAT\tSESSION\tCREATED\tSIZE\tPATH"); err != nil {
		return err
	}
	for _, file := range files {
		if _, err := fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t%s\n",
			file.Source,
			file.Format,
			file.SessionID,
			formatTime(file.CreatedAt),
			formatBytes(file.SizeBytes),
			file.Path,
		); err != nil {
			return err
		}
	}
	return nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

func formatBytes(size int64) string {
	if size < 0 {
		return "-"
	}
	if size < 1024 {
		return strconv.FormatInt(size, 10) + "B"
	}

	units := []string{"KiB", "MiB", "GiB", "TiB"}
	value := float64(size)
	for _, unit := range units {
		value /= 1024
		if value < 1024 {
			return fmt.Sprintf("%.1f%s", value, unit)
		}
	}
	return fmt.Sprintf("%.1fPiB", value/1024)
}
