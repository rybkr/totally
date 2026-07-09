package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"github.com/rybkr/totally/internal/session"
	"github.com/spf13/cobra"
)

type filesOptions struct {
	limit int
}

func newFilesCommand(stdout io.Writer, globals *globalOptions) *cobra.Command {
	var opts filesOptions

	cmd := &cobra.Command{
		Use:   "files",
		Short: "Find local agent session files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFiles(cmd, stdout, *globals, opts)
		},
	}

	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum number of files to print")

	return cmd
}

func runFiles(cmd *cobra.Command, stdout io.Writer, globals globalOptions, opts filesOptions) error {
	finders, err := globals.finders()
	if err != nil {
		return err
	}
	bounds, err := globals.timeRange(time.Now())
	if err != nil {
		return err
	}

	var files []session.FileRef
	for _, finder := range finders {
		found, err := finder.FindSessionFiles(cmd.Context(), session.FindOptions{
			Roots:           globals.homes,
			IncludeArchived: globals.archived,
		})
		if err != nil {
			return err
		}
		files = append(files, found...)
	}

	files = filterFilesByTimeRange(files, bounds)

	sort.Slice(files, func(i, j int) bool {
		if !files[i].CreatedAt.Equal(files[j].CreatedAt) {
			return files[i].CreatedAt.After(files[j].CreatedAt)
		}
		return files[i].Path > files[j].Path
	})

	if opts.limit > 0 && len(files) > opts.limit {
		files = files[:opts.limit]
	}

	switch globals.format {
	case outputFormatTable:
		return printFilesTable(stdout, files)
	case outputFormatJSON:
		return json.NewEncoder(stdout).Encode(files)
	default:
		return fmt.Errorf("unknown format %q", globals.format)
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
