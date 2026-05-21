package daemon

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/dashboard"
	"github.com/arafa-dev/ccx/internal/platform"
	"github.com/arafa-dev/ccx/internal/server"
)

// RunOptions configures a foreground daemon runtime.
type RunOptions struct {
	Root         string
	Version      string
	Port         int
	PollInterval time.Duration
	OnStatus     func(contracts.DaemonStatus)
}

// Run starts the foreground daemon and blocks until ctx or an OS termination
// signal stops it.
func Run(ctx context.Context, opts RunOptions) error {
	if opts.Port != 0 && (opts.Port < minPort || opts.Port > maxPort) {
		return fmt.Errorf("invalid --port %d: must be in range 1-65535", opts.Port)
	}
	root := opts.Root
	var err error
	if root == "" {
		root, err = platform.CCXHome()
		if err != nil {
			return err
		}
	}
	paths := RuntimePaths(root)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create daemon root: %w", err)
	}
	var lock *daemonLock
	if os.Getenv(envLockHeldByParent) == "1" {
		lock, err = adoptDaemonLock(&paths)
	} else {
		lock, err = acquireDaemonLock(&paths, defaultLockStaleAfter)
	}
	if err != nil {
		return err
	}
	defer lock.release()

	logFile, err := os.OpenFile(paths.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // path is controlled by ccx home.
	if err != nil {
		return fmt.Errorf("open daemon log: %w", err)
	}
	defer func() { _ = logFile.Close() }()
	logger := log.New(logFile, "", log.LstdFlags|log.LUTC)
	logger.Printf("daemon starting root=%s", root)

	runCtx, stopSignals := signalContext(ctx)
	defer stopSignals()

	pid := os.Getpid()
	if err := writePID(&paths, pid); err != nil {
		return err
	}
	startedAt := time.Now().UTC()
	defer func() {
		removePIDIf(&paths, pid)
		status, ok, readErr := readStatus(&paths)
		if readErr != nil || !ok || status.PID != pid {
			return
		}
		status.Running = false
		_ = writeStatus(&paths, &status)
		logger.Printf("daemon stopped")
	}()

	deps, err := buildRuntimeDeps(runCtx, root)
	if err != nil {
		return err
	}
	defer func() { _ = deps.Close() }()

	profiles, err := ingestAllProfiles(runCtx, deps)
	if err != nil {
		return fmt.Errorf("initial ingest: %w", err)
	}

	webFS, err := dashboard.FS()
	if err != nil {
		return fmt.Errorf("dashboard assets: %w", err)
	}
	srv := server.New(server.Deps{
		Store:    deps.Store,
		Pricing:  deps.Pricing,
		Profiles: deps.Profiles,
		WebRoot:  webFS,
	}, opts.Version)

	startPort, endPort := defaultStartPort, defaultEndPort
	if opts.Port != 0 {
		startPort, endPort = opts.Port, opts.Port
	}
	boundPort, runServer, err := srv.Serve(runCtx, startPort, endPort)
	if err != nil {
		return err
	}
	status := contracts.DaemonStatus{
		PID:             pid,
		Version:         opts.Version,
		StartedAt:       startedAt,
		Port:            boundPort,
		URL:             fmt.Sprintf("http://127.0.0.1:%d", boundPort),
		DBPath:          paths.DBPath,
		LogPath:         paths.LogPath,
		ProfilesWatched: len(profiles),
		Running:         true,
	}
	if err := writeStatus(&paths, &status); err != nil {
		return err
	}
	if opts.OnStatus != nil {
		opts.OnStatus(status)
	}
	logger.Printf("daemon listening url=%s profiles=%d", status.URL, status.ProfilesWatched)

	poll := opts.PollInterval
	if poll <= 0 {
		poll = defaultPollInterval
	}
	go runProfileWatcher(runCtx, deps, logger, poll)

	if err := runServer(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
