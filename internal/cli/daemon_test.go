package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/daemon"
)

func TestDaemonCLIJSONStatusStartStop(t *testing.T) {
	root := t.TempDir()
	proc := newCLIFakeProcess(root)

	statusOut, statusErr, code := runCLIWithOptions(Options{
		Args:          []string{"daemon", "status", "--json"},
		Build:         BuildInfo{Version: "test"},
		DaemonRoot:    root,
		DaemonProcess: proc,
	})
	if code != 0 {
		t.Fatalf("status exit=%d stderr=%q", code, statusErr)
	}
	var statusPayload struct {
		Status contracts.DaemonStatus `json:"status"`
	}
	if err := json.Unmarshal([]byte(statusOut), &statusPayload); err != nil {
		t.Fatalf("status json: %v\n%s", err, statusOut)
	}
	if statusPayload.Status.Running {
		t.Fatalf("missing daemon reported running: %+v", statusPayload.Status)
	}

	startOut, startErr, code := runCLIWithOptions(Options{
		Args:          []string{"daemon", "start", "--json", "--port", "7780"},
		Build:         BuildInfo{Version: "test"},
		DaemonRoot:    root,
		DaemonProcess: proc,
		Executable:    "/bin/ccx",
	})
	if code != 0 {
		t.Fatalf("start exit=%d stderr=%q", code, startErr)
	}
	var startPayload struct {
		Status  contracts.DaemonStatus `json:"status"`
		Started bool                   `json:"started"`
	}
	if err := json.Unmarshal([]byte(startOut), &startPayload); err != nil {
		t.Fatalf("start json: %v\n%s", err, startOut)
	}
	if !startPayload.Started || !startPayload.Status.Running || startPayload.Status.Port != 7780 {
		t.Fatalf("start payload = %+v", startPayload)
	}

	stopOut, stopErr, code := runCLIWithOptions(Options{
		Args:          []string{"daemon", "stop", "--json"},
		Build:         BuildInfo{Version: "test"},
		DaemonRoot:    root,
		DaemonProcess: proc,
	})
	if code != 0 {
		t.Fatalf("stop exit=%d stderr=%q", code, stopErr)
	}
	var stopPayload struct {
		Status  contracts.DaemonStatus `json:"status"`
		Stopped bool                   `json:"stopped"`
	}
	if err := json.Unmarshal([]byte(stopOut), &stopPayload); err != nil {
		t.Fatalf("stop json: %v\n%s", err, stopOut)
	}
	if !stopPayload.Stopped || stopPayload.Status.Running {
		t.Fatalf("stop payload = %+v", stopPayload)
	}
}

func TestDaemonCLIJSONErrorsAreStructured(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, code := runCLIWithOptions(Options{
		Args:          []string{"daemon", "start", "--json", "--port", "70000"},
		Build:         BuildInfo{Version: "test"},
		DaemonRoot:    root,
		DaemonProcess: newCLIFakeProcess(root),
	})
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &payload); err != nil {
		t.Fatalf("stderr is not structured JSON: %v\n%s", err, stderr)
	}
	if !strings.Contains(payload.Error, "invalid --port 70000") {
		t.Fatalf("error payload = %+v", payload)
	}
}

func TestDashboardUsesRunningDaemonURLAndExits(t *testing.T) {
	root := t.TempDir()
	proc := newCLIFakeProcess(root)
	proc.setAlive(2468, true)
	writeDaemonRuntime(t, root, contracts.DaemonStatus{
		PID:     2468,
		Version: "test",
		Port:    7781,
		URL:     "http://127.0.0.1:7781",
		Running: true,
	})
	var opened string

	out, stderr, code := runCLIWithOptions(Options{
		Args:          []string{"dashboard"},
		Build:         BuildInfo{Version: "test"},
		DaemonRoot:    root,
		DaemonProcess: proc,
		OpenBrowser: func(url string) error {
			opened = url
			return nil
		},
	})
	if code != 0 {
		t.Fatalf("dashboard exit=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(out, "http://127.0.0.1:7781") {
		t.Fatalf("dashboard output = %q", out)
	}
	if opened != "http://127.0.0.1:7781" {
		t.Fatalf("opened = %q", opened)
	}
}

func TestDashboardDaemonStartsDaemonWhenAbsent(t *testing.T) {
	root := t.TempDir()
	proc := newCLIFakeProcess(root)
	var opened string

	out, stderr, code := runCLIWithOptions(Options{
		Args:          []string{"dashboard", "--daemon", "--no-open", "--port", "7782"},
		Build:         BuildInfo{Version: "test"},
		DaemonRoot:    root,
		DaemonProcess: proc,
		Executable:    "/bin/ccx",
		OpenBrowser: func(url string) error {
			opened = url
			return nil
		},
	})
	if code != 0 {
		t.Fatalf("dashboard --daemon exit=%d stderr=%q", code, stderr)
	}
	if proc.startCalls != 1 {
		t.Fatalf("startCalls = %d, want 1", proc.startCalls)
	}
	if !strings.Contains(out, "http://127.0.0.1:7782") {
		t.Fatalf("dashboard --daemon output = %q", out)
	}
	if opened != "" {
		t.Fatalf("--no-open still opened %q", opened)
	}
}

func TestDashboardDaemonTreatsPIDOnlyDaemonAsNotReady(t *testing.T) {
	root := t.TempDir()
	proc := newCLIFakeProcess(root)
	proc.setAlive(2469, true)
	paths := daemon.RuntimePaths(root)
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.PIDPath, []byte("2469\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, stderr, code := runCLIWithOptions(Options{
		Args:          []string{"dashboard", "--daemon", "--no-open", "--port", "7783"},
		Build:         BuildInfo{Version: "test"},
		DaemonRoot:    root,
		DaemonProcess: proc,
		Executable:    "/bin/ccx",
	})
	if code != 0 {
		t.Fatalf("dashboard --daemon exit=%d stderr=%q", code, stderr)
	}
	if proc.startCalls != 1 {
		t.Fatalf("startCalls = %d, want 1", proc.startCalls)
	}
	if strings.Contains(out, "ccx dashboard at \n") || !strings.Contains(out, "http://127.0.0.1:7783") {
		t.Fatalf("dashboard --daemon output = %q", out)
	}
}

func runCLIWithOptions(opts Options) (string, string, int) {
	var stdout, stderr bytes.Buffer
	opts.Stdout = &stdout
	opts.Stderr = &stderr
	code := Run(context.Background(), opts)
	return stdout.String(), stderr.String(), code
}

type cliFakeProcess struct {
	mu             sync.Mutex
	root           string
	alive          map[int]bool
	nextPID        int
	startCalls     int
	terminateCalls int
}

func newCLIFakeProcess(root string) *cliFakeProcess {
	return &cliFakeProcess{root: root, alive: map[int]bool{}, nextPID: 9001}
}

func (f *cliFakeProcess) Alive(pid int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.alive[pid]
}

func (f *cliFakeProcess) Matches(int, string) bool {
	return true
}

func (f *cliFakeProcess) StartDetached(_ context.Context, spec *daemon.StartProcessSpec) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCalls++
	pid := f.nextPID
	f.alive[pid] = true
	port := 7777
	for i, arg := range spec.Args {
		if arg == "--port" && i+1 < len(spec.Args) {
			if parsed, err := strconv.Atoi(spec.Args[i+1]); err == nil {
				port = parsed
			}
		}
	}
	writeDaemonRuntimeLocked(spec.Root, contracts.DaemonStatus{
		PID:             pid,
		Version:         spec.Version,
		StartedAt:       time.Now().UTC(),
		Port:            port,
		URL:             "http://127.0.0.1:" + strconv.Itoa(port),
		DBPath:          filepath.Join(spec.Root, "state.db"),
		LogPath:         spec.LogPath,
		ProfilesWatched: 0,
		Running:         true,
	})
	return pid, nil
}

func (f *cliFakeProcess) Terminate(pid int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.terminateCalls++
	f.alive[pid] = false
	return nil
}

func (f *cliFakeProcess) setAlive(pid int, alive bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.alive[pid] = alive
}

func writeDaemonRuntime(t *testing.T, root string, status contracts.DaemonStatus) {
	t.Helper()
	writeDaemonRuntimeLocked(root, status)
}

func writeDaemonRuntimeLocked(root string, status contracts.DaemonStatus) {
	paths := daemon.RuntimePaths(root)
	_ = os.MkdirAll(root, 0o700)
	data, _ := json.Marshal(status)
	_ = os.WriteFile(paths.StatusPath, data, 0o600)
	_ = os.WriteFile(paths.PIDPath, []byte(strconv.Itoa(status.PID)+"\n"), 0o600)
}
