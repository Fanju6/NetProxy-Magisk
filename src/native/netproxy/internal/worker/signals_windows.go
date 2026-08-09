//go:build windows

package worker

import (
	"context"
	"os"
	"os/signal"
)

func withSignals(ctx context.Context) (context.Context, <-chan struct{}, func()) {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	return ctx, nil, stop
}
