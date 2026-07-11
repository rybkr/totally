package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rybkr/totally/internal/session"
	"github.com/spf13/cobra"
)

type filesOptions struct {
	limit   int
	latest  bool
	summary bool
	count   bool
	paths   bool
}

type filesSummary struct {
	Homes      []string `json:"homes"`
	Files      int      `json:"files"`
	Active     int      `json:"active"`
	Archived   int      `json:"archived"`
	Compressed int      `json:"compressed"`
	SizeBytes  int64    `json:"size_bytes"`
	Size       string   `json:"size"`
	Earliest   string   `json:"earliest"`
	Latest     string   `json:"latest"`
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
	cmd.Flags().BoolVar(&opts.summary, "summary", false, "print a storage and discovery summary")
	cmd.Flags().BoolVar(&opts.count, "count", false, "print only the number of discovered files")
	cmd.Flags().BoolVar(&opts.paths, "paths", false, "print only discovered file paths")

	return cmd
}

func runFiles(cmd *cobra.Command, stdout io.Writer, globals globalOptions, opts filesOptions) error {
	if err := validateFilesOptions(opts); err != nil {
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

	if opts.count {
		_, err := fmt.Fprintf(stdout, "%d\n", len(files))
		return err
	}

	if opts.paths {
		return printFilePaths(stdout, files)
	}

	if opts.summary {
		summary := summarizeFiles(files, globals)
		switch globals.format {
		case outputFormatTable:
			return printFilesSummary(stdout, summary)
		case outputFormatJSON:
			return json.NewEncoder(stdout).Encode(summary)
		default:
			return fmt.Errorf("unknown format %q", globals.format)
		}
	}

	switch globals.format {
	case outputFormatTable:
		if shouldPageTable(stdout, globals.noPager) {
			var table bytes.Buffer
			if err := printFilesTable(&table, files); err != nil {
				return err
			}
			return pageTableOutput(cmd, stdout, table.Bytes())
		}
		return printFilesTable(stdout, files)
	case outputFormatJSON:
		return json.NewEncoder(stdout).Encode(files)
	default:
		return fmt.Errorf("unknown format %q", globals.format)
	}
}

func validateFilesOptions(opts filesOptions) error {
	modes := 0
	for _, enabled := range []bool{opts.summary, opts.count, opts.paths} {
		if enabled {
			modes++
		}
	}
	if modes > 1 {
		return fmt.Errorf("--summary, --count, and --paths are mutually exclusive")
	}
	return nil
}

func printFilesTable(w io.Writer, files []session.FileRef) error {
	if _, err := fmt.Fprintln(w, "SOURCE\tROLE\tFORMAT\tSESSION\tCREATED\tUPDATED\tSIZE\tPATH"); err != nil {
		return err
	}
	for _, file := range files {
		if _, err := fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			file.Source,
			file.Role,
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

func printFilePaths(w io.Writer, files []session.FileRef) error {
	for _, file := range files {
		if _, err := fmt.Fprintln(w, file.Path); err != nil {
			return err
		}
	}
	return nil
}

func summarizeFiles(files []session.FileRef, globals globalOptions) filesSummary {
	summary := filesSummary{
		Homes: globals.homes,
		Files: len(files),
	}
	if len(summary.Homes) == 0 {
		summary.Homes = rootsFromFiles(files)
	}

	var earliest time.Time
	var latest time.Time
	for _, file := range files {
		if isArchivedFile(file) {
			summary.Archived++
		} else {
			summary.Active++
		}
		if file.Compressed {
			summary.Compressed++
		}
		if file.SizeBytes > 0 {
			summary.SizeBytes += file.SizeBytes
		}

		if !file.CreatedAt.IsZero() && (earliest.IsZero() || file.CreatedAt.Before(earliest)) {
			earliest = file.CreatedAt
		}
		comparable := latestComparableTime(file)
		if !comparable.IsZero() && (latest.IsZero() || comparable.After(latest)) {
			latest = comparable
		}
	}

	summary.Size = formatBytes(summary.SizeBytes)
	summary.Earliest = formatTime(earliest)
	summary.Latest = formatTime(latest)
	return summary
}

func printFilesSummary(w io.Writer, summary filesSummary) error {
	lines := []struct {
		label string
		value string
	}{
		{"Homes", strings.Join(summary.Homes, ", ")},
		{"Files", formatCount(summary.Files)},
		{"Active", formatCount(summary.Active)},
		{"Archived", formatCount(summary.Archived)},
		{"Compressed", formatCount(summary.Compressed)},
		{"Size", summary.Size},
		{"Earliest", summary.Earliest},
		{"Latest", summary.Latest},
	}

	for _, line := range lines {
		if _, err := fmt.Fprintf(w, "%-11s %s\n", line.label+":", fallback(line.value)); err != nil {
			return err
		}
	}
	return nil
}

func rootsFromFiles(files []session.FileRef) []string {
	var roots []string
	for _, file := range files {
		if root := rootFromSessionPath(file.Path); root != "" {
			addUnique(&roots, root)
		}
	}
	sort.Strings(roots)
	return roots
}

func rootFromSessionPath(path string) string {
	for _, dir := range []string{"sessions", "archived_sessions"} {
		marker := string(os.PathSeparator) + dir + string(os.PathSeparator)
		if idx := strings.Index(path, marker); idx > 0 {
			return path[:idx]
		}
	}
	return ""
}

func isArchivedFile(file session.FileRef) bool {
	marker := string(os.PathSeparator) + "archived_sessions" + string(os.PathSeparator)
	return strings.Contains(file.Path, marker)
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
