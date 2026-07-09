package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rybkr/totally/internal/session"
)

func TestFilesCommandPrintsDiscoveredFiles(t *testing.T) {
	root := t.TempDir()
	path := writeRollout(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde49.jsonl")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run(context.Background(), []string{"files", "--home", root}, &stdout, &stderr)
	if err != nil {
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
	err := run(context.Background(), []string{"files", "--home", root, "--format", "json"}, &stdout, &stderr)
	if err != nil {
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
