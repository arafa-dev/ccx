package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/daemon"
	"github.com/spf13/cobra"
)

func newDaemonCommand(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage the ccx background daemon",
	}
	cmd.AddCommand(
		newDaemonStartCommand(opts),
		newDaemonStatusCommand(opts),
		newDaemonStopCommand(opts),
		newDaemonRestartCommand(opts),
		newDaemonLogsCommand(opts),
	)
	return cmd
}

func newDaemonStartCommand(opts *Options) *cobra.Command {
	var (
		foreground   bool
		port         int
		pollInterval time.Duration
		asJSON       bool
	)
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the ccx daemon",
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validatePort(port); err != nil {
				return writeDaemonError(c, asJSON, err)
			}
			ctrl := daemonController(opts)
			if foreground {
				status, err := ctrl.Status(c.Context())
				if err != nil {
					return writeDaemonError(c, asJSON, err)
				}
				if status.Running {
					return writeDaemonStartResult(c.OutOrStdout(), &daemon.StartResult{
						Status:         status,
						AlreadyRunning: true,
					}, asJSON)
				}
				if err := daemon.Run(c.Context(), daemon.RunOptions{
					Root:         opts.DaemonRoot,
					Version:      opts.Build.Version,
					Port:         port,
					PollInterval: pollInterval,
					OnStatus: func(status contracts.DaemonStatus) {
						_ = writeDaemonStartResult(c.OutOrStdout(), &daemon.StartResult{
							Status:  status,
							Started: true,
						}, asJSON)
					},
				}); err != nil {
					return writeDaemonError(c, asJSON, err)
				}
				return nil
			}
			result, err := ctrl.StartDetached(c.Context(), daemon.StartOptions{
				Port:         port,
				PollInterval: pollInterval,
			})
			if err != nil {
				return writeDaemonError(c, asJSON, err)
			}
			return writeDaemonStartResult(c.OutOrStdout(), &result, asJSON)
		},
	}
	cmd.Flags().BoolVar(&foreground, "foreground", false, "run in the current process")
	cmd.Flags().IntVar(&port, "port", 0, "port (default: pick next free in 7777-7787)")
	cmd.Flags().DurationVar(&pollInterval, "poll-interval", 60*time.Second, "fallback full-scan interval")
	cmd.Flags().BoolVar(&asJSON, "json", false, "JSON output")
	return cmd
}

func newDaemonStatusCommand(opts *Options) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show daemon status",
		RunE: func(c *cobra.Command, _ []string) error {
			status, err := daemonController(opts).Status(c.Context())
			if err != nil {
				return writeDaemonError(c, asJSON, err)
			}
			return writeDaemonStatus(c.OutOrStdout(), &status, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "JSON output")
	return cmd
}

func newDaemonStopCommand(opts *Options) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the ccx daemon",
		RunE: func(c *cobra.Command, _ []string) error {
			result, err := daemonController(opts).Stop(c.Context())
			if err != nil {
				return writeDaemonError(c, asJSON, err)
			}
			return writeDaemonStopResult(c.OutOrStdout(), &result, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "JSON output")
	return cmd
}

func newDaemonRestartCommand(opts *Options) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart the ccx daemon",
		RunE: func(c *cobra.Command, _ []string) error {
			result, err := daemonController(opts).Restart(c.Context(), daemon.StartOptions{})
			if err != nil {
				return writeDaemonError(c, asJSON, err)
			}
			return writeDaemonStartResult(c.OutOrStdout(), &result, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "JSON output")
	return cmd
}

func newDaemonLogsCommand(opts *Options) *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Print daemon logs",
		RunE: func(c *cobra.Command, _ []string) error {
			return daemonController(opts).WriteLogs(c.Context(), c.OutOrStdout(), follow)
		},
	}
	cmd.Flags().BoolVar(&follow, "follow", false, "follow daemon.log")
	return cmd
}

func daemonController(opts *Options) *daemon.Controller {
	return &daemon.Controller{
		Root:       opts.DaemonRoot,
		Version:    opts.Build.Version,
		Executable: opts.Executable,
		Process:    opts.DaemonProcess,
	}
}

func validatePort(port int) error {
	if port != 0 && (port < 1 || port > 65535) {
		return fmt.Errorf("invalid --port %d: must be in range 1-65535", port)
	}
	return nil
}

func writeDaemonStartResult(w io.Writer, result *daemon.StartResult, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(w).Encode(result)
	}
	switch {
	case result.AlreadyRunning:
		_, _ = fmt.Fprintf(w, "ccx daemon already running at %s (pid %d)\n", result.Status.URL, result.Status.PID)
	case result.Started:
		_, _ = fmt.Fprintf(w, "ccx daemon started at %s (pid %d)\n", result.Status.URL, result.Status.PID)
	default:
		_, _ = fmt.Fprintf(w, "ccx daemon at %s (pid %d)\n", result.Status.URL, result.Status.PID)
	}
	return nil
}

func writeDaemonStatus(w io.Writer, status *contracts.DaemonStatus, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(w).Encode(map[string]*contracts.DaemonStatus{"status": status})
	}
	if status.Running {
		_, _ = fmt.Fprintf(w, "ccx daemon running at %s (pid %d)\n", status.URL, status.PID)
		return nil
	}
	if status.PID != 0 {
		_, _ = fmt.Fprintf(w, "ccx daemon not running (stale pid %d)\n", status.PID)
		return nil
	}
	_, _ = fmt.Fprintln(w, "ccx daemon not running")
	return nil
}

func writeDaemonStopResult(w io.Writer, result *daemon.StopResult, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(w).Encode(result)
	}
	if result.Stopped {
		_, _ = fmt.Fprintf(w, "ccx daemon stopped (pid %d)\n", result.Status.PID)
		return nil
	}
	_, _ = fmt.Fprintln(w, "ccx daemon not running")
	return nil
}

func writeDaemonError(c *cobra.Command, asJSON bool, err error) error {
	if !asJSON {
		return err
	}
	_ = json.NewEncoder(c.ErrOrStderr()).Encode(map[string]string{"error": err.Error()})
	return &structuredCLIError{err: err}
}
