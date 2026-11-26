package traffic

import (
	"errors"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

func TestDeleteOnlyMatchingBypassAndPersist(t *testing.T) {
	layout := paths.New(t.TempDir())
	c := New(layout)
	host := "example.test"
	primary := &SourceApp{UserID: 0, Package: "test.example"}
	clone := &SourceApp{UserID: 10, Package: "test.example"}
	c.rememberPinned(host, primary, "certificate_rejected")
	c.rememberPinned(host, clone, "client_ca_untrusted")
	c.rememberALPN(host, primary)
	c.rememberALPN(host, clone)

	if _, err := c.RemovePinnedBypass(host, "invalid"); !errors.Is(err, ErrBypassKey) {
		t.Fatalf("invalid target: %v", err)
	}
	pinned, err := c.RemovePinnedBypass("EXAMPLE.TEST", "0:test.example")
	if err != nil || len(pinned) != 1 || pinned[0].UserID != 10 || pinned[0].CreatedAt.IsZero() ||
		pinned[0].Reason != "client_ca_untrusted" {
		t.Fatalf("remaining pinning bypasses: %+v; %v", pinned, err)
	}
	if !c.isALPNBypassed(host, primary) {
		t.Fatal("pinning deletion must not change the ALPN list")
	}
	alpn, err := c.RemoveALPNBypass(host, "0:test.example")
	if err != nil || len(alpn) != 1 || alpn[0].UserID != 10 {
		t.Fatalf("remaining ALPN bypasses: %+v; %v", alpn, err)
	}
	reopened := New(layout)
	if reopened.isPinned(host, primary) || !reopened.isPinned(host, clone) ||
		reopened.isALPNBypassed(host, primary) || !reopened.isALPNBypassed(host, clone) {
		t.Fatal("deletion did not persist or deleted another app's bypass")
	}
}
