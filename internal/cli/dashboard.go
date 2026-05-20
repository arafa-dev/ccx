package cli

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/arafa-dev/ccx/internal/dashboard"
	"github.com/arafa-dev/ccx/internal/server"
	"github.com/spf13/cobra"
)

func newDashboardCommand(opts *Options) *cobra.Command {
	var (
		port   int
		noOpen bool
	)
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Open the local dashboard",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()
			deps, err := buildDeps(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = deps.Close() }()

			if err := ingestAllProfiles(ctx, deps); err != nil {
				return fmt.Errorf("initial ingest: %w", err)
			}

			webFS, err := dashboard.FS()
			if err != nil {
				return fmt.Errorf("dashboard assets: %w", err)
			}

			srv := server.New(server.Deps{
				Store:    deps.Store,
				Pricing:  deps.Pricing,
				Profiles: deps.Profiles,
				WebRoot:  webFS,
			}, opts.Build.Version)

			startPort := 7777
			endPort := 7787
			if port != 0 {
				startPort, endPort = port, port
			}

			runCtx, cancel := context.WithCancel(ctx)
			defer cancel()

			boundPort, runFn, err := srv.Serve(runCtx, startPort, endPort)
			if err != nil {
				return err
			}
			url := fmt.Sprintf("http://127.0.0.1:%d", boundPort)
			_, _ = fmt.Fprintf(c.OutOrStdout(), "ccx dashboard at %s\n", url)
			if !noOpen {
				go func() {
					time.Sleep(300 * time.Millisecond)
					_ = openBrowser(url)
				}()
			}
			return runFn()
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "port (default: pick next free in 7777-7787)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "do not open a browser")
	return cmd
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		//nolint:gosec // Command is constant and url is generated for the local dashboard.
		return exec.Command("open", url).Start()
	case "windows":
		//nolint:gosec // Command is constant and url is generated for the local dashboard.
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		//nolint:gosec // Command is constant and url is generated for the local dashboard.
		return exec.Command("xdg-open", url).Start()
	}
}
