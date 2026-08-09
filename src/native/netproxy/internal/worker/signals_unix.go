//go:build !windows

package worker

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func withSignals(ctx context.Context) (context.Context, <-chan struct{}, func()) {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	wake := make(chan struct{}, 1)
	updates := make(chan os.Signal, 1)
	signal.Notify(updates, syscall.SIGUSR1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-updates:
				select {
				case wake <- struct{}{}:
				default:
				}
			}
		}
	}()
	cleanup := func() {
		stop()
		signal.Stop(updates)
	}
	return ctx, wake, cleanup
}
