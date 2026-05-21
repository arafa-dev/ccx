//go:build darwin || linux

package daemon

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func signalContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
}
