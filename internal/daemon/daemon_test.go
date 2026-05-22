package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/profile"
	"github.com/arafa-dev/ccx/internal/storage"
)

func TestStatusHandlesMissingRunningAndStaleRuntimeFiles(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	proc := newFakeProcessManager()
	c := Controller{Root: root, Version: "test", Process: proc}

	missing, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("Status(missing): %v", err)
	}
	if missing.Running {
		t.Fatalf("missing status running = true")
	}
	if missing.DBPath != filepath.Join(root, "state.db") || missing.LogPath != filepath.Join(root, "daemon.log") {
		t.Fatalf("missing paths = db %q log %q", missing.DBPath, missing.LogPath)
	}

	started := time.Now().UTC().Truncate(time.Second)
	writeStatusFile(t, root, contracts.DaemonStatus{
		PID:            1234,
		Version:        "test",
		StartedAt:      started,
		Port:           7777,
		URL:            "http://127.0.0.1:7777",
		DBPath:         filepath.Join(root, "state.db"),
		LogPath:        filepath.Join(root, "daemon.log"),
		ExecutablePath: "/bin/ccx",
		StartToken:     "token-1234",
		Running:        true,
	})
	writePIDFile(t, root, 1234)
	paths := RuntimePaths(root)
	writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "token-1234", PID: 1234, CreatedAt: time.Now().UTC()})
	proc.setAlive(1234, true)

	running, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("Status(running): %v", err)
	}
	if !running.Running || running.PID != 1234 || running.URL != "http://127.0.0.1:7777" {
		t.Fatalf("running status = %+v", running)
	}

	proc.setAlive(1234, false)
	stale, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("Status(stale): %v", err)
	}
	if stale.Running || stale.PID != 1234 {
		t.Fatalf("stale status = %+v", stale)
	}

	if err := os.Remove(filepath.Join(root, "daemon.json")); err != nil {
		t.Fatal(err)
	}
	writePIDFile(t, root, 9999)
	proc.setAlive(9999, true)
	pidOnlyStarting, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("Status(pid-only live): %v", err)
	}
	if pidOnlyStarting.Running || pidOnlyStarting.URL != "" || pidOnlyStarting.PID != 9999 {
		t.Fatalf("pid-only live status = %+v", pidOnlyStarting)
	}

	proc.setAlive(9999, false)
	pidOnly, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("Status(pid-only stale): %v", err)
	}
	if pidOnly.Running || pidOnly.PID != 9999 {
		t.Fatalf("pid-only stale status = %+v", pidOnly)
	}
}

func TestStartDetachedRefusesDuplicateAndReplacesStalePID(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	proc := newFakeProcessManager()
	proc.nextPID = 4321
	c := Controller{
		Root:           root,
		Version:        "test",
		Executable:     "/bin/ccx",
		Process:        proc,
		StartupTimeout: 200 * time.Millisecond,
	}

	writeStatusFile(t, root, contracts.DaemonStatus{
		PID:            1234,
		Version:        "test",
		Port:           7777,
		URL:            "http://127.0.0.1:7777",
		ExecutablePath: "/bin/ccx",
		StartToken:     "token-1234",
		Running:        true,
	})
	writePIDFile(t, root, 1234)
	paths := RuntimePaths(root)
	writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "token-1234", PID: 1234, CreatedAt: time.Now().UTC()})
	proc.setAlive(1234, true)

	dup, err := c.StartDetached(ctx, StartOptions{Port: 7777, PollInterval: time.Minute})
	if err != nil {
		t.Fatalf("StartDetached(duplicate): %v", err)
	}
	if !dup.AlreadyRunning || dup.Started || proc.startCalls != 0 || dup.Status.PID != 1234 {
		t.Fatalf("duplicate result = %+v startCalls=%d", dup, proc.startCalls)
	}

	proc.setAlive(1234, false)
	proc.onStart = func(spec *StartProcessSpec, pid int) {
		writePIDFile(t, spec.Root, pid)
		writeStatusFile(t, spec.Root, contracts.DaemonStatus{
			PID:            pid,
			Version:        spec.Version,
			Port:           7778,
			URL:            "http://127.0.0.1:7778",
			DBPath:         filepath.Join(spec.Root, "state.db"),
			LogPath:        spec.LogPath,
			ExecutablePath: spec.Executable,
			StartToken:     spec.StartToken,
			Running:        true,
			StartedAt:      time.Now().UTC(),
		})
	}

	started, err := c.StartDetached(ctx, StartOptions{Port: 7778, PollInterval: time.Minute})
	if err != nil {
		t.Fatalf("StartDetached(stale): %v", err)
	}
	if !started.Started || started.AlreadyRunning || started.Status.PID != 4321 {
		t.Fatalf("started result = %+v", started)
	}
	if got := strings.TrimSpace(readFile(t, filepath.Join(root, "daemon.pid"))); got != "4321" {
		t.Fatalf("pid file = %q, want 4321", got)
	}
}

func TestStartDetachedReportsBusyWhenStartLockActiveAndRecoversStaleLock(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	lockPath := filepath.Join(root, "daemon.lock")
	proc := newFakeProcessManager()
	proc.nextPID = 5678
	c := Controller{
		Root:           root,
		Version:        "test",
		Executable:     "/bin/ccx",
		Process:        proc,
		StartupTimeout: 200 * time.Millisecond,
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	paths := RuntimePaths(root)
	writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "active-token", PID: 111, CreatedAt: time.Now().UTC()})

	busy, err := c.StartDetached(ctx, StartOptions{Port: 7777, PollInterval: time.Minute})
	if err == nil {
		t.Fatal("expected active start lock to fail")
	}
	if !strings.Contains(err.Error(), "daemon start already in progress") {
		t.Fatalf("error = %v", err)
	}
	if busy.Started || busy.AlreadyRunning || proc.startCalls != 0 {
		t.Fatalf("busy result = %+v startCalls=%d", busy, proc.startCalls)
	}

	old := time.Now().Add(-2 * time.Hour)
	writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "stale-token", PID: 111, CreatedAt: old})
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}
	proc.onStart = func(spec *StartProcessSpec, pid int) {
		writePIDFile(t, spec.Root, pid)
		writeStatusFile(t, spec.Root, contracts.DaemonStatus{
			PID:            pid,
			Version:        spec.Version,
			Port:           7777,
			URL:            "http://127.0.0.1:7777",
			DBPath:         filepath.Join(spec.Root, "state.db"),
			LogPath:        spec.LogPath,
			ExecutablePath: spec.Executable,
			StartToken:     spec.StartToken,
			Running:        true,
			StartedAt:      time.Now().UTC(),
		})
	}

	started, err := c.StartDetached(ctx, StartOptions{Port: 7777, PollInterval: time.Minute})
	if err != nil {
		t.Fatalf("StartDetached(stale lock): %v", err)
	}
	if !started.Started || started.Status.PID != 5678 || proc.startCalls != 1 {
		t.Fatalf("started result = %+v startCalls=%d", started, proc.startCalls)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file after successful start: %v", err)
	}
}

func TestStartDetachedOldLockWithLiveOwnerBlocks(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	proc := newFakeProcessManager()
	c := Controller{
		Root:           root,
		Version:        "test",
		Executable:     "/bin/ccx",
		Process:        proc,
		StartupTimeout: 50 * time.Millisecond,
		LockStaleAfter: time.Millisecond,
	}
	old := time.Now().Add(-2 * time.Hour)
	paths := RuntimePaths(root)
	writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "live-owner", PID: 4242, CreatedAt: old})
	proc.setAlive(4242, true)

	result, err := c.StartDetached(ctx, StartOptions{Port: 7777, PollInterval: time.Minute})
	if err == nil {
		t.Fatal("expected live old lock to block start")
	}
	if !strings.Contains(err.Error(), "daemon start already in progress") {
		t.Fatalf("error = %v", err)
	}
	if result.Started || proc.startCalls != 0 {
		t.Fatalf("result = %+v startCalls=%d, want no spawn", result, proc.startCalls)
	}
	record := readLockRecordForTest(t, &paths)
	if record.Token != "live-owner" || record.PID != 4242 {
		t.Fatalf("lock after blocked start = %+v", record)
	}
}

func TestStartDetachedTimeoutDoesNotReportPIDOnlyAsRunning(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	proc := newFakeProcessManager()
	proc.nextPID = 6543
	proc.onStart = func(spec *StartProcessSpec, pid int) {
		writePIDFile(t, spec.Root, pid)
	}
	c := Controller{
		Root:           root,
		Version:        "test",
		Executable:     "/bin/ccx",
		Process:        proc,
		StartupTimeout: 75 * time.Millisecond,
	}

	result, err := c.StartDetached(ctx, StartOptions{Port: 7777, PollInterval: time.Minute})
	if err == nil {
		t.Fatal("expected start timeout when status URL is never written")
	}
	if !strings.Contains(err.Error(), "daemon did not become ready") {
		t.Fatalf("error = %v", err)
	}
	if !result.Started || result.Status.PID != 6543 {
		t.Fatalf("result = %+v", result)
	}
	if result.Status.Running || result.Status.URL != "" {
		t.Fatalf("pid-only timeout status = %+v", result.Status)
	}
}

func TestStartDetachedPartialStartErrorKeepsLockWhileChildAlive(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	proc := newFakeProcessManager()
	proc.nextPID = 9753
	proc.startErr = errors.New("release failed")
	proc.keepAliveOnTerminate = true
	c := Controller{
		Root:           root,
		Version:        "test",
		Executable:     "/bin/ccx",
		Process:        proc,
		StartupTimeout: 50 * time.Millisecond,
		StopTimeout:    50 * time.Millisecond,
	}

	result, err := c.StartDetached(ctx, StartOptions{Port: 7777, PollInterval: time.Minute})
	if err == nil {
		t.Fatal("expected partial start error")
	}
	if result.Started {
		t.Fatalf("result = %+v, want not started", result)
	}
	if proc.terminateCalls != 1 {
		t.Fatalf("terminateCalls = %d, want 1", proc.terminateCalls)
	}
	if _, err := os.Stat(filepath.Join(root, "daemon.lock")); err != nil {
		t.Fatalf("expected lock to remain while partial child is live: %v", err)
	}
}

func TestStartDetachedSetChildPIDFailureKeepsLockWhileChildAlive(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	proc := newFakeProcessManager()
	proc.nextPID = 8642
	proc.keepAliveOnTerminate = true
	proc.skipAsyncLockAdopt = true
	restore := setSetChildPIDErrorHookForTest(errors.New("claim failed"))
	defer restore()
	c := Controller{
		Root:           root,
		Version:        "test",
		Executable:     "/bin/ccx",
		Process:        proc,
		StartupTimeout: 50 * time.Millisecond,
		StopTimeout:    50 * time.Millisecond,
	}

	first, err := c.StartDetached(ctx, StartOptions{Port: 7777, PollInterval: time.Minute})
	if err == nil {
		t.Fatal("expected setChildPID failure")
	}
	if first.Started {
		t.Fatalf("first result = %+v, want not started", first)
	}
	if proc.terminateCalls != 1 {
		t.Fatalf("terminateCalls = %d, want 1", proc.terminateCalls)
	}
	if _, err := os.Stat(filepath.Join(root, "daemon.lock")); err != nil {
		t.Fatalf("expected lock to remain while child is live: %v", err)
	}

	proc.nextPID = 9754
	second, err := c.StartDetached(ctx, StartOptions{Port: 7778, PollInterval: time.Minute})
	if err == nil {
		t.Fatal("expected preserved lock to block second start")
	}
	if !strings.Contains(err.Error(), "daemon start already in progress") {
		t.Fatalf("second error = %v", err)
	}
	if second.Started || proc.startCalls != 1 {
		t.Fatalf("second result = %+v startCalls=%d, want no second spawn", second, proc.startCalls)
	}
}

func TestStartDetachedTimeoutWithLiveChildPreservesLock(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	proc := newFakeProcessManager()
	proc.nextPID = 7654
	proc.keepAliveOnTerminate = true
	proc.onStart = func(spec *StartProcessSpec, pid int) {
		writePIDFile(t, spec.Root, pid)
	}
	c := Controller{
		Root:           root,
		Version:        "test",
		Executable:     "/bin/ccx",
		Process:        proc,
		StartupTimeout: 50 * time.Millisecond,
		StopTimeout:    50 * time.Millisecond,
	}

	first, err := c.StartDetached(ctx, StartOptions{Port: 7777, PollInterval: time.Minute})
	if err == nil {
		t.Fatal("expected first start to time out")
	}
	if !first.Started || first.Status.PID != 7654 {
		t.Fatalf("first result = %+v", first)
	}
	if _, err := os.Stat(filepath.Join(root, "daemon.lock")); err != nil {
		t.Fatalf("expected lock to remain while child is alive: %v", err)
	}

	proc.nextPID = 8765
	second, err := c.StartDetached(ctx, StartOptions{Port: 7778, PollInterval: time.Minute})
	if err == nil {
		t.Fatal("expected second start to be blocked by preserved lock")
	}
	if !strings.Contains(err.Error(), "daemon start already in progress") {
		t.Fatalf("second error = %v", err)
	}
	if second.Started || proc.startCalls != 1 {
		t.Fatalf("second result = %+v startCalls=%d", second, proc.startCalls)
	}
}

func TestStatusWithLiveNonDaemonPIDReportsNotRunning(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	proc := newFakeProcessManager()
	c := Controller{Root: root, Version: "test", Process: proc}
	writeStatusFile(t, root, contracts.DaemonStatus{
		PID:            1357,
		Version:        "test",
		Port:           7777,
		URL:            "http://127.0.0.1:7777",
		ExecutablePath: "/bin/ccx",
		Running:        true,
	})
	writePIDFile(t, root, 1357)
	proc.setAlive(1357, true)
	proc.setMatches(1357, false)

	status, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Running {
		t.Fatalf("identity mismatch status running = true: %+v", status)
	}
}

func TestStatusWithMatchingProcessButMissingLockReportsNotRunning(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	proc := newFakeProcessManager()
	c := Controller{Root: root, Version: "test", Process: proc}
	writeStatusFile(t, root, contracts.DaemonStatus{
		PID:            1360,
		Version:        "test",
		Port:           7777,
		URL:            "http://127.0.0.1:7777",
		ExecutablePath: "/bin/ccx",
		StartToken:     "status-token",
		Running:        true,
	})
	writePIDFile(t, root, 1360)
	proc.setAlive(1360, true)

	status, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Running {
		t.Fatalf("missing lock status running = true: %+v", status)
	}
}

func TestStatusWithMatchingProcessButMismatchedLockReportsNotRunning(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	proc := newFakeProcessManager()
	c := Controller{Root: root, Version: "test", Process: proc}
	writeStatusFile(t, root, contracts.DaemonStatus{
		PID:            1361,
		Version:        "test",
		Port:           7777,
		URL:            "http://127.0.0.1:7777",
		ExecutablePath: "/bin/ccx",
		StartToken:     "status-token",
		Running:        true,
	})
	writePIDFile(t, root, 1361)
	paths := RuntimePaths(root)
	writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "other-token", PID: 1361, CreatedAt: time.Now().UTC()})
	proc.setAlive(1361, true)

	status, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Running {
		t.Fatalf("mismatched lock status running = true: %+v", status)
	}
}

func TestStopWithLiveNonDaemonPIDDoesNotTerminate(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	proc := newFakeProcessManager()
	c := Controller{Root: root, Version: "test", Process: proc}
	writeStatusFile(t, root, contracts.DaemonStatus{
		PID:            2469,
		Version:        "test",
		Port:           7777,
		URL:            "http://127.0.0.1:7777",
		ExecutablePath: "/bin/ccx",
		Running:        true,
	})
	writePIDFile(t, root, 2469)
	proc.setAlive(2469, true)
	proc.setMatches(2469, false)

	stopped, err := c.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if stopped.Stopped {
		t.Fatalf("stop result = %+v, want not stopped", stopped)
	}
	if proc.terminateCalls != 0 {
		t.Fatalf("terminateCalls = %d, want 0", proc.terminateCalls)
	}
}

func TestStopWithMatchingProcessButMismatchedLockDoesNotTerminate(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	proc := newFakeProcessManager()
	c := Controller{Root: root, Version: "test", Process: proc}
	writeStatusFile(t, root, contracts.DaemonStatus{
		PID:            2470,
		Version:        "test",
		Port:           7777,
		URL:            "http://127.0.0.1:7777",
		ExecutablePath: "/bin/ccx",
		StartToken:     "status-token",
		Running:        true,
	})
	writePIDFile(t, root, 2470)
	paths := RuntimePaths(root)
	writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "other-token", PID: 2470, CreatedAt: time.Now().UTC()})
	proc.setAlive(2470, true)

	stopped, err := c.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if stopped.Stopped {
		t.Fatalf("stop result = %+v, want not stopped", stopped)
	}
	if proc.terminateCalls != 0 {
		t.Fatalf("terminateCalls = %d, want 0", proc.terminateCalls)
	}
}

func TestStopNoopsWhenAbsentAndTerminatesRunningDaemon(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	proc := newFakeProcessManager()
	c := Controller{Root: root, Version: "test", Process: proc, StopTimeout: 200 * time.Millisecond}

	absent, err := c.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop(absent): %v", err)
	}
	if absent.Stopped || proc.terminateCalls != 0 {
		t.Fatalf("absent stop = %+v terminateCalls=%d", absent, proc.terminateCalls)
	}

	writePIDFile(t, root, 2468)
	writeStatusFile(t, root, contracts.DaemonStatus{
		PID:            2468,
		Version:        "test",
		Port:           7777,
		URL:            "http://127.0.0.1:7777",
		ExecutablePath: "/bin/ccx",
		StartToken:     "token-2468",
		Running:        true,
	})
	paths := RuntimePaths(root)
	writeLockRecordForTest(t, &paths, daemonLockRecord{Token: "token-2468", PID: 2468, CreatedAt: time.Now().UTC()})
	proc.setAlive(2468, true)

	stopped, err := c.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop(running): %v", err)
	}
	if !stopped.Stopped || proc.terminateCalls != 1 {
		t.Fatalf("running stop = %+v terminateCalls=%d", stopped, proc.terminateCalls)
	}
	after, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("Status(after stop): %v", err)
	}
	if after.Running {
		t.Fatalf("after stop status = %+v", after)
	}
}

func TestForegroundRuntimeWritesStatusLogPIDAndServesHealth(t *testing.T) {
	root := t.TempDir()
	port := freeLocalPort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := runDaemonForTest(t, ctx, RunOptions{
		Root:         root,
		Version:      "test",
		Port:         port,
		PollInterval: time.Hour,
	})

	status := waitForStatus(t, root)
	if status.PID != os.Getpid() || status.Port != port || status.URL != "http://127.0.0.1:"+statusPortString(port) {
		t.Fatalf("status = %+v", status)
	}
	if got := strings.TrimSpace(readFile(t, filepath.Join(root, "daemon.pid"))); got != statusPIDString(os.Getpid()) {
		t.Fatalf("pid file = %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "daemon.log")); err != nil {
		t.Fatalf("daemon.log missing: %v", err)
	}

	res, err := http.Get(status.URL + "/api/health") //nolint:gosec,noctx // Local test server with test-owned URL.
	if err != nil {
		t.Fatalf("GET /api/health: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", res.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("daemon did not stop after context cancellation")
	}
}

func TestRuntimeInitialIngestLoadsJSONLIntoSQLite(t *testing.T) {
	root, cfgDir := setupProfileWithSession(t, "work", []sessionLine{
		{UUID: "evt-001", Tokens: 100, Timestamp: "2026-05-21T12:00:00Z"},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runDaemonForTest(t, ctx, RunOptions{
		Root:         root,
		Version:      "test",
		Port:         freeLocalPort(t),
		PollInterval: time.Hour,
	})
	waitForStatus(t, root)

	waitForUsageTotal(t, root, "work", 100)
	cancel()
	<-done
	_ = cfgDir
}

func TestWatchDebounceAppendedJSONLUpdatesUsageWithoutRestart(t *testing.T) {
	root, cfgDir := setupProfileWithSession(t, "work", []sessionLine{
		{UUID: "evt-001", Tokens: 100, Timestamp: "2026-05-21T12:00:00Z"},
	})
	sessionPath := filepath.Join(cfgDir, "projects", "sample-project", "session.jsonl")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runDaemonForTest(t, ctx, RunOptions{
		Root:         root,
		Version:      "test",
		Port:         freeLocalPort(t),
		PollInterval: time.Hour,
	})
	waitForStatus(t, root)
	waitForUsageTotal(t, root, "work", 100)
	before := waitForStatus(t, root)

	appendSessionLine(t, sessionPath, sessionLine{
		UUID: "evt-002", Tokens: 200, Timestamp: "2026-05-21T12:00:01Z",
	})

	waitForUsageTotal(t, root, "work", 300)
	after := waitForStatus(t, root)
	if after.PID != before.PID || after.StartedAt != before.StartedAt {
		t.Fatalf("daemon appears to have restarted: before=%+v after=%+v", before, after)
	}

	cancel()
	<-done
}

type fakeProcessManager struct {
	mu                   sync.Mutex
	alive                map[int]bool
	matches              map[int]bool
	nextPID              int
	startCalls           int
	terminateCalls       int
	startErr             error
	keepAliveOnTerminate bool
	skipAsyncLockAdopt   bool
	onStart              func(*StartProcessSpec, int)
}

func newFakeProcessManager() *fakeProcessManager {
	return &fakeProcessManager{alive: map[int]bool{}, matches: map[int]bool{}, nextPID: 1000}
}

func (f *fakeProcessManager) Alive(pid int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.alive[pid]
}

func (f *fakeProcessManager) Matches(pid int, _ string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	matches, ok := f.matches[pid]
	return !ok || matches
}

func (f *fakeProcessManager) StartDetached(_ context.Context, spec *StartProcessSpec) (int, error) {
	f.mu.Lock()
	f.startCalls++
	pid := f.nextPID
	f.alive[pid] = true
	onStart := f.onStart
	if f.startErr != nil {
		err := f.startErr
		f.mu.Unlock()
		return pid, err
	}
	skipAsyncLockAdopt := f.skipAsyncLockAdopt
	f.mu.Unlock()
	go func() {
		time.Sleep(5 * time.Millisecond)
		if !skipAsyncLockAdopt {
			paths := RuntimePaths(spec.Root)
			if err := writeLockRecord(paths.LockPath, daemonLockRecord{
				Token:     spec.StartToken,
				PID:       pid,
				CreatedAt: time.Now().UTC(),
			}); err != nil {
				return
			}
		}
		if onStart != nil {
			onStart(spec, pid)
		}
	}()
	return pid, nil
}

func (f *fakeProcessManager) Terminate(pid int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.terminateCalls++
	if !f.keepAliveOnTerminate {
		f.alive[pid] = false
	}
	return nil
}

func (f *fakeProcessManager) setAlive(pid int, alive bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.alive[pid] = alive
}

func (f *fakeProcessManager) setMatches(pid int, matches bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.matches[pid] = matches
}

type sessionLine struct {
	UUID      string
	Tokens    int
	Timestamp string
}

func setupProfileWithSession(t *testing.T, profileName string, lines []sessionLine) (string, string) {
	t.Helper()
	root := t.TempDir()
	cfgDir := filepath.Join(root, "claude-"+profileName)
	projectDir := filepath.Join(cfgDir, "projects", "sample-project")
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(projectDir, "session.jsonl")
	for _, line := range lines {
		appendSessionLine(t, sessionPath, line)
	}
	mgr, err := profile.NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Add(context.Background(), contracts.Profile{Name: profileName, ConfigDir: cfgDir}); err != nil {
		t.Fatal(err)
	}
	return root, cfgDir
}

func appendSessionLine(t *testing.T, path string, line sessionLine) {
	t.Helper()
	data := `{"type":"assistant","uuid":"` + line.UUID + `","sessionId":"sess-001","timestamp":"` + line.Timestamp + `","message":{"model":"claude-sonnet-4-6","usage":{"input_tokens":` + statusPIDString(line.Tokens) + `,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}` + "\n"
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // Test-owned fixture path.
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(data); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func runDaemonForTest(t *testing.T, ctx context.Context, opts RunOptions) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, opts)
	}()
	t.Cleanup(func() {
		select {
		case <-done:
		default:
		}
	})
	return done
}

func waitForStatus(t *testing.T, root string) contracts.DaemonStatus {
	t.Helper()
	var status contracts.DaemonStatus
	eventually(t, func() bool {
		data, err := os.ReadFile(filepath.Join(root, "daemon.json")) //nolint:gosec // Test-owned path.
		if err != nil {
			return false
		}
		if err := json.Unmarshal(data, &status); err != nil {
			return false
		}
		return status.Running && status.URL != ""
	})
	return status
}

func waitForUsageTotal(t *testing.T, root, profileName string, want int) {
	t.Helper()
	eventually(t, func() bool {
		ctx := context.Background()
		store, err := storage.NewStore(ctx, filepath.Join(root, "state.db"))
		if err != nil {
			return false
		}
		defer func() { _ = store.Close() }()
		rows, err := store.QueryUsage(ctx, contracts.UsageQuery{Profile: profileName})
		if err != nil {
			return false
		}
		var total int
		for _, row := range rows {
			total += row.Usage.InputTokens
		}
		return total == want
	})
}

func eventually(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !ok() {
		t.Fatal("condition not met before deadline")
	}
}

func freeLocalPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

func writeStatusFile(t *testing.T, root string, status contracts.DaemonStatus) {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "daemon.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writePIDFile(t *testing.T, root string, pid int) {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "daemon.pid"), []byte(statusPIDString(pid)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // Test-owned path.
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func statusPIDString(pid int) string {
	return strconv.Itoa(pid)
}

func statusPortString(port int) string {
	return strconv.Itoa(port)
}
