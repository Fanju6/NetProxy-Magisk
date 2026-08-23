//go:build !linux && !android

package worker

import "context"

func defaultNetworkEventSource(ctx context.Context, _ func()) error {
	<-ctx.Done()
	return nil
}
