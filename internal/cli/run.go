package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/headroom"
	profilepkg "github.com/arafa-dev/ccx/internal/profile"
	"github.com/arafa-dev/ccx/internal/run"
	"github.com/spf13/cobra"
)

func newRunCommand(_ *Options) *cobra.Command {
	var (
		overrideProfile string
		binaryOverride  string
		printOnly       bool
		quiet           bool
		verbose         bool
	)
	cmd := &cobra.Command{
		Use:   "run [-- args...]",
		Short: "Pick a profile and launch Claude Code",
		Args:  cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
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

			scanFailures, err := ingestSuggestProfiles(ctx, deps, profiles)
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
				_, _ = fmt.Fprintf(c.ErrOrStderr(), "ccx: %s -> profile=%s config_dir=%s\n", why, profile.Name, profile.ConfigDir)
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
	return cmd
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
