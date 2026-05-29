package run

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
)

// OSChildLauncher starts a real child process for Supervisor.
type OSChildLauncher struct{}

// Start launches spec.BinaryPath and returns a process handle the supervisor
// can terminate between turns.
//
//nolint:gocritic // ChildLauncher interface uses value specs.
func (OSChildLauncher) Start(ctx context.Context, spec LaunchSpec) (StartedProcess, error) {
	if spec.BinaryPath == "" {
		return nil, errors.New("launching claude: empty binary path")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	//nolint:gosec // Launching the selected claude binary is this package's purpose.
	cmd := exec.Command(spec.BinaryPath, spec.Args...)
	cmd.Env = spec.Env
	applyStdio(cmd, &spec)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting claude: %w", err)
	}

	return &startedCommand{
		cmd:            cmd,
		stopForwarding: forwardSignals(cmd.Process),
	}, nil
}

type startedCommand struct {
	cmd            *exec.Cmd
	stopForwarding func()
	stopOnce       sync.Once
}

func (p *startedCommand) SignalTerminate() error {
	if p.cmd.Process == nil {
		return nil
	}
	return signalTerminateProcess(p.cmd.Process)
}

func (p *startedCommand) Kill() error {
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

func (p *startedCommand) Wait() (int, error) {
	err := p.cmd.Wait()
	p.stopOnce.Do(p.stopForwarding)
	if err == nil {
		return p.cmd.ProcessState.ExitCode(), nil
	}
	if code, ok := signaledExitCode(err); ok {
		return code, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 0, fmt.Errorf("waiting for claude: %w", err)
}
