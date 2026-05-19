package scanner

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// Scanner implements contracts.Scanner. Create with NewScanner.
type Scanner struct {
	cursors CursorStore
	workers int
	logger  *slog.Logger
}

// NewScanner constructs a Scanner using the given CursorStore. The worker
// pool size defaults to runtime.NumCPU() (minimum 1). The logger defaults to
// slog.Default().
func NewScanner(cs CursorStore) *Scanner {
	w := runtime.NumCPU()
	if w < 1 {
		w = 1
	}
	return &Scanner{cursors: cs, workers: w, logger: slog.Default()}
}

// Scan walks <profile.ConfigDir>/projects/*/<session-uuid>.jsonl and emits
// parsed Events on the returned channel. The events channel is closed when
// scanning completes or ctx is cancelled. The errs channel reports fatal
// errors (e.g., directory traversal failures); it is also closed when done.
// Per-line parse failures are logged and skipped, not reported on errs.
//
//nolint:gocritic // contracts.Scanner requires a value Profile parameter.
func (s *Scanner) Scan(ctx context.Context, profile contracts.Profile) (<-chan contracts.Event, <-chan error) {
	events := make(chan contracts.Event, 256)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		files, err := s.listJSONL(profile.ConfigDir)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				errs <- err
			}
			return
		}

		_ = files
		_ = ctx
		// File processing is added in later tasks.
	}()

	return events, errs
}

// listJSONL returns every <configDir>/projects/<project>/<session>.jsonl file.
// Missing configDir or projects dir returns fs.ErrNotExist.
func (s *Scanner) listJSONL(configDir string) ([]string, error) {
	projectsDir := filepath.Join(configDir, "projects")
	info, err := os.Stat(projectsDir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fs.ErrNotExist
	}

	var out []string
	err = filepath.WalkDir(projectsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".jsonl" {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
