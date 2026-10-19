package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSessionTimeFiltersUseTranscriptTimestamps(t *testing.T) {
	originalLocal := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = originalLocal })

	root := t.TempDir()
	const (
		laterID   = "019f44e4-5c01-7d22-9805-50cecaefde49"
		earlierID = "019f44e4-5c01-7d22-9805-50cecaefde50"
	)
	// These filenames were written in different timezones. Their ordering in
	// the reader's timezone is the reverse of the actual session start times.
	later := time.Date(2026, 7, 9, 3, 20, 44, 0, time.UTC)
	earlier := time.Date(2026, 7, 8, 20, 20, 44, 0, time.UTC)
	writeRolloutContents(t, root, "sessions/rollout-2026-07-08T20-20-44-"+laterID+".jsonl", inspectFixtureForSessionAt(laterID, later))
	earlierContents := strings.ReplaceAll(inspectFixtureForSessionAt(earlierID, earlier), `"input_tokens":100`, `"input_tokens":200`)
	earlierContents = strings.ReplaceAll(earlierContents, `"total_tokens":125`, `"total_tokens":225`)
	writeRolloutContents(t, root, "sessions/rollout-2026-07-09T03-20-44-"+earlierID+".jsonl", earlierContents)

	for _, test := range []struct {
		name   string
		flags  []string
		id     string
		tokens int64
	}{
		{"since", []string{"--since", "2026-07-09T03:00:00Z"}, laterID, 125},
		{"until", []string{"--until", "2026-07-08T21:00:00Z"}, earlierID, 225},
		{"inclusive bounds", []string{"--since", later.Format(time.RFC3339), "--until", later.Format(time.RFC3339)}, laterID, 125},
		{"aliases", []string{"--after", "2026-07-09T03:00:00Z", "--before", "2026-07-09T04:00:00Z"}, laterID, 125},
		{"calendar date", []string{"--since", "2026-07-09", "--until", "2026-07-09"}, laterID, 125},
	} {
		for _, command := range []string{"sessions", "stats", "show"} {
			t.Run(test.name+"/"+command, func(t *testing.T) {
				var stdout, stderr bytes.Buffer
				cmd := newTestRootCommand(t, &stdout, &stderr)
				args := []string{command, "--home", root, "--format", "json"}
				if command == "show" {
					args = append(args, "--latest")
				}
				cmd.SetArgs(append(args, test.flags...))
				if err := cmd.ExecuteContext(context.Background()); err != nil {
					t.Fatalf("run %s: %v; stderr: %s", command, err, stderr.String())
				}
				switch command {
				case "sessions":
					var reports []sessionListReport
					if err := json.Unmarshal(stdout.Bytes(), &reports); err != nil {
						t.Fatal(err)
					}
					if len(reports) != 1 || reports[0].SessionID != test.id {
						t.Fatalf("expected only session %s, got %s", test.id, stdout.String())
					}
				case "stats":
					var report statsReport
					if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
						t.Fatal(err)
					}
					if report.Sessions != 1 || report.TokenUsage.TotalTokens != test.tokens {
						t.Fatalf("expected one session with %d tokens, got %s", test.tokens, stdout.String())
					}
				case "show":
					var report showReport
					if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
						t.Fatal(err)
					}
					if report.SessionID != test.id {
						t.Fatalf("expected session %s, got %s", test.id, report.SessionID)
					}
				}
			})
		}
	}
}

func TestSessionTimeFiltersFallBackToFilenameWithoutTranscriptTimestamps(t *testing.T) {
	root := t.TempDir()
	const sessionID = "019f44e4-5c01-7d22-9805-50cecaefde49"
	writeRolloutContents(t, root, "sessions/rollout-2026-07-09T03-20-44-"+sessionID+".jsonl", "{}\n")
	for _, test := range []struct {
		since string
		count int
	}{
		{"2026-07-09", 1},
		{"2026-07-10", 0},
	} {
		t.Run(test.since, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cmd := newTestRootCommand(t, &stdout, &stderr)
			cmd.SetArgs([]string{"sessions", "--home", root, "--since", test.since, "--format", "json"})
			if err := cmd.ExecuteContext(context.Background()); err != nil {
				t.Fatal(err)
			}
			var reports []sessionListReport
			if err := json.Unmarshal(stdout.Bytes(), &reports); err != nil {
				t.Fatal(err)
			}
			if len(reports) != test.count {
				t.Fatalf("expected %d sessions, got %s", test.count, stdout.String())
			}
			if test.count == 1 {
				wantCreated := time.Date(2026, 7, 9, 3, 20, 44, 0, time.Local)
				if reports[0].SessionID != sessionID || !reports[0].CreatedAt.Equal(wantCreated) {
					t.Fatalf("filename fallback was not preserved: %s", stdout.String())
				}
			}
		})
	}
}
