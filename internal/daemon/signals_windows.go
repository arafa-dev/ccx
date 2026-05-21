//go:build windows

package daemon

import (
	"context"
	"os"
	"os/signal"
)

func signalContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, os.Interrupt)
}
