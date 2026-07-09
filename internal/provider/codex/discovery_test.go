package codex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rybkr/totally/internal/session"
)

func TestFinderFindsCodexRolloutFiles(t *testing.T) {
	root := t.TempDir()
	activeID := "019f44e4-5c01-7d22-9805-50cecaefde49"
	archivedID := "019f44e4-5c01-7d22-9805-50cecaefde50"

	activePath := writeFile(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+activeID+".jsonl")
	writeFile(t, root, "archived_sessions/2026/07/07/rollout-2026-07-07T20-20-44-"+archivedID+".jsonl")
	writeFile(t, root, "sessions/2026/07/08/not-a-rollout.jsonl")

	files, err := NewFinder().FindSessionFiles(context.Background(), session.FindOptions{
		Roots: []string{root},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 active file, got %d", len(files))
	}
	if files[0].Path != activePath {
		t.Fatalf("unexpected active path: %s", files[0].Path)
	}
	if files[0].Source != Source || files[0].Role != session.FileRoleTranscript {
		t.Fatalf("unexpected source/role: %s/%s", files[0].Source, files[0].Role)
	}
	if files[0].SessionID != activeID {
		t.Fatalf("unexpected session ID: %s", files[0].SessionID)
	}

	wantCreated := time.Date(2026, 7, 8, 20, 20, 44, 0, time.Local)
	if !files[0].CreatedAt.Equal(wantCreated) {
		t.Fatalf("unexpected created time: %s", files[0].CreatedAt)
	}

	withArchived, err := NewFinder().FindSessionFiles(context.Background(), session.FindOptions{
		Roots:           []string{root},
		IncludeArchived: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(withArchived) != 2 {
		t.Fatalf("expected active and archived files, got %d", len(withArchived))
	}
}

func TestFinderPrefersPlainRolloutOverCompressedSibling(t *testing.T) {
	root := t.TempDir()
	id := "019f44e4-5c01-7d22-9805-50cecaefde49"
	plain := writeFile(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+id+".jsonl")
	writeFile(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+id+".jsonl.zst")

	files, err := NewFinder().FindSessionFiles(context.Background(), session.FindOptions{
		Roots: []string{root},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Path != plain {
		t.Fatalf("expected plain path, got %s", files[0].Path)
	}
	if files[0].Compressed {
		t.Fatal("expected plain rollout to be marked uncompressed")
	}
}

func TestFinderReturnsCompressedRolloutWhenPlainIsAbsent(t *testing.T) {
	root := t.TempDir()
	id := "019f44e4-5c01-7d22-9805-50cecaefde49"
	compressed := writeFile(t, root, "sessions/2026/07/08/rollout-2026-07-08T20-20-44-"+id+".jsonl.zst")

	files, err := NewFinder().FindSessionFiles(context.Background(), session.FindOptions{
		Roots: []string{root},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Path != compressed {
		t.Fatalf("expected compressed path, got %s", files[0].Path)
	}
	if !files[0].Compressed || files[0].Format != session.FileFormatJSONLZstd {
		t.Fatalf("expected compressed jsonl.zst, got compressed=%v format=%s", files[0].Compressed, files[0].Format)
	}
}

func TestParseRolloutPathInterpretsTimestampAsLocalTime(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.FixedZone("TestLocal", -7*60*60)
	t.Cleanup(func() {
		time.Local = oldLocal
	})

	ref, _, ok := parseRolloutPath("rollout-2026-07-08T20-20-44-019f44e4-5c01-7d22-9805-50cecaefde49.jsonl")
	if !ok {
		t.Fatal("expected rollout path to parse")
	}

	want := time.Date(2026, 7, 8, 20, 20, 44, 0, time.Local)
	if !ref.CreatedAt.Equal(want) {
		t.Fatalf("got %s, want %s", ref.CreatedAt, want)
	}
}

func writeFile(t *testing.T, root string, rel string) string {
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
