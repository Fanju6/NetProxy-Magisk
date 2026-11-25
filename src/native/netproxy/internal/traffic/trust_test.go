package traffic

import (
	"os"
	"reflect"
	"testing"
)

func TestNamespaceArgsUseExplicitMountNamespace(t *testing.T) {
	command := []string{"/system/bin/mount", "-o", "bind", "/proc/1/root/dev/netproxy/traffic-trust/system", "/system/etc/security/cacerts"}
	want := append([]string{"--mount=/proc/593/ns/mnt", "--"}, command...)
	if got := namespaceArgs(593, command); !reflect.DeepEqual(got, want) {
		t.Fatalf("namespace args = %q; want %q", got, want)
	}
}

func TestUniqueMountTargetsIncludeExistingAppNamespaces(t *testing.T) {
	namespaces := map[int]string{1: "mnt:[10]", 81: "mnt:[10]", 200: "mnt:[11]", 1100: "mnt:[12]", 1101: "mnt:[12]"}
	got := uniqueMountTargets([]int{1100, 1101, 100, 81, 1, 200}, func(pid int) (mountTarget, error) {
		namespace, ok := namespaces[pid]
		if !ok {
			return mountTarget{}, os.ErrNotExist
		}
		return mountTarget{pid, namespace}, nil
	})
	want := []mountTarget{{1, "mnt:[10]"}, {200, "mnt:[11]"}, {1100, "mnt:[12]"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mount targets = %v; want %v", got, want)
	}
}

func TestListedMountMatchesExactEscapedPath(t *testing.T) {
	const info = "29 23 0:28 / /dev rw - tmpfs tmpfs rw\n30 29 0:29 / /dev/netproxy\\040test/system rw - tmpfs tmpfs rw\n"
	for path, want := range map[string]bool{
		"/dev/netproxy test/system": true,
		"/dev/netproxy test":        false,
		"/dev/netproxy test/apex":   false,
	} {
		if got := listedMount(info, path); got != want {
			t.Errorf("listedMount(%q) = %t; want %t", path, got, want)
		}
	}
}
