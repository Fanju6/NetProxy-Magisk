package module

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

func TestPolicyLifecycleWaitUsesCallerCancellation(t *testing.T) {
	options := policyModuleOptions(t)
	lock, err := waitLifecycleLock(t.Context(), options.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	for _, delay := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
		var err error
		if delay {
			_, err = DelayPolicyGroup(ctx, options, "Main")
		} else {
			_, err = SelectPolicyGroup(ctx, options, "Main", "provider-a/same")
		}
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("lifecycle wait ignored cancellation: delay=%v %v", delay, err)
		}
	}
}

func TestPolicyStoppedDoesNotReadOrCreateModuleSelection(t *testing.T) {
	options := policyModuleOptions(t)
	for _, delay := range []bool{false, true} {
		var err error
		if delay {
			_, err = DelayPolicyGroup(t.Context(), options, "Main")
		} else {
			_, err = SelectPolicyGroup(t.Context(), options, "Main", "provider-a/same")
		}
		value, ok := errors.AsType[*service.Error](err)
		if !ok || value.Code != "policy.core_not_ready" {
			t.Fatalf("stopped operation = %v", err)
		}
	}
	for _, path := range []string{options.ModuleConfig, options.StateFile, options.WorkerPIDFile, options.SingBoxPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("unexpected module write: %s %v", path, err)
		}
	}
}

func policyModuleOptions(t *testing.T) Options {
	t.Helper()
	root := t.TempDir()
	t.Setenv("NETPROXY_MODULE_DIR", root)
	t.Setenv("NETPROXY_DEV_ROOT", filepath.Join(root, "dev"))
	return NewOptions(root)
}
