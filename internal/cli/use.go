package cli

import (
	"fmt"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/platform"
	"github.com/spf13/cobra"
)

func newUseCommand(_ *Options) *cobra.Command {
	var shellOverride string
	cmd := &cobra.Command{
		Use:   "use [name]",
		Short: "Activate a profile in the current shell",
		Long: `Prints shell commands that, when eval'd, switch the active profile.

  eval "$(ccx use work)"

If <name> is omitted, opens an interactive picker.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = deps.Close() }()

			var p contracts.Profile
			if len(args) == 1 {
				p, err = deps.Profiles.Get(ctx, args[0])
				if err != nil {
					return err
				}
			} else {
				profiles, err := deps.Profiles.List(ctx)
				if err != nil {
					return err
				}
				if len(profiles) == 0 {
					return fmt.Errorf("no profiles registered; run `ccx profile add` first")
				}
				p, err = pickProfileFallback(profiles)
				if err != nil {
					return err
				}
			}

			sh := platform.DetectShell()
			if shellOverride != "" {
				parsed, ok := contracts.ParseShell(shellOverride)
				if !ok {
					return contracts.ErrUnknownShell
				}
				sh = parsed
			}
			script, err := deps.Shell.EmitUseScript(p, sh)
			if err != nil {
				return err
			}
			if err := deps.Profiles.MarkUsed(ctx, p.Name); err != nil {
				return err
			}
			_, _ = fmt.Fprint(c.OutOrStdout(), script)
			return nil
		},
	}
	cmd.Flags().StringVar(&shellOverride, "shell", "", "force shell flavor (zsh|bash|fish|pwsh); default: auto-detect")
	return cmd
}

func pickProfileFallback(profiles []contracts.Profile) (contracts.Profile, error) {
	return profiles[0], nil
}
