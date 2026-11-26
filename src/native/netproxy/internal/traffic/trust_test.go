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

func TestOwnedCAMountLayersDistinguishLegacyAndUnrelatedTmpfs(t *testing.T) {
	store := systemStores[1]
	const stock = "19 11 0:6 / /apex/com.android.conscrypt/cacerts ro - ext4 /dev/block/loop2 ro\n"
	const legacy = "20 19 0:21 / /apex/com.android.conscrypt/cacerts rw - tmpfs tmpfs rw\n"
	const legacyBind = "28 19 0:35 /netproxy/traffic-trust/conscrypt /apex/com.android.conscrypt/cacerts rw - tmpfs tmpfs rw\n"
	const marked = "21 20 0:22 / /apex/com.android.conscrypt/cacerts rw - tmpfs netproxy-traffic-ca-conscrypt rw\n"
	const other = "22 21 0:23 / /apex/com.android.conscrypt/cacerts rw - tmpfs other-ca-tool rw\n"
	for _, tc := range []struct {
		info, top string
		fail      bool
	}{
		{marked + stock + legacy, "21", false},
		{stock + legacy, "20", false},
		{stock + legacyBind, "28", false},
		{stock, "", false},
		{stock + "24 19 0:24 / /apex/com.android.conscrypt/cacerts rw - tmpfs tmpfs rw\n", "", false},
		{stock + legacy + marked + other, "", true},
	} {
		mount, owned, err := ownedMountTop(tc.info, store, "/dev/netproxy/traffic-trust/conscrypt", map[string]bool{"0:21": true})
		if mount.id != tc.top || owned != (tc.top != "") || (err != nil) != tc.fail {
			t.Fatalf("mount %q, owned=%t, err=%v for %q", mount.id, owned, err, tc.info)
		}
	}
}
