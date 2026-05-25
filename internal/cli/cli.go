// Package cli wires Phase 1 packages into the cobra command tree exposed by cmd/ccx.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/arafa-dev/ccx/internal/daemon"
	"github.com/arafa-dev/ccx/internal/run"
	"github.com/spf13/cobra"
)

// BuildInfo carries version metadata baked in at build time.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Options configures a single Run invocation, used by tests and main.
type Options struct {
	Args          []string
	Stdin         io.Reader
	Stdout        io.Writer
	Stderr        io.Writer
	Build         BuildInfo
	DaemonRoot    string
	DaemonProcess daemon.ProcessManager
	Executable    string
	OpenBrowser   func(string) error
}

// Execute is the production entry point using os.Args and os.Stdin/out/err.
func Execute(ctx context.Context, build BuildInfo) int {
	return Run(ctx, Options{
		Args:   os.Args[1:],
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Build:  build,
	})
}

// Run builds the root cobra command from Options and executes it.
//
//nolint:gocritic // Value options keep test call sites simple and immutable.
func Run(ctx context.Context, opts Options) int {
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	root := newRootCommand(&opts)
	root.SetArgs(opts.Args)
	root.SetOut(opts.Stdout)
	root.SetErr(opts.Stderr)
	if opts.Stdin != nil {
		root.SetIn(opts.Stdin)
	}
	if err := root.ExecuteContext(ctx); err != nil {
		var coded run.ExitCodeError
		if errors.As(err, &coded) {
			return coded.Code
		}
		var structured *structuredCLIError
		if !errors.As(err, &structured) {
			_, _ = fmt.Fprintf(opts.Stderr, "Error: %s\n", err)
		}
		return 1
	}
	return 0
}

type structuredCLIError struct {
	err error
}

func (e *structuredCLIError) Error() string {
	return e.err.Error()
}

func (e *structuredCLIError) Unwrap() error {
	return e.err
}

func newRootCommand(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "ccx",
		Short:         "Multi-account workspace manager for Claude Code",
		Long:          "ccx switches between Claude Code accounts and tracks usage across them.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(
		newVersionCommand(opts),
		newProfileCommand(opts),
		newUseCommand(opts),
		newInitCommand(opts),
		newUsageCommand(opts),
		newSuggestCommand(opts),
		newRunCommand(opts),
		newDashboardCommand(opts),
		newDaemonCommand(opts),
		newDoctorCommand(opts),
		newHooksCommand(opts),
	)
	return cmd
}

func newVersionCommand(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		RunE: func(c *cobra.Command, _ []string) error {
			_, _ = fmt.Fprintf(c.OutOrStdout(), "ccx %s (commit %s, built %s)\n",
				opts.Build.Version, opts.Build.Commit, opts.Build.Date)
			return nil
		},
	}
}
