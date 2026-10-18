package module

import (
	"context"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

func ReadPolicyGroups(ctx context.Context, options Options) (service.PolicySnapshot, error) {
	return service.ReadPolicyGroups(ctx, networkControlOptions(options))
}

func SelectPolicyGroup(ctx context.Context, options Options, groupTag, memberTag string) (service.PolicySnapshot, error) {
	return withPolicyLifecycle(ctx, options, func() (service.PolicySnapshot, error) {
		return service.SelectPolicyGroup(ctx, networkControlOptions(options), groupTag, memberTag)
	})
}

func DelayPolicyGroup(ctx context.Context, options Options, groupTag string) (service.PolicySnapshot, error) {
	return withPolicyLifecycle(ctx, options, func() (service.PolicySnapshot, error) {
		return service.DelayPolicyGroup(ctx, networkControlOptions(options), groupTag)
	})
}

func withPolicyLifecycle(ctx context.Context, options Options, operation func() (service.PolicySnapshot, error)) (service.PolicySnapshot, error) {
	lock, err := waitLifecycleLock(ctx, options.StateFile)
	if err != nil {
		return service.PolicySnapshot{}, err
	}
	defer lock.release()
	if err := recoverConfigApply(ctx, options); err != nil {
		return service.PolicySnapshot{}, err
	}
	return operation()
}
