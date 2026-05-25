package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/profile"
)

// Options configures claude binary discovery.
type Options struct {
	BinaryPath string
	LookPath   func(name string) (string, error)
}

// LaunchSpec configures a claude child process launch.
type LaunchSpec struct {
	BinaryPath string
	Args       []string
	Env        []string
	Stdin      *os.File
	Stdout     *os.File
	Stderr     *os.File
}

// LocateClaude resolves the claude binary path from an explicit override or PATH.
func LocateClaude(opts Options) (string, error) {
	if opts.BinaryPath != "" {
		return opts.BinaryPath, nil
	}

	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	path, err := lookPath("claude")
	if err != nil {
		return "", fmt.Errorf("locating claude binary: %w", err)
	}
	return path, nil
}

// BuildEnv returns base with ccx-managed claude profile variables replaced.
//
//nolint:gocritic // The value signature is the public API for this package.
func BuildEnv(p contracts.Profile, base []string) []string {
	env := make([]string, 0, len(base)+2)
	for _, entry := range base {
		if isManagedEnv(entry, profile.EnvConfigDir) || isManagedEnv(entry, profile.EnvActiveProfile) {
			continue
		}
		env = append(env, entry)
	}

	env = append(
		env,
		profile.EnvConfigDir+"="+p.ConfigDir,
		profile.EnvActiveProfile+"="+p.Name,
	)
	return env
}

// Launch starts the configured child process and returns its exit code. The
// context is checked before launch; OS signals are forwarded explicitly so an
// interactive child can handle Ctrl-C/SIGTERM and choose its own exit status.
//
//nolint:gocritic // The value signature is the public API for this package.
func Launch(ctx context.Context, spec LaunchSpec) (int, error) {
	if spec.BinaryPath == "" {
		return 0, errors.New("launching claude: empty binary path")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	cmd := exec.Command(spec.BinaryPath, spec.Args...) //nolint:gosec // Launching the selected claude binary is this package's purpose.
	cmd.Env = spec.Env
	cmd.Stdin = spec.Stdin
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	cmd.Stdout = spec.Stdout
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	cmd.Stderr = spec.Stderr
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting claude: %w", err)
	}

	stopForwarding := forwardSignals(cmd.Process)
	err := cmd.Wait()
	stopForwarding()
	if err == nil {
		return cmd.ProcessState.ExitCode(), nil
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

func isManagedEnv(entry, key string) bool {
	return strings.HasPrefix(entry, key+"=")
}

func forwardSignals(process *os.Process) func() {
	signals := forwardedSignals()
	if len(signals) == 0 {
		return func() {}
	}

	signalCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(signalCh, signals...)

	go func() {
		for {
			select {
			case sig := <-signalCh:
				_ = process.Signal(sig)
			case <-done:
				return
			}
		}
	}()

	return func() {
		signal.Stop(signalCh)
		close(done)
	}
}
