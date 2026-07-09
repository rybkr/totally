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
	limit  int
	latest bool
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
	cmd.Flags().BoolVar(&opts.latest, "latest", false, "print only the most recently updated session file")

	return cmd
}

func runFiles(cmd *cobra.Command, stdout io.Writer, globals globalOptions, opts filesOptions) error {
	files, err := discoverSessionFiles(cmd, globals)
	if err != nil {
		return err
	}

	sortFilesByCreated(files)

	if opts.latest {
		sortFilesByUpdated(files)
		opts.limit = 1
	}
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
	if _, err := fmt.Fprintln(w, "SOURCE\tFORMAT\tSESSION\tCREATED\tUPDATED\tSIZE\tPATH"); err != nil {
		return err
	}
	for _, file := range files {
		if _, err := fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			file.Source,
			file.Format,
			file.SessionID,
			formatTime(file.CreatedAt),
			formatTime(file.UpdatedAt),
			formatBytes(file.SizeBytes),
			file.Path,
		); err != nil {
			return err
		}
	}
	return nil
}

func discoverSessionFiles(cmd *cobra.Command, globals globalOptions) ([]session.FileRef, error) {
	finders, err := globals.finders()
	if err != nil {
		return nil, err
	}
	bounds, err := globals.timeRange(time.Now())
	if err != nil {
		return nil, err
	}

	var files []session.FileRef
	for _, finder := range finders {
		found, err := finder.FindSessionFiles(cmd.Context(), session.FindOptions{
			Roots:           globals.homes,
			IncludeArchived: globals.archived,
		})
		if err != nil {
			return nil, err
		}
		files = append(files, found...)
	}

	return filterFilesByTimeRange(files, bounds), nil
}

func latestSessionFile(cmd *cobra.Command, globals globalOptions) (session.FileRef, error) {
	files, err := discoverSessionFiles(cmd, globals)
	if err != nil {
		return session.FileRef{}, err
	}
	if len(files) == 0 {
		return session.FileRef{}, fmt.Errorf("no session files found")
	}
	sortFilesByUpdated(files)
	return files[0], nil
}

func sortFilesByCreated(files []session.FileRef) {
	sort.Slice(files, func(i, j int) bool {
		if !files[i].CreatedAt.Equal(files[j].CreatedAt) {
			return files[i].CreatedAt.After(files[j].CreatedAt)
		}
		return files[i].Path > files[j].Path
	})
}

func sortFilesByUpdated(files []session.FileRef) {
	sort.Slice(files, func(i, j int) bool {
		left := latestComparableTime(files[i])
		right := latestComparableTime(files[j])
		if !left.Equal(right) {
			return left.After(right)
		}
		return files[i].Path > files[j].Path
	})
}

func latestComparableTime(file session.FileRef) time.Time {
	if !file.UpdatedAt.IsZero() {
		return file.UpdatedAt
	}
	return file.CreatedAt
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
