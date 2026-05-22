package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
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
	Matches(pid int, expectedExecutable string) bool
	Identity(pid int) (string, bool)
	StartDetached(ctx context.Context, spec *StartProcessSpec) (int, error)
	Terminate(pid int) error
}

// StartProcessSpec describes the foreground daemon child used by detached
// start. Tests use Root/Version to synthesize the child status file.
type StartProcessSpec struct {
	Root       string
	Version    string
	StartToken string
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
	LockStaleAfter time.Duration
}

// Status reads daemon runtime files and verifies the recorded process is alive.
func (c *Controller) Status(_ context.Context) (contracts.DaemonStatus, error) {
	root, err := c.root()
	if err != nil {
		return contracts.DaemonStatus{}, err
	}
	paths := RuntimePaths(root)
	status, hasStatus, err := readStatus(&paths)
	if err != nil {
		return contracts.DaemonStatus{}, err
	}
	if pid, ok, err := readPID(&paths); err != nil {
		return contracts.DaemonStatus{}, err
	} else if ok && status.PID == 0 {
		status.PID = pid
	}
	c.fillStatusDefaults(&paths, &status)
	status.Running = hasStatus && status.PID > 0 && status.URL != "" && c.processOwns(status.PID, status.ProcessIdentity)
	if status.Running {
		expectedExecutable, err := c.expectedExecutable(status.ExecutablePath)
		if err != nil || expectedExecutable == "" || !c.process().Matches(status.PID, expectedExecutable) {
			status.Running = false
		}
	}
	if status.Running && !statusLockMatches(&paths, &status) {
		status.Running = false
	}
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
	c.removeAbandonedStatusRuntime(&paths, &status)

	token, err := newLockToken()
	if err != nil {
		return StartResult{}, err
	}
	lock, err := acquireDaemonLock(&paths, c.lockStaleAfter(), token, c.processIdentity(os.Getpid()), c.lockOwnerAlive)
	if err != nil {
		status, statusErr := c.Status(ctx)
		if statusErr != nil {
			return StartResult{}, statusErr
		}
		if status.Running {
			return StartResult{Status: status, AlreadyRunning: true}, nil
		}
		return StartResult{Status: status}, err
	}
	releaseLock := true
	defer func() {
		if releaseLock {
			lock.release()
		}
	}()

	status, err = c.Status(ctx)
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
		StartToken: token,
		Executable: exe,
		Args:       args,
		Env:        append(os.Environ(), envLockHeldByParent+"=1", envLockToken+"="+token, envLockParentPID+"="+strconv.Itoa(os.Getpid())),
		LogPath:    paths.LogPath,
	}
	pid, err := c.process().StartDetached(ctx, spec)
	if err != nil {
		if pid > 0 {
			childIdentity := c.processIdentity(pid)
			_ = lock.setChildPID(pid, childIdentity)
			c.terminateSpawnedChildOrPreserveLock(ctx, pid, childIdentity, lock, &releaseLock)
		}
		return StartResult{}, fmt.Errorf("start detached daemon: %w", err)
	}
	childIdentity := c.processIdentity(pid)
	if err := lock.setChildPID(pid, childIdentity); err != nil {
		c.terminateSpawnedChildOrPreserveLock(ctx, pid, childIdentity, lock, &releaseLock)
		return StartResult{}, err
	}

	status, err = c.waitForReadyStatus(ctx, pid)
	if err != nil {
		if status.PID == 0 {
			status.PID = pid
		}
		c.terminateSpawnedChildOrPreserveLock(ctx, pid, childIdentity, lock, &releaseLock)
		return StartResult{Status: status, Started: true}, err
	}
	lock.disown()
	releaseLock = false
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
			removeLockIfOwner(&paths, status.StartToken, status.PID, status.ProcessIdentity)
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
			removeLockIfOwner(&paths, status.StartToken, status.PID, status.ProcessIdentity)
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

func (c *Controller) waitForProcessDeath(ctx context.Context, pid int, identity string) bool {
	deadline := time.Now().Add(c.stopTimeout())
	for time.Now().Before(deadline) {
		if !c.processOwns(pid, identity) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(statusPollInterval):
		}
	}
	return !c.processOwns(pid, identity)
}

func (c *Controller) terminateSpawnedChildOrPreserveLock(ctx context.Context, pid int, identity string, lock *daemonLock, releaseLock *bool) {
	if !c.processOwns(pid, identity) {
		lock.releaseChildPID(pid, identity)
		return
	}
	_ = c.process().Terminate(pid)
	if !c.waitForProcessDeath(ctx, pid, identity) {
		lock.disown()
		*releaseLock = false
		return
	}
	lock.releaseChildPID(pid, identity)
}

func statusLockMatches(paths *Paths, status *contracts.DaemonStatus) bool {
	if status.StartToken == "" || status.PID <= 0 {
		return false
	}
	record, _, err := readLockRecord(paths.LockPath)
	if err != nil {
		return false
	}
	return record.Token == status.StartToken &&
		record.PID == status.PID &&
		status.ProcessIdentity != "" &&
		record.ProcessIdentity == status.ProcessIdentity
}

func (c *Controller) removeAbandonedStatusRuntime(paths *Paths, status *contracts.DaemonStatus) {
	if status.PID <= 0 || c.processOwns(status.PID, status.ProcessIdentity) {
		return
	}
	removePIDIf(paths, status.PID)
	removeLockIfOwner(paths, status.StartToken, status.PID, status.ProcessIdentity)
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

func (c *Controller) lockStaleAfter() time.Duration {
	if c.LockStaleAfter > 0 {
		return c.LockStaleAfter
	}
	return defaultLockStaleAfter
}

func (c *Controller) expectedExecutable(statusExecutable string) (string, error) {
	if statusExecutable != "" {
		return statusExecutable, nil
	}
	if c.Executable != "" {
		return c.Executable, nil
	}
	return os.Executable()
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

func (c *Controller) processIdentity(pid int) string {
	identity, ok := c.process().Identity(pid)
	if !ok {
		return ""
	}
	return identity
}

func (c *Controller) processOwns(pid int, identity string) bool {
	if identity == "" || !c.process().Alive(pid) {
		return false
	}
	got, ok := c.process().Identity(pid)
	return ok && got == identity
}

func (c *Controller) lockOwnerAlive(record *daemonLockRecord) bool {
	if record == nil {
		return false
	}
	if record.PID > 0 && c.processOwns(record.PID, record.ProcessIdentity) {
		return true
	}
	return record.ChildPID > 0 && c.processOwns(record.ChildPID, record.ChildProcessIdentity)
}

type osProcessManager struct{}

func (osProcessManager) Alive(pid int) bool {
	return platform.ProcessAlive(pid)
}

func (osProcessManager) Matches(pid int, expectedExecutable string) bool {
	return platform.ProcessMatches(pid, expectedExecutable)
}

func (osProcessManager) Identity(pid int) (string, bool) {
	return platform.ProcessIdentity(pid)
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
