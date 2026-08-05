package fetch

import (
	"context"
	"errors"
	"net"
	"testing"
)

type testResolver struct {
	addresses []net.IPAddr
	err       error
	calls     int
}

func (resolver *testResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	resolver.calls++
	return resolver.addresses, resolver.err
}

func TestLookupIPAddressesFallsBackAfterSystemResolverFailure(t *testing.T) {
	system := &testResolver{err: errors.New("local DNS unavailable")}
	fallback := &testResolver{addresses: []net.IPAddr{{IP: net.ParseIP("203.0.113.8")}}}

	addresses, err := lookupIPAddresses(context.Background(), "example.com", []ipResolver{system, fallback})
	if err != nil {
		t.Fatal(err)
	}
	if system.calls != 1 || fallback.calls != 1 {
		t.Fatalf("unexpected resolver calls: system=%d fallback=%d", system.calls, fallback.calls)
	}
	if got := addresses[0].IP.String(); got != "203.0.113.8" {
		t.Fatalf("unexpected resolved address: %s", got)
	}
}

func TestLookupIPAddressesStopsAfterFirstSuccess(t *testing.T) {
	system := &testResolver{addresses: []net.IPAddr{{IP: net.ParseIP("192.0.2.10")}}}
	fallback := &testResolver{addresses: []net.IPAddr{{IP: net.ParseIP("203.0.113.8")}}}

	addresses, err := lookupIPAddresses(context.Background(), "example.com", []ipResolver{system, fallback})
	if err != nil {
		t.Fatal(err)
	}
	if fallback.calls != 0 {
		t.Fatalf("fallback resolver should not run after system success: %d", fallback.calls)
	}
	if got := addresses[0].IP.String(); got != "192.0.2.10" {
		t.Fatalf("unexpected resolved address: %s", got)
	}
}
