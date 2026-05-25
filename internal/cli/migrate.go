package cli

import (
	"fmt"

	"github.com/arafa-dev/ccx/internal/platform"
	"github.com/arafa-dev/ccx/internal/profile"
	"github.com/arafa-dev/ccx/internal/quotamigrate"
	"github.com/spf13/cobra"
)

func newMigrateSharedHistoryCommand(_ *Options) *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "migrate-shared-history",
		Short: "Symlink every profile's projects/ to ~/.ccx/shared-projects/",
		Long: `Walks the profile registry and prints (or executes) the filesystem changes
needed so every profile's <config_dir>/projects/ is a symlink to one shared
directory. Required for ccx run --supervise mid-session swap to preserve
conversation history.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			home, err := platform.CCXHome()
			if err != nil {
				return err
			}
			registry, err := profile.NewManager(home)
			if err != nil {
				return err
			}
			profiles, err := registry.List(ctx)
			if err != nil {
				return err
			}
			steps, err := quotamigrate.Plan(home, profiles)
			if err != nil {
				return err
			}
			if len(steps) == 0 {
				_, _ = fmt.Fprintln(c.OutOrStdout(), "Nothing to do; all profiles already use shared projects.")
				return nil
			}
			for _, step := range steps {
				_, _ = fmt.Fprintln(c.OutOrStdout(), step.String())
			}
			if dryRun {
				_, _ = fmt.Fprintln(c.OutOrStdout(), "\n(dry-run; pass without --dry-run to apply)")
				return nil
			}
			if err := quotamigrate.Apply(steps); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "\nMigration complete (%d step(s)).\n", len(steps))

			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the plan without modifying anything")

	return cmd
}
