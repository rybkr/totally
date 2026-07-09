package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rybkr/totally/internal/provider/codex"
	"github.com/rybkr/totally/internal/session"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "totally:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return nil
	}

	switch args[0] {
	case "files":
		return runFiles(ctx, args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runFiles(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) error {
	var homes stringList
	fs := flag.NewFlagSet("files", flag.ContinueOnError)
	fs.SetOutput(stderr)
	agent := fs.String("agent", string(codex.Source), "agent session format to discover")
	archived := fs.Bool("archived", false, "include archived sessions")
	format := fs.String("format", "table", "output format: table, json")
	limit := fs.Int("limit", 0, "maximum number of files to print")
	fs.Var(&homes, "home", "agent home directory; may be repeated")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	finder, err := finderForAgent(*agent)
	if err != nil {
		return err
	}

	files, err := finder.FindSessionFiles(ctx, session.FindOptions{
		Roots:           []string(homes),
		IncludeArchived: *archived,
		Limit:           *limit,
	})
	if err != nil {
		return err
	}

	switch *format {
	case "table":
		return printFilesTable(stdout, files)
	case "json":
		return json.NewEncoder(stdout).Encode(files)
	default:
		return fmt.Errorf("unknown format %q", *format)
	}
}

func finderForAgent(agent string) (session.Finder, error) {
	switch strings.ToLower(agent) {
	case "", string(codex.Source):
		return codex.NewFinder(), nil
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

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  totally files [--agent codex] [--home PATH] [--archived] [--limit N] [--format table|json]")
}

type stringList []string

func (l *stringList) String() string {
	if l == nil {
		return ""
	}
	return strings.Join(*l, ",")
}

func (l *stringList) Set(value string) error {
	if value == "" {
		return errors.New("value cannot be empty")
	}
	*l = append(*l, value)
	return nil
}
