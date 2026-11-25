package traffic

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserCARemovalPreservesOtherCertificatesAndUsers(t *testing.T) {
	ca, err := createCA(filepath.Join(t.TempDir(), "debug-ca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	other, err := createCA(filepath.Join(t.TempDir(), "other-ca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	user0, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer user0.Close()
	clone, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer clone.Close()
	uid, gid := os.Getuid(), os.Getgid()
	label := func(string) error { return nil }
	if err := clone.Mkdir(userCertDir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := ca.public.Filename
	base := strings.TrimSuffix(name, ".0")
	for path, data := range map[string][]byte{
		name:        other.cert.Raw,
		base + ".2": other.cert.Raw,
	} {
		if err := publishUserCAFile(clone, filepath.Join(userCertDir, path), data, 0o644, uid, gid, label); err != nil {
			t.Fatal(err)
		}
	}
	for _, root := range []*os.Root{user0, clone} {
		if changed, err := reconcileUserCA(root, ca, uid, gid, label); err != nil || !changed {
			t.Fatalf("could not install public CA: %t, %v", changed, err)
		}
		if changed, err := reconcileUserCA(root, ca, uid, gid, label); err != nil || changed {
			t.Fatalf("repeat install should be a no-op: %t, %v", changed, err)
		}
	}
	for root, filename := range map[*os.Root]string{user0: name, clone: base + ".1"} {
		path := filepath.Join(userCertDir, filename)
		if body, err := readUserCAFile(root, path); err != nil || !bytes.Equal(body, ca.cert.Raw) {
			t.Fatalf("store does not contain the exact public DER: %v", err)
		}
		if info, err := root.Stat(path); err != nil || info.Mode().Perm() != 0o644 {
			t.Fatalf("incorrect public CA permissions: %v %v", info, err)
		}
		if changed, err := reconcileUserCA(root, nil, uid, gid, label); err != nil || !changed {
			t.Fatalf("could not remove installed CA: %t, %v", changed, err)
		}
		if _, err := root.Lstat(userTrustMarker); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("ownership record remains after removal: %v", err)
		}
	}
	if _, err := user0.Stat(filepath.Join(userCertDir, name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned CA remains after removal: %v", err)
	}
	for _, filename := range []string{name, base + ".2"} {
		if data, err := readUserCAFile(clone, filepath.Join(userCertDir, filename)); err != nil || !bytes.Equal(data, other.cert.Raw) {
			t.Fatalf("unrelated certificate changed: %v", err)
		}
	}
	if data, err := readUserCAFile(clone, filepath.Join(userCertDir, base+".1")); err != nil || len(data) != 0 {
		t.Fatalf("required certificate tombstone missing: %v", err)
	}
}
