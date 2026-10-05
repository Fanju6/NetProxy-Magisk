//go:build windows

package worker

import (
	"context"
	"errors"
	"os"
	"os/signal"
)

func wakeProcess(pid int) error { return errors.New("Windows 不支持 Worker 进程信号") }

func withSignals(ctx context.Context) (context.Context, <-chan struct{}, func()) {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	return ctx, nil, stop
}
