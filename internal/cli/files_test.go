package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rybkr/totally/internal/session"
	"github.com/spf13/cobra"
)

func TestFilesCommandPrintsDiscoveredFiles(t *testing.T) {
	root := t.TempDir()
	path := writeRollout(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde49.jsonl")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"files", "--home", root})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "SOURCE\tFORMAT\tSESSION\tCREATED\tSIZE\tPATH") {
		t.Fatalf("missing header in output:\n%s", output)
	}
	if !strings.Contains(output, path) {
		t.Fatalf("missing rollout path in output:\n%s", output)
	}
}

func TestFilesCommandPrintsJSON(t *testing.T) {
	root := t.TempDir()
	writeRollout(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde49.jsonl")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"files", "--home", root, "--format", "json"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	var files []session.FileRef
	if err := json.Unmarshal(stdout.Bytes(), &files); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].SessionID != "019f44e4-5c01-7d22-9805-50cecaefde49" {
		t.Fatalf("unexpected session ID: %s", files[0].SessionID)
	}
}

func TestFilesCommandDefaultsToAllAgents(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)

	flag := cmd.PersistentFlags().Lookup("agent")
	if flag == nil {
		t.Fatal("missing agent flag")
	}
	if flag.DefValue != "all" {
		t.Fatalf("expected default agent all, got %q", flag.DefValue)
	}
}

func TestFilesCommandFiltersSince(t *testing.T) {
	root := t.TempDir()
	writeRollout(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde49.jsonl")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"--since", "2026-07-09", "files", "--home", root, "--format", "json"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	var files []session.FileRef
	if err := json.Unmarshal(stdout.Bytes(), &files); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	if len(files) != 0 {
		t.Fatalf("expected no files after since filter, got %d", len(files))
	}
}

func TestFilesCommandIncludesRecentLocalRolloutWithRelativeSince(t *testing.T) {
	root := t.TempDir()
	recentTime := time.Now().Add(-50 * time.Minute)
	oldTime := time.Now().Add(-2 * time.Hour)
	recentID := "019f44e4-5c01-7d22-9805-50cecaefde49"
	oldID := "019f44e4-5c01-7d22-9805-50cecaefde50"
	writeRollout(t, root, "sessions/2026/07/08/rollout-"+recentTime.Format("2006-01-02T15-04-05")+"-"+recentID+".jsonl")
	writeRollout(t, root, "sessions/2026/07/08/rollout-"+oldTime.Format("2006-01-02T15-04-05")+"-"+oldID+".jsonl")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"--since", "1h", "files", "--home", root, "--format", "json"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	var files []session.FileRef
	if err := json.Unmarshal(stdout.Bytes(), &files); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	if len(files) != 1 || files[0].SessionID != recentID {
		t.Fatalf("expected recent local rollout only, got %+v", files)
	}
}

func TestFilesCommandRejectsInvalidFormatBeforeDiscovery(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"files", "--home", "\x00", "--format", "xml"})

	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected invalid format to fail")
	}
	if !strings.Contains(err.Error(), `unknown format "xml"`) {
		t.Fatalf("expected format error, got %v", err)
	}
}

func TestFilesCommandReadsConfigFile(t *testing.T) {
	root := t.TempDir()
	writeRollout(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde49.jsonl")
	config := writeConfig(t, fmt.Sprintf("home = [%q]\nformat = \"json\"\n", root))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	cmd.SetArgs([]string{"--config", config, "files"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	var files []session.FileRef
	if err := json.Unmarshal(stdout.Bytes(), &files); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
}

func TestFilesCommandEnvOverridesConfigFile(t *testing.T) {
	configRoot := t.TempDir()
	envRoot := t.TempDir()
	writeRollout(t, configRoot, "sessions/2026/07/07/rollout-2026-07-07T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde49.jsonl")
	writeRollout(t, envRoot, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde50.jsonl")
	config := writeConfig(t, fmt.Sprintf("home = [%q]\nformat = \"json\"\n", configRoot))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	t.Setenv("TOTALLY_HOME", envRoot)
	cmd.SetArgs([]string{"--config", config, "files"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	var files []session.FileRef
	if err := json.Unmarshal(stdout.Bytes(), &files); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	if len(files) != 1 || files[0].SessionID != "019f44e4-5c01-7d22-9805-50cecaefde50" {
		t.Fatalf("expected env home to win, got %+v", files)
	}
}

func TestFilesCommandFlagOverridesEnv(t *testing.T) {
	envRoot := t.TempDir()
	flagRoot := t.TempDir()
	writeRollout(t, envRoot, "sessions/2026/07/07/rollout-2026-07-07T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde49.jsonl")
	writeRollout(t, flagRoot, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde50.jsonl")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := newTestRootCommand(t, &stdout, &stderr)
	t.Setenv("TOTALLY_HOME", envRoot)
	t.Setenv("TOTALLY_FORMAT", "json")
	cmd.SetArgs([]string{"files", "--home", flagRoot})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	var files []session.FileRef
	if err := json.Unmarshal(stdout.Bytes(), &files); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	if len(files) != 1 || files[0].SessionID != "019f44e4-5c01-7d22-9805-50cecaefde50" {
		t.Fatalf("expected flag home to win, got %+v", files)
	}
}

func newTestRootCommand(t *testing.T, stdout *bytes.Buffer, stderr *bytes.Buffer) *cobra.Command {
	t.Helper()

	config := writeConfig(t, "")
	t.Setenv("TOTALLY_CONFIG", config)
	for _, key := range []string{"TOTALLY_AGENT", "TOTALLY_HOME", "TOTALLY_ARCHIVED", "TOTALLY_SINCE", "TOTALLY_UNTIL", "TOTALLY_FORMAT"} {
		t.Setenv(key, "")
	}

	return NewRootCommand(stdout, stderr)
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeRollout(t *testing.T, root string, rel string) string {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
