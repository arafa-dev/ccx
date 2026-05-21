// Command ccx is the user-facing CLI binary for the ccx workspace manager.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/arafa-dev/ccx/internal/cli"
)

// Set by the build via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Execute(ctx, cli.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	})
	stop()
	os.Exit(code)
}
