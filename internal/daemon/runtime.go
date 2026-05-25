package daemon

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/dashboard"
	"github.com/arafa-dev/ccx/internal/headroom"
	"github.com/arafa-dev/ccx/internal/hooks"
	"github.com/arafa-dev/ccx/internal/platform"
	"github.com/arafa-dev/ccx/internal/quotawire"
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
	processIdentity, ok := platform.ProcessIdentity(os.Getpid())
	if !ok {
		return fmt.Errorf("determine daemon process identity")
	}
	if os.Getenv(envLockHeldByParent) == "1" {
		parentPID, _ := strconv.Atoi(os.Getenv(envLockParentPID))
		lock, err = adoptDaemonLockWithRetry(ctx, &paths, os.Getenv(envLockToken), parentPID, os.Getpid(), processIdentity, defaultStartupWait)
	} else {
		lock, err = acquireDaemonLock(&paths, defaultLockStaleAfter, "", processIdentity, platformLockOwnerAlive)
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
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}

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
	statusProvider := &currentStatusProvider{}
	srv := server.New(server.Deps{
		Store:    deps.Store,
		Pricing:  deps.Pricing,
		Profiles: deps.Profiles,
		WebRoot:  webFS,
		Daemon:   statusProvider,
		Hooks:    &hooks.Service{Profiles: deps.Profiles},
		Headroom: headroom.Evaluator{Store: deps.Store, Pricing: deps.Pricing},
		Quota:    &quotawire.Adapter{Store: deps.Store, Profiles: deps.Profiles},
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
		ExecutablePath:  exe,
		StartToken:      lock.token,
		ProcessIdentity: processIdentity,
		ProfilesWatched: len(profiles),
		Running:         true,
	}
	if err := writeStatus(&paths, &status); err != nil {
		return err
	}
	statusProvider.Set(&status)
	if opts.OnStatus != nil {
		opts.OnStatus(status)
	}
	logger.Printf("daemon listening url=%s profiles=%d", status.URL, status.ProfilesWatched)

	poll := opts.PollInterval
	if poll <= 0 {
		poll = defaultPollInterval
	}
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		runProfileWatcher(runCtx, deps, logger, poll)
	}()

	err = runServer()
	stopSignals()
	<-watcherDone
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

type currentStatusProvider struct {
	mu     sync.RWMutex
	status contracts.DaemonStatus
}

func platformLockOwnerAlive(record *daemonLockRecord) bool {
	if record == nil {
		return false
	}
	if record.PID > 0 && platformProcessOwns(record.PID, record.ProcessIdentity) {
		return true
	}
	return record.ChildPID > 0 && platformProcessOwns(record.ChildPID, record.ChildProcessIdentity)
}

func platformProcessOwns(pid int, identity string) bool {
	if identity == "" || !platform.ProcessAlive(pid) {
		return false
	}
	got, ok := platform.ProcessIdentity(pid)
	return ok && got == identity
}

func (p *currentStatusProvider) Set(status *contracts.DaemonStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = *status
}

func (p *currentStatusProvider) Status(context.Context) (contracts.DaemonStatus, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status, nil
}
