package cli

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/daemon"
	"github.com/arafa-dev/ccx/internal/dashboard"
	"github.com/arafa-dev/ccx/internal/headroom"
	"github.com/arafa-dev/ccx/internal/hooks"
	"github.com/arafa-dev/ccx/internal/quotawire"
	"github.com/arafa-dev/ccx/internal/recstream"
	"github.com/arafa-dev/ccx/internal/server"
	"github.com/arafa-dev/ccx/internal/storage"
	"github.com/spf13/cobra"
)

func newDashboardCommand(opts *Options) *cobra.Command {
	var (
		port       int
		noOpen     bool
		daemonMode bool
	)
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Open the local dashboard",
		RunE: func(c *cobra.Command, _ []string) error {
			if err := validatePort(port); err != nil {
				return err
			}

			ctrl := daemonController(opts)
			status, err := ctrl.Status(c.Context())
			if err != nil {
				return err
			}
			if status.Running && status.URL != "" {
				openOrPrintDashboard(c, opts, status.URL, noOpen)
				return nil
			}
			if daemonMode {
				result, err := ctrl.StartDetached(c.Context(), daemon.StartOptions{Port: port})
				if err != nil {
					return err
				}
				openOrPrintDashboard(c, opts, result.Status.URL, noOpen)
				return nil
			}

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
			quotaProvider, err := dashboardQuotaProvider(deps)
			if err != nil {
				return err
			}
			headroomStore, err := dashboardHeadroomStore(deps)
			if err != nil {
				return err
			}
			recommendations := recstream.NewHub()
			defer recommendations.Close()

			srv := server.New(server.Deps{
				Store:           deps.Store,
				Pricing:         deps.Pricing,
				Profiles:        deps.Profiles,
				WebRoot:         webFS,
				Hooks:           &hooks.Service{Profiles: deps.Profiles},
				Headroom:        headroom.Evaluator{Store: headroomStore, Pricing: deps.Pricing},
				Ingestor:        dashboardHeadroomIngestor{deps: deps},
				Quota:           quotaProvider,
				Recommendations: recommendations,
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
					_ = browserOpener(opts)(url)
				}()
			}
			return runFn()
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "port (default: pick next free in 7777-7787)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "do not open a browser")
	cmd.Flags().BoolVar(&daemonMode, "daemon", false, "start or use the background daemon")
	return cmd
}

func dashboardQuotaProvider(deps *Deps) (server.QuotaProvider, error) {
	store, ok := deps.Store.(*storage.Store)
	if !ok {
		return nil, fmt.Errorf("dashboard quota provider requires *storage.Store, got %T", deps.Store)
	}
	return &quotawire.Adapter{Store: store, Profiles: deps.Profiles}, nil
}

func dashboardHeadroomStore(deps *Deps) (headroom.Store, error) {
	store, ok := deps.Store.(headroom.Store)
	if !ok {
		return nil, fmt.Errorf("dashboard headroom requires headroom.Store, got %T", deps.Store)
	}
	return store, nil
}

type dashboardHeadroomIngestor struct {
	deps *Deps
}

func (i dashboardHeadroomIngestor) IngestHeadroomProfiles(ctx context.Context, profiles []contracts.Profile) (map[string]string, error) {
	return ingestSuggestProfiles(ctx, i.deps, profiles)
}

func openOrPrintDashboard(c *cobra.Command, opts *Options, url string, noOpen bool) {
	_, _ = fmt.Fprintf(c.OutOrStdout(), "ccx dashboard at %s\n", url)
	if !noOpen {
		if err := browserOpener(opts)(url); err != nil {
			_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: failed to open browser: %v\n", err)
		}
	}
}

func browserOpener(opts *Options) func(string) error {
	if opts.OpenBrowser != nil {
		return opts.OpenBrowser
	}
	return openBrowser
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
