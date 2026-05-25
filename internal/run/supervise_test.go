package run_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/run"
)

func TestSupervisorSwapsAfterHardEventAndResumesSession(t *testing.T) {
	events := make(chan contracts.RecommendationEvent, 1)
	events <- contracts.RecommendationEvent{Profile: "work", Level: contracts.RecommendationHard}

	launcher := newFakeChildLauncher([]*fakeStartedProcess{
		newFakeStartedProcess(false),
		newFakeStartedProcess(true),
	})
	hooks := &fakeHookSource{sessions: map[string]string{"work": "sid-1"}}
	picker := func(context.Context, string) (contracts.Profile, string, error) {
		return contracts.Profile{Name: "personal", ConfigDir: "/profiles/personal"}, "test pick", nil
	}

	supervisor := run.Supervisor{
		Profiles:      []contracts.Profile{{Name: "work"}, {Name: "personal"}},
		Picker:        picker,
		Events:        events,
		Hooks:         hooks,
		Launcher:      launcher,
		BinaryPath:    "/bin/claude",
		BaseEnv:       []string{"PATH=/bin", "CLAUDE_CONFIG_DIR=/old"},
		ShutdownGrace: time.Second,
	}

	if err := supervisor.Run(context.Background(), contracts.Profile{Name: "work", ConfigDir: "/profiles/work"}, []string{"--model", "sonnet"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	calls := launcher.callsSnapshot()
	if len(calls) != 2 {
		t.Fatalf("launch calls = %d, want 2", len(calls))
	}
	if got := envValue(calls[1].Env, "CLAUDE_CONFIG_DIR"); got != "/profiles/personal" {
		t.Fatalf("second launch CLAUDE_CONFIG_DIR = %q, want /profiles/personal", got)
	}
	if !slices.Contains(calls[1].Args, "--resume") || !slices.Contains(calls[1].Args, "sid-1") {
		t.Fatalf("second launch args = %v, want --resume sid-1", calls[1].Args)
	}
	if hooks.waitedFor[0] != "sid-1" {
		t.Fatalf("WaitForStop sessions = %v, want sid-1", hooks.waitedFor)
	}
	if !launcher.processes[0].terminated {
		t.Fatal("first child was not gracefully terminated")
	}
}

func TestSupervisorNoHardEventReturnsChildExit(t *testing.T) {
	launcher := newFakeChildLauncher([]*fakeStartedProcess{newFakeStartedProcess(true)})
	pickerCalled := false
	supervisor := run.Supervisor{
		Picker: func(context.Context, string) (contracts.Profile, string, error) {
			pickerCalled = true
			return contracts.Profile{}, "", nil
		},
		Events:        nil,
		Hooks:         &fakeHookSource{},
		Launcher:      launcher,
		BinaryPath:    "/bin/claude",
		ShutdownGrace: time.Second,
	}

	if err := supervisor.Run(context.Background(), contracts.Profile{Name: "work", ConfigDir: "/profiles/work"}, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if pickerCalled {
		t.Fatal("picker called without a hard event")
	}
	if got := len(launcher.callsSnapshot()); got != 1 {
		t.Fatalf("launch calls = %d, want 1", got)
	}
}

func TestSupervisorReturnsPickerErrorAfterHardEvent(t *testing.T) {
	events := make(chan contracts.RecommendationEvent, 1)
	events <- contracts.RecommendationEvent{Profile: "work", Level: contracts.RecommendationHard}
	wantErr := run.ErrNoRecommendation
	launcher := newFakeChildLauncher([]*fakeStartedProcess{newFakeStartedProcess(false)})
	supervisor := run.Supervisor{
		Picker: func(context.Context, string) (contracts.Profile, string, error) {
			return contracts.Profile{}, "", wantErr
		},
		Events:        events,
		Hooks:         &fakeHookSource{sessions: map[string]string{"work": "sid-1"}},
		Launcher:      launcher,
		BinaryPath:    "/bin/claude",
		ShutdownGrace: time.Second,
	}

	err := supervisor.Run(context.Background(), contracts.Profile{Name: "work", ConfigDir: "/profiles/work"}, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run error = %v, want %v", err, wantErr)
	}
	if got := len(launcher.callsSnapshot()); got != 1 {
		t.Fatalf("launch calls = %d, want 1", got)
	}
	if !launcher.processes[0].terminated {
		t.Fatal("child was not terminated before picker error returned")
	}
}

func TestSupervisorCanSwapMoreThanOnce(t *testing.T) {
	events := make(chan contracts.RecommendationEvent, 2)
	events <- contracts.RecommendationEvent{Profile: "work", Level: contracts.RecommendationHard}
	events <- contracts.RecommendationEvent{Profile: "personal", Level: contracts.RecommendationHard}

	launcher := newFakeChildLauncher([]*fakeStartedProcess{
		newFakeStartedProcess(false),
		newFakeStartedProcess(false),
		newFakeStartedProcess(true),
	})
	picks := map[string]contracts.Profile{
		"work":     {Name: "personal", ConfigDir: "/profiles/personal"},
		"personal": {Name: "side", ConfigDir: "/profiles/side"},
	}
	hooks := &fakeHookSource{sessions: map[string]string{"work": "sid-1", "personal": "sid-2"}}
	supervisor := run.Supervisor{
		Picker: func(_ context.Context, exclude string) (contracts.Profile, string, error) {
			return picks[exclude], "test pick", nil
		},
		Events:        events,
		Hooks:         hooks,
		Launcher:      launcher,
		BinaryPath:    "/bin/claude",
		BaseEnv:       []string{"PATH=/bin"},
		ShutdownGrace: time.Second,
	}

	if err := supervisor.Run(context.Background(), contracts.Profile{Name: "work", ConfigDir: "/profiles/work"}, []string{"--verbose"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	calls := launcher.callsSnapshot()
	if len(calls) != 3 {
		t.Fatalf("launch calls = %d, want 3", len(calls))
	}
	if got := envValue(calls[2].Env, "CLAUDE_CONFIG_DIR"); got != "/profiles/side" {
		t.Fatalf("third launch CLAUDE_CONFIG_DIR = %q, want /profiles/side", got)
	}
	if !slices.Contains(calls[2].Args, "sid-2") {
		t.Fatalf("third launch args = %v, want resumed sid-2", calls[2].Args)
	}
}

func TestSupervisorReturnsChildExitWhileWaitingForStop(t *testing.T) {
	events := make(chan contracts.RecommendationEvent, 1)
	events <- contracts.RecommendationEvent{Profile: "work", Level: contracts.RecommendationHard}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	launcher := newFakeChildLauncher([]*fakeStartedProcess{newFakeStartedProcessExit(7)})
	hooks := &fakeHookSource{
		sessions: map[string]string{"work": "sid-1"},
		waitFunc: func(ctx context.Context, _ string) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
	supervisor := run.Supervisor{
		Picker:        func(context.Context, string) (contracts.Profile, string, error) { return contracts.Profile{}, "", nil },
		Events:        events,
		Hooks:         hooks,
		Launcher:      launcher,
		BinaryPath:    "/bin/claude",
		ShutdownGrace: time.Second,
	}

	err := supervisor.Run(ctx, contracts.Profile{Name: "work", ConfigDir: "/profiles/work"}, nil)
	var exitErr run.ExitCodeError
	if !errors.As(err, &exitErr) || exitErr.Code != 7 {
		t.Fatalf("Run error = %v, want exit 7 while waiting for Stop", err)
	}
}

func TestSupervisorShutdownReturnsWhenChildIgnoresTerminateAndKill(t *testing.T) {
	events := make(chan contracts.RecommendationEvent, 1)
	events <- contracts.RecommendationEvent{Profile: "work", Level: contracts.RecommendationHard}

	launcher := newFakeChildLauncher([]*fakeStartedProcess{newFakeHungProcess()})
	supervisor := run.Supervisor{
		Events:        events,
		Hooks:         &fakeHookSource{},
		Launcher:      launcher,
		BinaryPath:    "/bin/claude",
		ShutdownGrace: 5 * time.Millisecond,
	}

	result := make(chan error, 1)
	go func() {
		result <- supervisor.Run(context.Background(), contracts.Profile{Name: "work", ConfigDir: "/profiles/work"}, nil)
	}()

	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "did not exit") {
			t.Fatalf("Run error = %v, want did not exit error", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Run hung after child ignored terminate and kill")
	}
}

func TestSupervisorCancelWhileWaitingForStopKillsStubbornChild(t *testing.T) {
	events := make(chan contracts.RecommendationEvent, 1)
	events <- contracts.RecommendationEvent{Profile: "work", Level: contracts.RecommendationHard}
	waitStarted := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	launcher := newFakeChildLauncher([]*fakeStartedProcess{newFakeHungProcess()})
	supervisor := run.Supervisor{
		Events: events,
		Hooks: &fakeHookSource{
			sessions: map[string]string{"work": "sid-1"},
			waitFunc: func(ctx context.Context, _ string) error {
				close(waitStarted)
				<-ctx.Done()
				return ctx.Err()
			},
		},
		Launcher:      launcher,
		BinaryPath:    "/bin/claude",
		ShutdownGrace: 5 * time.Millisecond,
	}

	result := make(chan error, 1)
	go func() {
		result <- supervisor.Run(ctx, contracts.Profile{Name: "work", ConfigDir: "/profiles/work"}, nil)
	}()
	<-waitStarted
	cancel()

	select {
	case <-result:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Run hung after cancellation while waiting for Stop")
	}
	if !launcher.processes[0].killed {
		t.Fatal("stubborn child was not killed after cancellation")
	}
}

func TestSupervisorReplacesMalformedResumeArgs(t *testing.T) {
	events := make(chan contracts.RecommendationEvent, 1)
	events <- contracts.RecommendationEvent{Profile: "work", Level: contracts.RecommendationHard}

	launcher := newFakeChildLauncher([]*fakeStartedProcess{
		newFakeStartedProcess(false),
		newFakeStartedProcess(true),
	})
	supervisor := run.Supervisor{
		Picker: func(context.Context, string) (contracts.Profile, string, error) {
			return contracts.Profile{Name: "personal", ConfigDir: "/profiles/personal"}, "test pick", nil
		},
		Events:        events,
		Hooks:         &fakeHookSource{sessions: map[string]string{"work": "sid-1"}},
		Launcher:      launcher,
		BinaryPath:    "/bin/claude",
		ShutdownGrace: time.Second,
	}

	if err := supervisor.Run(context.Background(), contracts.Profile{Name: "work", ConfigDir: "/profiles/work"}, []string{"--resume", "--resume=old", "--model", "sonnet"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := launcher.callsSnapshot()[1].Args
	want := []string{"--resume", "sid-1", "--model", "sonnet"}
	if !slices.Equal(got, want) {
		t.Fatalf("resume args = %v, want %v", got, want)
	}
}

func TestSupervisorStripsResumeAliasBeforeRelaunch(t *testing.T) {
	events := make(chan contracts.RecommendationEvent, 1)
	events <- contracts.RecommendationEvent{Profile: "work", Level: contracts.RecommendationHard}

	launcher := newFakeChildLauncher([]*fakeStartedProcess{
		newFakeStartedProcess(false),
		newFakeStartedProcess(true),
	})
	supervisor := run.Supervisor{
		Picker: func(context.Context, string) (contracts.Profile, string, error) {
			return contracts.Profile{Name: "personal", ConfigDir: "/profiles/personal"}, "test pick", nil
		},
		Events:        events,
		Hooks:         &fakeHookSource{sessions: map[string]string{"work": "sid-1"}},
		Launcher:      launcher,
		BinaryPath:    "/bin/claude",
		ShutdownGrace: time.Second,
	}

	if err := supervisor.Run(context.Background(), contracts.Profile{Name: "work", ConfigDir: "/profiles/work"}, []string{"-r", "old", "-r=older", "--model", "sonnet"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := launcher.callsSnapshot()[1].Args
	want := []string{"--resume", "sid-1", "--model", "sonnet"}
	if !slices.Equal(got, want) {
		t.Fatalf("resume args = %v, want %v", got, want)
	}
}

type fakeHookSource struct {
	sessions  map[string]string
	waitedFor []string
	waitFunc  func(context.Context, string) error
}

func (h *fakeHookSource) CurrentSessionID(_ context.Context, profile string) (string, error) {
	return h.sessions[profile], nil
}

func (h *fakeHookSource) WaitForStop(ctx context.Context, sessionID string) error {
	if h.waitFunc != nil {
		return h.waitFunc(ctx, sessionID)
	}
	h.waitedFor = append(h.waitedFor, sessionID)
	return nil
}

type fakeChildLauncher struct {
	mu        sync.Mutex
	processes []*fakeStartedProcess
	calls     []run.LaunchSpec
}

func newFakeChildLauncher(processes []*fakeStartedProcess) *fakeChildLauncher {
	return &fakeChildLauncher{processes: processes}
}

func (l *fakeChildLauncher) Start(_ context.Context, spec run.LaunchSpec) (run.StartedProcess, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, spec)
	if len(l.calls) > len(l.processes) {
		return nil, errors.New("unexpected launch")
	}
	return l.processes[len(l.calls)-1], nil
}

func (l *fakeChildLauncher) callsSnapshot() []run.LaunchSpec {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]run.LaunchSpec(nil), l.calls...)
}

type fakeStartedProcess struct {
	exit          chan int
	terminated    bool
	killed        bool
	ignoreSignals bool
}

func newFakeStartedProcess(autoExit bool) *fakeStartedProcess {
	p := &fakeStartedProcess{exit: make(chan int, 1)}
	if autoExit {
		p.exit <- 0
	}
	return p
}

func newFakeStartedProcessExit(code int) *fakeStartedProcess {
	p := &fakeStartedProcess{exit: make(chan int, 1)}
	p.exit <- code
	return p
}

func newFakeHungProcess() *fakeStartedProcess {
	return &fakeStartedProcess{exit: make(chan int), ignoreSignals: true}
}

func (p *fakeStartedProcess) SignalTerminate() error {
	p.terminated = true
	if p.ignoreSignals {
		return nil
	}
	select {
	case p.exit <- 0:
	default:
	}
	return nil
}

func (p *fakeStartedProcess) Kill() error {
	p.killed = true
	if p.ignoreSignals {
		return nil
	}
	select {
	case p.exit <- 137:
	default:
	}
	return nil
}

func (p *fakeStartedProcess) Wait() (int, error) {
	return <-p.exit, nil
}

func envValue(env []string, key string) string {
	for _, entry := range env {
		if len(entry) > len(key) && entry[:len(key)+1] == key+"=" {
			return entry[len(key)+1:]
		}
	}
	return ""
}
