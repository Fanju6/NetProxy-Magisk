//go:build linux

package traffic

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"golang.org/x/net/http2"
)

func TestH2GRPCCaptureThroughCoreBridge(t *testing.T) {
	framed := []byte{0, 0, 0, 0, 6, 8, 7, 18, 2, 8, 9}
	origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if r.ProtoMajor != 2 || err != nil || !bytes.Equal(body, framed) ||
			r.Header.Get("Te") != "trailers" || r.Header.Get("X-Rewritten") != "yes" || r.Trailer.Get("X-Request") != "complete" {
			t.Errorf("invalid origin request: proto=%s, body=%x, headers=%v, trailers=%v, err=%v", r.Proto, body, r.Header, r.Trailer, err)
		}
		w.Header().Set("Content-Type", "application/grpc+proto")
		w.Header().Set("Trailer", "Grpc-Status, Grpc-Message")
		_, _ = w.Write(append(bytes.Clone(framed), framed...))
		w.Header().Set("Grpc-Status", "3")
		w.Header().Set("Grpc-Message", "sample%20error")
	}))
	origin.EnableHTTP2 = true
	origin.StartTLS()
	defer origin.Close()
	layout := paths.New(t.TempDir())
	c := New(layout)
	c.upstreamRoots = origin.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	if err := c.ReplaceRules([]Rule{{"^https://127[.]0[.]0[.]1:", "request", `return {set_headers: {"X-Rewritten": "yes"}};`, []string{"127.0.0.1"}, nil, ""}}); err != nil {
		t.Fatal(err)
	}
	roots := startProtocolCapture(t, c)
	dial := bridgeFixture(layout)
	if core := os.Getenv("NETPROXY_TEST_CAPTURE_CORE"); core != "" {
		dial = startCaptureTestCore(t, core, layout)
	}
	conn, err := dial(t.Context(), "tcp", origin.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	stream := tls.Client(conn, &tls.Config{RootCAs: roots, ServerName: "127.0.0.1", NextProtos: []string{"h2"}})
	defer stream.Close()
	if err := stream.HandshakeContext(t.Context()); err != nil || stream.ConnectionState().NegotiatedProtocol != "h2" {
		t.Fatalf("downstream h2 negotiation: %v", err)
	}
	client, err := (&http2.Transport{}).NewClientConn(stream)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	for i := 0; i < 2; i++ {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, origin.URL+"/example.Echo/Send", bytes.NewReader(framed))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/grpc")
		req.Header.Set("Te", "trailers")
		req.Trailer = http.Header{"X-Request": {"complete"}}
		res, err := client.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if err != nil || !bytes.Equal(body, append(bytes.Clone(framed), framed...)) || res.Trailer.Get("Grpc-Status") != "3" ||
			res.Trailer.Get("Grpc-Message") != "sample%20error" {
			t.Fatalf("invalid response: %x, %v, %v", body, res.Trailer, err)
		}
	}
	entries := c.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected one record per stream: %+v", entries)
	}
	for i, e := range entries {
		record, ok := c.Record(e.ID)
		if !ok || e.Protocol != "h2" || e.Sequence != uint64(len(entries)-i) || e.Status != 200 || !e.Rewritten || record.HTTP == nil {
			t.Fatalf("incorrect stream record: %+v", record)
		}
		req, res := record.HTTP.Request, record.HTTP.Response
		if req.Trailers.Get("X-Request") != "complete" || req.GRPC == nil || len(req.GRPC.Messages) != 1 ||
			res.Trailers.Get("Grpc-Status") != "3" || res.GRPC == nil || res.GRPC.Status != "3" ||
			res.GRPC.Message != "sample error" || len(res.GRPC.Messages) != 2 {
			t.Fatalf("incomplete gRPC record: req=%+v / %+v, res=%+v / %+v", req.Trailers, req.GRPC, res.Trailers, res.GRPC)
		}
		fields := req.GRPC.Messages[0].Fields
		if len(fields) != 2 || fields[0].Value != "7" || fields[1].Wire != 2 ||
			len(fields[1].Fields) != 1 || fields[1].Fields[0].Value != "9" {
			t.Fatalf("incorrect protobuf fields: %+v", fields)
		}
	}
}

func startProtocolCapture(t *testing.T, c *Controller) *x509.CertPool {
	t.Helper()
	if _, err := c.SetMITMPolicy(MITMPolicy{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Stop)
	ca, err := c.Certificate(false)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(ca.PEM)) {
		t.Fatal("invalid test CA")
	}
	return roots
}

func TestALPNIncompatibilityBypassesNextConnection(t *testing.T) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "original")
	}))
	defer origin.Close()
	layout := paths.New(t.TempDir())
	c := New(layout)
	originRoots := origin.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	c.upstreamRoots = originRoots
	roots := startProtocolCapture(t, c)
	uid := uint32(10123)
	const pkg = "com.example.capture"
	dial := bridgeFixtureFor(layout, &Exit{UID: &uid, Package: pkg})
	get := func(offer []string, trust *x509.CertPool) error {
		tr := &http.Transport{DialContext: dial, TLSClientConfig: &tls.Config{RootCAs: trust, NextProtos: offer}}
		defer tr.CloseIdleConnections()
		res, err := (&http.Client{Transport: tr, Timeout: 5 * time.Second}).Get(origin.URL)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		_, err = io.Copy(io.Discard, res.Body)
		return err
	}
	if err := get([]string{"h2"}, roots); err == nil {
		t.Fatal("h2-only client accepted a non-h2 origin")
	}
	fresh := New(layout)
	if !fresh.isALPNBypassed("127.0.0.1", &SourceApp{UserID: 0, Package: pkg}) ||
		fresh.isALPNBypassed("127.0.0.1", &SourceApp{UserID: 1, Package: pkg}) ||
		fresh.isALPNBypassed("127.0.0.1", &SourceApp{UserID: 0, Package: "com.example.other"}) {
		t.Fatal("expected persisted host + user + package bypass")
	}
	if err := get([]string{"http/1.1"}, originRoots); err != nil {
		t.Fatalf("next connection was not passed through: %v", err)
	}
	if entries := c.Entries(); len(entries) != 2 || entries[0].PassthroughReason != "alpn_bypassed" || entries[1].PassthroughReason != "alpn_incompatible" {
		t.Fatalf("unexpected ALPN attempts: %+v", entries)
	}
	if _, err := c.ClearALPNBypasses(); err != nil {
		t.Fatal(err)
	}
	if entries, err := New(layout).ALPNBypasses(); err != nil || len(entries) != 0 {
		t.Fatalf("bypasses not cleared: %+v, %v", entries, err)
	}
	if err := get([]string{"h2", "http/1.1"}, roots); err != nil {
		t.Fatalf("client offering HTTP/1.1 could not negotiate with HTTP/1.1 origin: %v", err)
	}
}
