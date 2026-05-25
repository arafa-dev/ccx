package run

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
)

const defaultShutdownGrace = 5 * time.Second

// HookSource provides the hook telemetry reads the supervisor needs.
type HookSource interface {
	CurrentSessionID(ctx context.Context, profile string) (string, error)
	WaitForStop(ctx context.Context, sessionID string) error
}

// StartedProcess is a running child process controlled by the supervisor.
type StartedProcess interface {
	SignalTerminate() error
	Kill() error
	Wait() (int, error)
}

// ChildLauncher starts a Claude Code child process.
type ChildLauncher interface {
	Start(ctx context.Context, spec LaunchSpec) (StartedProcess, error)
}

// Supervisor runs Claude Code and swaps to a sibling profile after a hard
// recommendation event reaches a turn boundary.
type Supervisor struct {
	Profiles      []contracts.Profile
	Picker        func(ctx context.Context, exclude string) (contracts.Profile, string, error)
	Events        <-chan contracts.RecommendationEvent
	Hooks         HookSource
	Launcher      ChildLauncher
	BinaryPath    string
	BaseEnv       []string
	ResumeFlag    string
	Logger        func(format string, args ...any)
	ShutdownGrace time.Duration
}

// Run starts Claude Code under initial and supervises pressure-driven swaps.
func (s *Supervisor) Run(ctx context.Context, initial contracts.Profile, args []string) error { //nolint:gocritic // Profile is a value-style contract type.
	if s.Launcher == nil {
		return errors.New("supervisor launcher is nil")
	}
	current := initial
	runArgs := append([]string(nil), args...)
	for {
		s.markLaunch(current.Name, time.Now().UTC())
		child, err := s.Launcher.Start(ctx, LaunchSpec{
			BinaryPath: s.BinaryPath,
			Args:       append([]string(nil), runArgs...),
			Env:        BuildEnv(current, s.BaseEnv),
		})
		if err != nil {
			return fmt.Errorf("starting claude under %s: %w", current.Name, err)
		}
		launchDone := make(chan processResult, 1)
		go func() {
			exit, err := child.Wait()
			launchDone <- processResult{exit: exit, err: err}
		}()

		swap := false
		for !swap {
			select {
			case <-ctx.Done():
				if err := s.shutdown(context.Background(), child, launchDone); err != nil {
					return err
				}
				return ctx.Err()
			case result := <-launchDone:
				return result.asError()
			case ev, ok := <-s.Events:
				if !ok {
					s.Events = nil
					continue
				}
				if ev.Profile == current.Name && ev.Level == contracts.RecommendationHard {
					swap = true
				}
			}
		}

		sessionID, err := s.currentSessionID(ctx, current.Name)
		if err != nil {
			if shutdownErr := s.shutdown(context.Background(), child, launchDone); shutdownErr != nil {
				return shutdownErr
			}
			return err
		}
		if sessionID != "" {
			waitCtx, cancelWait := context.WithCancel(ctx)
			waitDone := make(chan error, 1)
			go func() {
				waitDone <- s.Hooks.WaitForStop(waitCtx, sessionID)
			}()
			select {
			case result := <-launchDone:
				cancelWait()
				return result.asError()
			case err := <-waitDone:
				cancelWait()
				if err != nil {
					if shutdownErr := s.shutdown(context.Background(), child, launchDone); shutdownErr != nil {
						return shutdownErr
					}
					return err
				}
			case <-ctx.Done():
				cancelWait()
				if shutdownErr := s.shutdown(context.Background(), child, launchDone); shutdownErr != nil {
					return shutdownErr
				}
				return ctx.Err()
			}
		}
		if err := s.shutdown(ctx, child, launchDone); err != nil {
			return err
		}

		next, why, err := s.pick(ctx, current.Name)
		if err != nil {
			s.log("supervisor: cannot pick sibling after hard event on %s: %v", current.Name, err)
			return err
		}
		s.log("supervisor: swapping %s -> %s (%s)", current.Name, next.Name, why)
		if sessionID != "" {
			resumeFlag := s.ResumeFlag
			if resumeFlag == "" {
				resumeFlag = "--resume"
			}
			runArgs = appendResumeFlag(runArgs, resumeFlag, sessionID)
		}
		current = next
	}
}

type processResult struct {
	exit int
	err  error
}

func (r processResult) asError() error {
	if r.err != nil {
		return r.err
	}
	if r.exit != 0 {
		return ExitCodeError{Code: r.exit}
	}
	return nil
}

type launchMarker interface {
	MarkLaunch(profile string, at time.Time)
}

func (s *Supervisor) markLaunch(profile string, at time.Time) {
	if marker, ok := s.Hooks.(launchMarker); ok {
		marker.MarkLaunch(profile, at)
	}
}

func (s *Supervisor) currentSessionID(ctx context.Context, profile string) (string, error) {
	if s.Hooks == nil {
		return "", nil
	}
	return s.Hooks.CurrentSessionID(ctx, profile)
}

func (s *Supervisor) pick(ctx context.Context, exclude string) (contracts.Profile, string, error) {
	if s.Picker == nil {
		return contracts.Profile{}, "", fmt.Errorf("supervisor picker is nil: %w", ErrNoRecommendation)
	}
	return s.Picker(ctx, exclude)
}

func (s *Supervisor) shutdown(ctx context.Context, child StartedProcess, launchDone <-chan processResult) error {
	grace := s.ShutdownGrace
	if grace <= 0 {
		grace = defaultShutdownGrace
	}
	_ = child.SignalTerminate()
	select {
	case <-launchDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(grace):
		s.log("supervisor: terminate grace elapsed; killing child")
		if err := child.Kill(); err != nil {
			return fmt.Errorf("killing child: %w", err)
		}
	}
	select {
	case <-launchDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(grace):
		return errors.New("child did not exit after kill")
	}
}

func (s *Supervisor) log(format string, args ...any) {
	if s.Logger != nil {
		s.Logger(format, args...)
	}
}

func appendResumeFlag(args []string, flag, sessionID string) []string {
	aliases := resumeFlagAliases(flag)
	out := make([]string, 0, len(args)+2)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case isResumeFlag(arg, aliases):
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
			}
		case isResumeFlagAssignment(arg, aliases):
			continue
		default:
			out = append(out, arg)
		}
	}
	return append([]string{flag, sessionID}, out...)
}

func resumeFlagAliases(flag string) []string {
	if flag == "--resume" || flag == "-r" {
		return []string{"--resume", "-r"}
	}
	return []string{flag}
}

func isResumeFlag(arg string, aliases []string) bool {
	for _, alias := range aliases {
		if arg == alias {
			return true
		}
	}
	return false
}

func isResumeFlagAssignment(arg string, aliases []string) bool {
	for _, alias := range aliases {
		if strings.HasPrefix(arg, alias+"=") {
			return true
		}
	}
	return false
}
