package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	hookspkg "github.com/arafa-dev/ccx/internal/hooks"
	"github.com/spf13/cobra"
)

func newHooksCommand(_ *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hooks",
		Short: "Manage Claude Code hook telemetry",
	}
	cmd.AddCommand(
		newHooksInstallCmd(),
		newHooksStatusCmd(),
		newHooksUninstallCmd(),
		newHooksRecordCmd(),
	)
	return cmd
}

func newHooksInstallCmd() *cobra.Command {
	var (
		profileFlag string
		force       bool
		asJSON      bool
	)
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install ccx-managed Claude Code hooks",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = deps.Close() }()

			svc := &hookspkg.Service{Profiles: deps.Profiles, Store: deps.Store}
			results, err := svc.Install(ctx, hookspkg.InstallOptions{Profile: profileFlag, Force: force})
			if asJSON {
				if encErr := writeHookResultsJSON(c.OutOrStdout(), results); encErr != nil {
					return encErr
				}
			} else if len(results) > 0 {
				if renderErr := renderHookResults(c.OutOrStdout(), results); renderErr != nil {
					return renderErr
				}
			}
			return err
		},
	}
	cmd.Flags().StringVar(&profileFlag, "profile", "", "limit to one profile (default: all)")
	cmd.Flags().BoolVar(&force, "force", false, "replace existing ccx-managed hooks")
	cmd.Flags().BoolVar(&asJSON, "json", false, "JSON output")
	return cmd
}

func newHooksStatusCmd() *cobra.Command {
	var (
		profileFlag string
		asJSON      bool
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show ccx-managed Claude Code hook status",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = deps.Close() }()

			svc := &hookspkg.Service{Profiles: deps.Profiles, Store: deps.Store}
			results, err := svc.Status(ctx, hookspkg.StatusOptions{Profile: profileFlag})
			if err != nil {
				return err
			}
			if asJSON {
				return writeHookResultsJSON(c.OutOrStdout(), results)
			}
			return renderHookResults(c.OutOrStdout(), results)
		},
	}
	cmd.Flags().StringVar(&profileFlag, "profile", "", "limit to one profile (default: all)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "JSON output")
	return cmd
}

func newHooksUninstallCmd() *cobra.Command {
	var (
		profileFlag string
		asJSON      bool
	)
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove ccx-managed Claude Code hooks",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = deps.Close() }()

			svc := &hookspkg.Service{Profiles: deps.Profiles, Store: deps.Store}
			results, err := svc.Uninstall(ctx, hookspkg.UninstallOptions{Profile: profileFlag})
			if asJSON {
				if encErr := writeHookResultsJSON(c.OutOrStdout(), results); encErr != nil {
					return encErr
				}
			} else if len(results) > 0 {
				if renderErr := renderHookResults(c.OutOrStdout(), results); renderErr != nil {
					return renderErr
				}
			}
			return err
		},
	}
	cmd.Flags().StringVar(&profileFlag, "profile", "", "limit to one profile (default: all)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "JSON output")
	return cmd
}

func newHooksRecordCmd() *cobra.Command {
	var profileFlag string
	cmd := &cobra.Command{
		Use:    "record --profile <name>",
		Short:  "Record Claude Code hook telemetry",
		Hidden: true,
		RunE: func(c *cobra.Command, _ []string) error {
			if profileFlag == "" {
				return fmt.Errorf("hooks record requires --profile")
			}
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = deps.Close() }()

			svc := &hookspkg.Service{Profiles: deps.Profiles, Store: deps.Store}
			_, err = svc.Record(ctx, hookspkg.RecordOptions{
				Profile: profileFlag,
				Input:   c.InOrStdin(),
			})
			return err
		},
	}
	cmd.Flags().StringVar(&profileFlag, "profile", "", "registered profile name (required)")
	return cmd
}

func writeHookResultsJSON(w io.Writer, results []hookspkg.Result) error {
	enc := json.NewEncoder(w)
	return enc.Encode(results)
}

func renderHookResults(w io.Writer, results []hookspkg.Result) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PROFILE\tSTATUS\tSETTINGS\tBACKUP\tMESSAGE")
	for _, result := range results {
		message := result.Message
		if result.Error != "" {
			message = result.Error
		}
		backup := result.BackupPath
		if backup == "" {
			backup = "-"
		}
		_, _ = fmt.Fprintf(
			tw, "%s\t%s\t%s\t%s\t%s\n",
			result.Profile,
			result.Status,
			result.SettingsPath,
			backup,
			message,
		)
	}
	return tw.Flush()
}
