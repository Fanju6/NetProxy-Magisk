package module

import (
	"context"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

func ReadPolicyGroups(ctx context.Context, options Options) (service.PolicySnapshot, error) {
	return service.ReadPolicyGroups(ctx, networkControlOptions(options))
}

func SelectPolicyGroup(ctx context.Context, options Options, target service.PolicyTarget, member string) (service.PolicySnapshot, error) {
	release, err := lockPolicyOperation(ctx, options)
	if err != nil {
		return service.PolicySnapshot{}, err
	}
	defer release()
	return service.SelectPolicyGroup(ctx, networkControlOptions(options), target, member)
}

func RequestPolicyDelay(ctx context.Context, options Options, target service.PolicyTarget) error {
	release, err := lockPolicyOperation(ctx, options)
	if err != nil {
		return err
	}
	defer release()
	return service.RequestPolicyDelay(ctx, networkControlOptions(options), target)
}

func lockPolicyOperation(ctx context.Context, options Options) (func(), error) {
	lock, err := waitLifecycleLock(ctx, options.StateFile)
	if err != nil {
		return nil, err
	}
	if err := recoverConfigApply(ctx, options); err != nil {
		lock.release()
		return nil, err
	}
	return lock.release, nil
}
