package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rybkr/totally/internal/session"
)

const (
	Source session.Source = "codex"

	sessionsDir         = "sessions"
	archivedSessionsDir = "archived_sessions"
	compressedSuffix    = ".zst"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Finder discovers Codex rollout transcript files.
type Finder struct{}

func NewFinder() Finder {
	return Finder{}
}

func (Finder) Source() session.Source {
	return Source
}

func (Finder) FindSessionFiles(ctx context.Context, opts session.FindOptions) ([]session.FileRef, error) {
	roots := opts.Roots
	if len(roots) == 0 {
		root, err := DefaultHome()
		if err != nil {
			return nil, err
		}
		roots = []string{root}
	}

	found := make(map[string]session.FileRef)
	for _, root := range roots {
		if err := findRoot(ctx, root, opts.IncludeArchived, found); err != nil {
			return nil, err
		}
	}

	files := make([]session.FileRef, 0, len(found))
	for _, file := range found {
		files = append(files, file)
	}

	sort.Slice(files, func(i, j int) bool {
		if !files[i].CreatedAt.Equal(files[j].CreatedAt) {
			return files[i].CreatedAt.After(files[j].CreatedAt)
		}
		return files[i].Path > files[j].Path
	})

	if opts.Limit > 0 && len(files) > opts.Limit {
		files = files[:opts.Limit]
	}

	return files, nil
}

func DefaultHome() (string, error) {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return home, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if home == "" {
		return "", errors.New("could not resolve user home directory")
	}

	return filepath.Join(home, ".codex"), nil
}

func findRoot(ctx context.Context, root string, includeArchived bool, found map[string]session.FileRef) error {
	dirs := []string{filepath.Join(root, sessionsDir)}
	if includeArchived {
		dirs = append(dirs, filepath.Join(root, archivedSessionsDir))
	}

	for _, dir := range dirs {
		if err := walkRollouts(ctx, dir, found); err != nil {
			return err
		}
	}

	return nil
}

func walkRollouts(ctx context.Context, dir string, found map[string]session.FileRef) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}

	return filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		ref, canonicalPath, ok := parseRolloutPath(path)
		if !ok {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		ref.UpdatedAt = info.ModTime()
		ref.SizeBytes = info.Size()

		if existing, ok := found[canonicalPath]; ok && !existing.Compressed {
			return nil
		}
		found[canonicalPath] = ref
		return nil
	})
}

func parseRolloutPath(path string) (session.FileRef, string, bool) {
	name := filepath.Base(path)
	plainName := strings.TrimSuffix(name, compressedSuffix)
	compressed := plainName != name

	if !strings.HasPrefix(plainName, "rollout-") || !strings.HasSuffix(plainName, ".jsonl") {
		return session.FileRef{}, "", false
	}

	core := strings.TrimSuffix(strings.TrimPrefix(plainName, "rollout-"), ".jsonl")
	if len(core) < 37 || core[len(core)-37] != '-' {
		return session.FileRef{}, "", false
	}

	sessionID := core[len(core)-36:]
	if !uuidPattern.MatchString(sessionID) {
		return session.FileRef{}, "", false
	}

	createdAt, err := time.ParseInLocation("2006-01-02T15-04-05", core[:len(core)-37], time.Local)
	if err != nil {
		return session.FileRef{}, "", false
	}

	format := session.FileFormatJSONL
	canonicalPath := path
	if compressed {
		format = session.FileFormatJSONLZstd
		canonicalPath = strings.TrimSuffix(path, compressedSuffix)
	}

	return session.FileRef{
		Source:     Source,
		Role:       session.FileRoleTranscript,
		Format:     format,
		Path:       path,
		Compressed: compressed,
		SessionID:  sessionID,
		CreatedAt:  createdAt,
	}, canonicalPath, true
}
