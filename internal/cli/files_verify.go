package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rybkr/totally/internal/provider/codex"
	"github.com/rybkr/totally/internal/session"
	"github.com/spf13/cobra"
)

type filesVerifyReport struct {
	Valid        bool               `json:"valid"`
	FilesChecked int                `json:"files_checked"`
	Issues       []filesVerifyIssue `json:"issues,omitempty"`
}

type filesVerifyIssue struct {
	Path      string `json:"path"`
	SessionID string `json:"session_id,omitempty"`
	Location  string `json:"location,omitempty"`
	Field     string `json:"field,omitempty"`
	Message   string `json:"message"`
}

func newFilesVerifyCommand(stdout io.Writer, globals *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "verify [PATH ...]",
		Short: "Validate raw session transcripts",
		Long:  "Validate raw session transcripts. With no paths, verifies discovered transcripts; paths verify those files directly.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, paths []string) error {
			return runFilesVerify(cmd, stdout, *globals, paths)
		},
	}
}

func runFilesVerify(cmd *cobra.Command, stdout io.Writer, globals globalOptions, paths []string) error {
	files, err := filesForVerification(cmd, globals, paths)
	if err != nil {
		return err
	}
	parsers, err := globals.parsers()
	if err != nil {
		return err
	}
	report := filesVerifyReport{Valid: true, FilesChecked: len(files)}
	for _, file := range files {
		parser, err := parserForSource(parsers, file.Source)
		if err != nil {
			report.Issues = append(report.Issues, filesVerifyIssue{Path: file.Path, SessionID: file.SessionID, Message: err.Error()})
			continue
		}
		record, err := parser.ParseSession(cmd.Context(), file)
		if err != nil {
			report.Issues = append(report.Issues, filesVerifyIssue{Path: file.Path, SessionID: file.SessionID, Message: fmt.Sprintf("cannot parse transcript: %v", err)})
			continue
		}
		appendTokenUsageIssues(&report.Issues, file.Path, record.SessionID, "token_usage", record.TokenUsage)
		for i, segment := range record.UsageSegments {
			appendTokenUsageIssues(&report.Issues, file.Path, record.SessionID, fmt.Sprintf("usage_segments[%d].token_usage", i), segment.TokenUsage)
		}
	}
	sort.Slice(report.Issues, func(i, j int) bool {
		if report.Issues[i].Path != report.Issues[j].Path {
			return report.Issues[i].Path < report.Issues[j].Path
		}
		if report.Issues[i].Location != report.Issues[j].Location {
			return report.Issues[i].Location < report.Issues[j].Location
		}
		return report.Issues[i].Field < report.Issues[j].Field
	})
	report.Valid = len(report.Issues) == 0
	if globals.format == outputFormatJSON {
		if err := json.NewEncoder(stdout).Encode(report); err != nil {
			return err
		}
	} else if report.Valid {
		if _, err := fmt.Fprintf(stdout, "Verified %d transcript(s): valid.\n", report.FilesChecked); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(stdout, "Transcript verification failed (%d issue(s) across %d transcript(s)).\n", len(report.Issues), report.FilesChecked); err != nil {
			return err
		}
		for _, issue := range report.Issues {
			if _, err := fmt.Fprintf(stdout, "\n  %s\n    %s%s: %s\n", issue.Path, issueLocation(issue), issueField(issue), issue.Message); err != nil {
				return err
			}
		}
	}
	if !report.Valid {
		return fmt.Errorf("transcript verification failed")
	}
	return nil
}

func appendTokenUsageIssues(issues *[]filesVerifyIssue, path, sessionID, location string, usage session.TokenUsage) {
	for _, issue := range session.ValidateTokenUsage(usage) {
		*issues = append(*issues, filesVerifyIssue{Path: path, SessionID: sessionID, Location: location, Field: issue.Field, Message: issue.Message})
	}
}

func issueLocation(issue filesVerifyIssue) string {
	if issue.Location == "" {
		return ""
	}
	return issue.Location
}

func issueField(issue filesVerifyIssue) string {
	if issue.Field == "" {
		return ""
	}
	return "." + issue.Field
}

func filesForVerification(cmd *cobra.Command, globals globalOptions, paths []string) ([]session.FileRef, error) {
	if len(paths) == 0 {
		return discoverSessionFiles(cmd, globals)
	}
	files := make([]session.FileRef, 0, len(paths))
	for _, path := range paths {
		file, err := explicitVerificationFile(path)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

func explicitVerificationFile(path string) (session.FileRef, error) {
	info, err := os.Stat(path)
	if err != nil {
		return session.FileRef{}, err
	}
	if !info.Mode().IsRegular() {
		return session.FileRef{}, fmt.Errorf("%s is not a regular file", path)
	}
	format := session.FileFormatJSONL
	compressed := false
	switch {
	case strings.HasSuffix(path, ".jsonl"):
	case strings.HasSuffix(path, ".jsonl.zst"):
		format, compressed = session.FileFormatJSONLZstd, true
	default:
		return session.FileRef{}, fmt.Errorf("unsupported transcript file %q: expected .jsonl or .jsonl.zst", path)
	}
	return session.FileRef{Source: codex.Source, Role: session.FileRoleTranscript, Format: format, Path: filepath.Clean(path), Compressed: compressed, UpdatedAt: info.ModTime(), SizeBytes: info.Size()}, nil
}
