package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/headroom"
	profilepkg "github.com/arafa-dev/ccx/internal/profile"
	"github.com/arafa-dev/ccx/internal/run"
	"github.com/spf13/cobra"
)

const minimumSupervisorPollInterval = 250 * time.Millisecond

func newRunCommand(opts *Options) *cobra.Command {
	var (
		overrideProfile string
		binaryOverride  string
		printOnly       bool
		quiet           bool
		verbose         bool
		supervise       bool
		pollInterval    time.Duration
	)
	cmd := &cobra.Command{
		Use:   "run [-- args...]",
		Short: "Pick a profile and launch Claude Code",
		Args:  cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			if supervise && pollInterval > 0 && pollInterval < minimumSupervisorPollInterval {
				return fmt.Errorf("--poll-interval must be at least %s", minimumSupervisorPollInterval)
			}
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = deps.Close() }()

			profiles, err := deps.Profiles.List(ctx)
			if err != nil {
				return err
			}
			if len(profiles) == 0 {
				return fmt.Errorf("no profiles registered; run `ccx profile add` first")
			}

			scanFailures, err := runScanFailures(ctx, opts, deps, profiles, overrideProfile)
			if err != nil {
				return err
			}
			headroomStore, err := suggestHeadroomStore(deps)
			if err != nil {
				return err
			}
			evaluator := headroom.Evaluator{
				Store:   headroomStore,
				Pricing: deps.Pricing,
			}
			adapter := evaluatorAdapter{
				ev: evaluator,
				opts: headroom.Options{
					UnavailableReasons: scanFailures,
				},
			}
			profile, why, err := run.Pick(ctx, run.PickOptions{
				Profiles:  profiles,
				Override:  overrideProfile,
				Evaluator: adapter,
			})
			if err != nil {
				return err
			}

			if !quiet {
				_, _ = fmt.Fprintf(c.ErrOrStderr(), "ccx: %s -> profile=%s\n", why, profile.Name)
			}

			binary, err := run.LocateClaude(run.Options{BinaryPath: binaryOverride})
			if err != nil {
				return err
			}
			env := run.BuildEnv(profile, os.Environ())
			if printOnly {
				_, _ = fmt.Fprintf(
					c.OutOrStdout(),
					"binary=%s profile=%s %s=%s %s=%s args=%s\n",
					binary,
					profile.Name,
					profilepkg.EnvConfigDir,
					managedEnvValue(env, profilepkg.EnvConfigDir),
					profilepkg.EnvActiveProfile,
					managedEnvValue(env, profilepkg.EnvActiveProfile),
					joinRunArgs(args),
				)
				return nil
			}
			if verbose {
				_, _ = fmt.Fprintf(c.ErrOrStderr(), "ccx: launching binary=%s args=%s\n", binary, joinRunArgs(args))
			}

			if supervise {
				return runSupervisor(ctx, opts, c, deps, profiles, &profile, adapter, binary, args, pollInterval)
			}

			exitCode, err := run.Launch(ctx, run.LaunchSpec{
				BinaryPath: binary,
				Args:       args,
				Env:        env,
			})
			if err != nil {
				return err
			}
			if exitCode != 0 {
				return run.ExitCodeError{Code: exitCode}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&overrideProfile, "profile", "", "profile name to use without automatic selection")
	cmd.Flags().StringVar(&binaryOverride, "claude-binary", "", "path to claude binary")
	cmd.Flags().BoolVar(&printOnly, "print-only", false, "print planned launch without starting claude")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "suppress profile selection rationale")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "print extra launch detail")
	cmd.Flags().BoolVar(&supervise, "supervise", false, "stay attached and mid-session swap on hard pressure")
	cmd.Flags().DurationVar(&pollInterval, "poll-interval", 0, "supervisor hook polling interval (minimum 250ms)")
	return cmd
}

func runSupervisor(
	ctx context.Context,
	opts *Options,
	c *cobra.Command,
	deps *Deps,
	profiles []contracts.Profile,
	initial *contracts.Profile,
	adapter evaluatorAdapter,
	binary string,
	args []string,
	pollInterval time.Duration,
) error {
	if initial == nil {
		return errors.New("supervise requires an initial profile")
	}
	hookStore, ok := deps.Store.(run.QueryHookEventsStore)
	if !ok {
		return fmt.Errorf("supervise requires hook event queries, got %T", deps.Store)
	}
	hooks := &run.DBHookSource{
		Store:        hookStore,
		PollInterval: pollInterval,
	}
	events, eventsErr := recommendationEvents(ctx, opts)
	if eventsErr != nil {
		_, _ = fmt.Fprintf(c.ErrOrStderr(), "ccx: supervisor: recommendation stream unavailable (%v); mid-session swaps disabled\n", eventsErr)
	}
	supervisor := &run.Supervisor{
		Profiles: profiles,
		Picker: func(ctx context.Context, exclude string) (contracts.Profile, string, error) {
			return run.Pick(ctx, run.PickOptions{
				Profiles:  excludeProfile(profiles, exclude),
				Evaluator: adapter,
			})
		},
		Events:     events,
		Hooks:      hooks,
		Launcher:   run.OSChildLauncher{},
		BinaryPath: binary,
		BaseEnv:    os.Environ(),
		ResumeFlag: "--resume",
		Logger: func(format string, args ...any) {
			_, _ = fmt.Fprintf(c.ErrOrStderr(), "ccx: "+format+"\n", args...)
		},
	}
	return supervisor.Run(ctx, *initial, args)
}

func recommendationEvents(ctx context.Context, opts *Options) (<-chan contracts.RecommendationEvent, error) {
	status, err := daemonController(opts).Status(ctx)
	if err != nil || !status.Running || status.URL == "" {
		if err != nil {
			return nil, fmt.Errorf("daemon status: %w", err)
		}
		if !status.Running {
			return nil, errors.New("daemon is not running")
		}
		return nil, errors.New("daemon status did not include a URL")
	}
	events, err := run.OpenSSE(ctx, strings.TrimRight(status.URL, "/")+"/api/recommendations/live")
	if err != nil {
		return nil, err
	}
	return events, nil
}

func excludeProfile(profiles []contracts.Profile, exclude string) []contracts.Profile {
	filtered := make([]contracts.Profile, 0, len(profiles))
	for i := range profiles {
		if profiles[i].Name != exclude {
			filtered = append(filtered, profiles[i])
		}
	}
	return filtered
}

func runScanFailures(ctx context.Context, opts *Options, deps *Deps, profiles []contracts.Profile, overrideProfile string) (map[string]string, error) {
	if overrideProfile != "" {
		return map[string]string{}, nil
	}
	status, err := daemonController(opts).Status(ctx)
	if err != nil {
		return nil, err
	}
	if status.Running {
		return map[string]string{}, nil
	}
	return ingestSuggestProfiles(ctx, deps, profiles)
}

type evaluatorAdapter struct {
	ev   headroom.Evaluator
	opts headroom.Options
}

func (a evaluatorAdapter) Evaluate(ctx context.Context, profiles []contracts.Profile, _ headroom.Options) (headroom.Result, error) {
	return a.ev.Evaluate(ctx, profiles, a.opts)
}

func joinRunArgs(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		if needsRunArgQuote(arg) {
			quoted = append(quoted, strconv.Quote(arg))
			continue
		}
		quoted = append(quoted, arg)
	}
	return strings.Join(quoted, " ")
}

func needsRunArgQuote(arg string) bool {
	if arg == "" {
		return true
	}
	return strings.ContainsAny(arg, `"'\\$;&|<>(){}[]*?!`) || strings.IndexFunc(arg, unicode.IsSpace) >= 0
}

func managedEnvValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}
