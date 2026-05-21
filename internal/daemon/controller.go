package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/platform"
)

const (
	defaultStartPort    = 7777
	defaultEndPort      = 7787
	defaultPollInterval = 60 * time.Second
	defaultStartupWait  = 10 * time.Second
	defaultStopWait     = 5 * time.Second
	statusPollInterval  = 50 * time.Millisecond
	minPort             = 1
	maxPort             = 65535
)

// ProcessManager abstracts process operations for lifecycle tests.
type ProcessManager interface {
	Alive(pid int) bool
	StartDetached(ctx context.Context, spec *StartProcessSpec) (int, error)
	Terminate(pid int) error
}

// StartProcessSpec describes the foreground daemon child used by detached
// start. Tests use Root/Version to synthesize the child status file.
type StartProcessSpec struct {
	Root       string
	Version    string
	Executable string
	Args       []string
	Env        []string
	LogPath    string
}

// StartOptions controls daemon start behavior.
type StartOptions struct {
	Port         int
	PollInterval time.Duration
}

// StartResult is returned by StartDetached.
type StartResult struct {
	Status         contracts.DaemonStatus `json:"status"`
	Started        bool                   `json:"started"`
	AlreadyRunning bool                   `json:"already_running"`
}

// StopResult is returned by Stop.
type StopResult struct {
	Status  contracts.DaemonStatus `json:"status"`
	Stopped bool                   `json:"stopped"`
}

// Controller owns daemon lifecycle operations for one ccx root.
type Controller struct {
	Root           string
	Version        string
	Executable     string
	Process        ProcessManager
	StartupTimeout time.Duration
	StopTimeout    time.Duration
}

// Status reads daemon runtime files and verifies the recorded process is alive.
func (c *Controller) Status(_ context.Context) (contracts.DaemonStatus, error) {
	root, err := c.root()
	if err != nil {
		return contracts.DaemonStatus{}, err
	}
	paths := RuntimePaths(root)
	status, _, err := readStatus(&paths)
	if err != nil {
		return contracts.DaemonStatus{}, err
	}
	if pid, ok, err := readPID(&paths); err != nil {
		return contracts.DaemonStatus{}, err
	} else if ok && status.PID == 0 {
		status.PID = pid
	}
	c.fillStatusDefaults(&paths, &status)
	status.Running = status.PID > 0 && c.process().Alive(status.PID)
	return status, nil
}

// StartDetached starts a foreground daemon child unless one is already alive.
func (c *Controller) StartDetached(ctx context.Context, opts StartOptions) (StartResult, error) {
	if opts.Port != 0 && (opts.Port < minPort || opts.Port > maxPort) {
		return StartResult{}, fmt.Errorf("invalid --port %d: must be in range 1-65535", opts.Port)
	}
	root, err := c.root()
	if err != nil {
		return StartResult{}, err
	}
	paths := RuntimePaths(root)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return StartResult{}, fmt.Errorf("create daemon root: %w", err)
	}

	status, err := c.Status(ctx)
	if err != nil {
		return StartResult{}, err
	}
	if status.Running {
		return StartResult{Status: status, AlreadyRunning: true}, nil
	}
	removeRuntimeState(&paths)

	exe := c.Executable
	if exe == "" {
		exe, err = os.Executable()
		if err != nil {
			return StartResult{}, fmt.Errorf("locate executable: %w", err)
		}
	}
	poll := opts.PollInterval
	if poll <= 0 {
		poll = defaultPollInterval
	}
	args := []string{"daemon", "start", "--foreground", "--poll-interval", poll.String()}
	if opts.Port != 0 {
		args = append(args, "--port", fmt.Sprintf("%d", opts.Port))
	}
	spec := &StartProcessSpec{
		Root:       root,
		Version:    c.Version,
		Executable: exe,
		Args:       args,
		Env:        os.Environ(),
		LogPath:    paths.LogPath,
	}
	pid, err := c.process().StartDetached(ctx, spec)
	if err != nil {
		return StartResult{}, fmt.Errorf("start detached daemon: %w", err)
	}

	status, err = c.waitForReadyStatus(ctx, pid)
	if err != nil {
		return StartResult{Status: status, Started: true}, err
	}
	return StartResult{Status: status, Started: true}, nil
}

// Stop gracefully terminates a running daemon and marks stale state stopped.
func (c *Controller) Stop(ctx context.Context) (StopResult, error) {
	status, err := c.Status(ctx)
	if err != nil {
		return StopResult{}, err
	}
	root, err := c.root()
	if err != nil {
		return StopResult{}, err
	}
	paths := RuntimePaths(root)
	if !status.Running {
		if status.PID != 0 {
			status.Running = false
			c.fillStatusDefaults(&paths, &status)
			if err := writeStatus(&paths, &status); err != nil {
				return StopResult{}, err
			}
			removePIDIf(&paths, status.PID)
		}
		return StopResult{Status: status}, nil
	}

	if err := c.process().Terminate(status.PID); err != nil {
		return StopResult{}, fmt.Errorf("terminate daemon pid %d: %w", status.PID, err)
	}
	deadline := time.Now().Add(c.stopTimeout())
	for time.Now().Before(deadline) {
		if !c.process().Alive(status.PID) {
			status.Running = false
			c.fillStatusDefaults(&paths, &status)
			if err := writeStatus(&paths, &status); err != nil {
				return StopResult{}, err
			}
			removePIDIf(&paths, status.PID)
			return StopResult{Status: status, Stopped: true}, nil
		}
		select {
		case <-ctx.Done():
			return StopResult{}, ctx.Err()
		case <-time.After(statusPollInterval):
		}
	}
	return StopResult{}, fmt.Errorf("daemon pid %d did not stop within %s", status.PID, c.stopTimeout())
}

// Restart stops any running daemon and starts a detached one.
func (c *Controller) Restart(ctx context.Context, opts StartOptions) (StartResult, error) {
	if _, err := c.Stop(ctx); err != nil {
		return StartResult{}, err
	}
	return c.StartDetached(ctx, opts)
}

func (c *Controller) waitForReadyStatus(ctx context.Context, pid int) (contracts.DaemonStatus, error) {
	timeout := c.StartupTimeout
	if timeout <= 0 {
		timeout = defaultStartupWait
	}
	deadline := time.Now().Add(timeout)
	var last contracts.DaemonStatus
	for time.Now().Before(deadline) {
		status, err := c.Status(ctx)
		if err == nil {
			last = status
			if status.PID == pid && status.Running && status.URL != "" {
				return status, nil
			}
		}
		if !c.process().Alive(pid) {
			if last.PID == 0 {
				last.PID = pid
			}
			last.Running = false
			return last, errors.New("daemon process exited before becoming ready")
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(statusPollInterval):
		}
	}
	return last, fmt.Errorf("daemon did not become ready within %s", timeout)
}

func (c *Controller) root() (string, error) {
	if c.Root != "" {
		return c.Root, nil
	}
	return platform.CCXHome()
}

func (c *Controller) process() ProcessManager {
	if c.Process != nil {
		return c.Process
	}
	return osProcessManager{}
}

func (c *Controller) stopTimeout() time.Duration {
	if c.StopTimeout > 0 {
		return c.StopTimeout
	}
	return defaultStopWait
}

func (c *Controller) fillStatusDefaults(paths *Paths, status *contracts.DaemonStatus) {
	if status.Version == "" {
		status.Version = c.Version
	}
	if status.DBPath == "" {
		status.DBPath = paths.DBPath
	}
	if status.LogPath == "" {
		status.LogPath = paths.LogPath
	}
	if status.Port != 0 && status.URL == "" {
		status.URL = fmt.Sprintf("http://127.0.0.1:%d", status.Port)
	}
}

type osProcessManager struct{}

func (osProcessManager) Alive(pid int) bool {
	return platform.ProcessAlive(pid)
}

func (osProcessManager) StartDetached(ctx context.Context, spec *StartProcessSpec) (int, error) {
	return platform.StartDetachedProcess(ctx, &platform.DetachedProcessSpec{
		Executable: spec.Executable,
		Args:       spec.Args,
		Env:        spec.Env,
		LogPath:    spec.LogPath,
	})
}

func (osProcessManager) Terminate(pid int) error {
	return platform.TerminateProcess(pid)
}
