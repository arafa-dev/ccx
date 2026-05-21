package daemon

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/fsnotify/fsnotify"
)

const debounceInterval = 500 * time.Millisecond

type profileWatcher struct {
	deps         *runtimeDeps
	logger       *log.Logger
	fs           *fsnotify.Watcher
	pollInterval time.Duration

	mu           sync.Mutex
	scanMu       sync.Mutex
	profiles     map[string]contracts.Profile
	projectsDirs map[string]string
	watched      map[string]struct{}
	timers       map[string]*time.Timer
}

func runProfileWatcher(ctx context.Context, deps *runtimeDeps, logger *log.Logger, pollInterval time.Duration) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Printf("watcher unavailable: %v", err)
		runPollingFallback(ctx, deps, logger, pollInterval)
		return
	}
	defer func() { _ = fsWatcher.Close() }()

	w := &profileWatcher{
		deps:         deps,
		logger:       logger,
		fs:           fsWatcher,
		pollInterval: pollInterval,
		profiles:     map[string]contracts.Profile{},
		projectsDirs: map[string]string{},
		watched:      map[string]struct{}{},
		timers:       map[string]*time.Timer{},
	}
	w.refreshProfiles(ctx)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	defer w.stopTimers()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-fsWatcher.Events:
			if !ok {
				return
			}
			w.handleEvent(ctx, event)
		case err, ok := <-fsWatcher.Errors:
			if !ok {
				return
			}
			logger.Printf("watcher error: %v", err)
		case <-ticker.C:
			w.refreshProfiles(ctx)
			go w.scanAll(ctx)
		}
	}
}

func runPollingFallback(ctx context.Context, deps *runtimeDeps, logger *log.Logger, pollInterval time.Duration) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := ingestAllProfiles(ctx, deps); err != nil {
				logger.Printf("poll ingest failed: %v", err)
			}
		}
	}
}

func (w *profileWatcher) refreshProfiles(ctx context.Context) {
	profiles, err := w.deps.Profiles.List(ctx)
	if err != nil {
		w.logger.Printf("list profiles for watcher: %v", err)
		return
	}
	w.mu.Lock()
	for i := range profiles {
		p := profiles[i]
		w.profiles[p.Name] = p
		w.projectsDirs[p.Name] = filepath.Clean(filepath.Join(p.ConfigDir, "projects"))
	}
	w.mu.Unlock()
	for i := range profiles {
		w.addProfileWatches(profiles[i])
	}
}

func (w *profileWatcher) addProfileWatches(p contracts.Profile) { //nolint:gocritic // Profile is a contract value.
	projectsDir := filepath.Join(p.ConfigDir, "projects")
	_ = w.addWatch(projectsDir)
	_ = filepath.WalkDir(projectsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			w.logger.Printf("watch walk %s: %v", path, err)
			return nil
		}
		if d.IsDir() {
			_ = w.addWatch(path)
		}
		return nil
	})
}

func (w *profileWatcher) addWatch(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return err
	}
	clean := filepath.Clean(path)
	w.mu.Lock()
	if _, ok := w.watched[clean]; ok {
		w.mu.Unlock()
		return nil
	}
	w.mu.Unlock()
	if err := w.fs.Add(clean); err != nil {
		w.logger.Printf("watch add %s: %v", clean, err)
		return err
	}
	w.mu.Lock()
	w.watched[clean] = struct{}{}
	w.mu.Unlock()
	return nil
}

func (w *profileWatcher) handleEvent(ctx context.Context, event fsnotify.Event) {
	if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) == 0 {
		return
	}
	if event.Op&(fsnotify.Create|fsnotify.Rename) != 0 {
		_ = w.addWatch(event.Name)
	}
	profileName := w.profileForPath(event.Name)
	if profileName == "" {
		return
	}
	w.schedule(ctx, profileName)
}

func (w *profileWatcher) profileForPath(path string) string {
	clean := filepath.Clean(path)
	w.mu.Lock()
	defer w.mu.Unlock()
	for name, root := range w.projectsDirs {
		if clean == root || strings.HasPrefix(clean, root+string(filepath.Separator)) {
			return name
		}
	}
	return ""
}

func (w *profileWatcher) schedule(ctx context.Context, profileName string) {
	w.mu.Lock()
	if timer, ok := w.timers[profileName]; ok {
		timer.Reset(debounceInterval)
		w.mu.Unlock()
		return
	}
	w.timers[profileName] = time.AfterFunc(debounceInterval, func() {
		w.mu.Lock()
		delete(w.timers, profileName)
		w.mu.Unlock()
		w.scanProfile(ctx, profileName)
	})
	w.mu.Unlock()
}

func (w *profileWatcher) scanAll(ctx context.Context) {
	w.scanMu.Lock()
	defer w.scanMu.Unlock()
	if _, err := ingestAllProfiles(ctx, w.deps); err != nil {
		w.logger.Printf("poll ingest failed: %v", err)
	}
}

func (w *profileWatcher) scanProfile(ctx context.Context, profileName string) {
	w.mu.Lock()
	p, ok := w.profiles[profileName]
	w.mu.Unlock()
	if !ok {
		w.refreshProfiles(ctx)
		w.mu.Lock()
		p, ok = w.profiles[profileName]
		w.mu.Unlock()
		if !ok {
			return
		}
	}
	w.scanMu.Lock()
	defer w.scanMu.Unlock()
	if err := ingestProfile(ctx, w.deps, p); err != nil {
		w.logger.Printf("watch ingest profile=%s failed: %v", profileName, err)
	}
}

func (w *profileWatcher) stopTimers() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, timer := range w.timers {
		timer.Stop()
	}
	w.timers = map[string]*time.Timer{}
}
