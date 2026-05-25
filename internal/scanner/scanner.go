package scanner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/arafa-dev/ccx/internal/contracts"
)

// SharedCursorProfile is the profile-name sentinel used for scan cursors that
// belong to the shared-projects scan path rather than to one ccx profile.
const SharedCursorProfile = "__shared__"

// AttributedEvent is an Event plus the ccx profile that owns its Claude Code
// session.
type AttributedEvent struct {
	Event   contracts.Event
	Profile string
}

// SessionLookup resolves Claude Code session IDs to the owning ccx profile.
type SessionLookup interface {
	ProfileForSession(ctx context.Context, sessionID string) (profile string, ok bool, err error)
}

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

	go s.run(ctx, profile.Name, profile.ConfigDir, events, errs)

	return events, errs
}

// ScanShared walks a shared projects root once and emits events attributed to
// their owning ccx profile via lookup.
func (s *Scanner) ScanShared(ctx context.Context, projectsRoot string, lookup SessionLookup) (events <-chan AttributedEvent, errs <-chan error) {
	eventCh := make(chan AttributedEvent, 256)
	errCh := make(chan error, 1)

	go s.runShared(ctx, projectsRoot, lookup, eventCh, errCh)

	return eventCh, errCh
}

func (s *Scanner) run(ctx context.Context, profileName, configDir string, events chan<- contracts.Event, errs chan<- error) {
	defer close(events)
	defer close(errs)

	files, err := s.listJSONL(configDir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			errs <- err
		}
		return
	}
	if len(files) == 0 {
		return
	}

	jobs := make(chan string)
	var wg sync.WaitGroup
	for range s.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				s.processFile(ctx, profileName, path, events)
			}
		}()
	}

	for _, p := range files {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			errs <- ctx.Err()
			return
		case jobs <- p:
		}
	}
	close(jobs)
	wg.Wait()
}

func (s *Scanner) runShared(ctx context.Context, projectsRoot string, lookup SessionLookup, events chan<- AttributedEvent, errs chan<- error) {
	defer close(events)
	defer close(errs)

	files, err := s.listProjectJSONL(projectsRoot)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			errs <- err
		}
		return
	}

	for _, path := range files {
		select {
		case <-ctx.Done():
			errs <- ctx.Err()
			return
		default:
		}
		if err := s.processSharedFile(ctx, path, lookup, events); err != nil {
			errs <- err
			return
		}
	}
}

func (s *Scanner) processFile(ctx context.Context, profileName, path string, out chan<- contracts.Event) {
	cur, err := s.cursors.Get(ctx, profileName, path)
	if err != nil {
		s.logger.Warn("scanner: cursor get failed", "path", path, "err", err)
		return
	}

	project := projectNameFromDir(filepath.Base(filepath.Dir(path)))
	end, inode, err := readFile(ctx, path, project, cur, out)
	if err != nil {
		s.logger.Warn("scanner: read failed", "path", path, "err", err)
		return
	}

	if err := s.cursors.Set(ctx, profileName, path, Cursor{Offset: end, Inode: inode}); err != nil {
		s.logger.Warn("scanner: cursor set failed", "path", path, "err", err)
	}
}

func (s *Scanner) processSharedFile(ctx context.Context, path string, lookup SessionLookup, out chan<- AttributedEvent) error {
	cur, err := s.cursors.Get(ctx, SharedCursorProfile, path)
	if err != nil {
		s.logger.Warn("scanner: shared cursor get failed", "path", path, "err", err)
		return nil
	}

	project := projectNameFromDir(filepath.Base(filepath.Dir(path)))
	unknownSession := false
	end, inode, err := readFileWithEmit(ctx, path, project, cur, func(ev contracts.Event) error {
		profile, ok, err := lookup.ProfileForSession(ctx, ev.SessionID)
		if err != nil {
			return fmt.Errorf("lookup session %q: %w", ev.SessionID, err)
		}
		if !ok {
			unknownSession = true
			s.logger.Debug("scanner: shared event skipped; session owner unknown", "session_id", ev.SessionID, "path", path)
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- AttributedEvent{Event: ev, Profile: profile}:
			return nil
		}
	})
	if err != nil {
		return err
	}
	if unknownSession {
		return nil
	}

	if err := s.cursors.Set(ctx, SharedCursorProfile, path, Cursor{Offset: end, Inode: inode}); err != nil {
		s.logger.Warn("scanner: shared cursor set failed", "path", path, "err", err)
	}

	return nil
}

// listJSONL returns every <configDir>/projects/<project>/<session>.jsonl file.
// Missing configDir or projects dir returns fs.ErrNotExist.
func (s *Scanner) listJSONL(configDir string) ([]string, error) {
	projectsDir := filepath.Join(configDir, "projects")
	return s.listProjectJSONL(projectsDir)
}

func (s *Scanner) listProjectJSONL(projectsDir string) ([]string, error) {
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
