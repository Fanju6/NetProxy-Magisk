package traffic

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"slices"
	"strings"
	"syscall"
	"time"
)

func canonicalHost(host string) string {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if address, err := netip.ParseAddr(host); err == nil {
		return address.String()
	}
	return host
}

type helloProbe struct {
	net.Conn
	reader io.Reader
}

func (p helloProbe) Read(b []byte) (int, error) { return p.reader.Read(b) }
func (p helloProbe) Write([]byte) (int, error)  { return 0, errBridge }

func sniffTLS(ctx context.Context, client *bufferedStream, metadata flow) (host, reason string) {
	first, err := client.reader.Peek(1)
	if err != nil {
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return "", "sniff_timeout"
		}
		return "", "protocol_unidentified"
	}
	if first[0] != 22 {
		return "", "non_tls"
	}
	var replay bytes.Buffer
	reader := io.TeeReader(io.LimitReader(client.reader, 64<<10), &replay)
	reason = "tls_hello_unavailable"
	probe := tls.Server(helloProbe{client, reader}, &tls.Config{
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			host = hello.ServerName
			if host == "" {
				host, _, _ = net.SplitHostPort(metadata.Destination)
			}
			host = canonicalHost(host)
			switch {
			case len(hello.SupportedProtos) != 0 && !slices.Contains(hello.SupportedProtos, "http/1.1"):
				reason = "unsupported_alpn"
			case !validMITMHost(host):
				reason = "tls_host_unavailable"
			default:
				reason = ""
			}
			return nil, errBridge
		},
	})
	if err := probe.HandshakeContext(ctx); errors.Is(err, os.ErrDeadlineExceeded) {
		reason = "sniff_timeout"
	}
	client.reader = bufio.NewReader(io.MultiReader(&replay, client.reader))
	return host, reason
}

type handshakePeer struct {
	net.Conn
	certificateSelected bool
	certificateFlight   bool
}

func (p *handshakePeer) Write(b []byte) (int, error) {
	n, err := p.Conn.Write(b)
	if p.certificateSelected && err == nil && n == len(b) {
		p.certificateFlight = true
	}
	return n, err
}

func (p *handshakePeer) rejectedCertificate(err error) bool {
	if !p.certificateFlight {
		return false
	}
	var peerError *net.OpError
	if errors.As(err, &peerError) && peerError.Op == "remote error" {
		// crypto/tls wraps TCP peer alerts in its private alert type.
		switch peerError.Err.Error() {
		case tls.AlertError(42).Error(), tls.AlertError(46).Error(), tls.AlertError(48).Error():
			return true
		}
	}
	return errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNRESET) && peerError != nil && peerError.Op == "read"
}

func (s *session) intercept(client *bufferedStream, upstream net.Conn, metadata flow, host string, policy *mitmState, now time.Time) bool {
	if policy.ctx.Err() != nil {
		return false
	}
	leaf, err := policy.certificate(host, now)
	if err != nil {
		return false
	}
	defer context.AfterFunc(policy.ctx, func() { _ = client.Close() })()
	peer := &handshakePeer{Conn: client}
	secure := tls.Server(peer, &tls.Config{
		MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"},
		GetCertificate: func(info *tls.ClientHelloInfo) (*tls.Certificate, error) {
			if info.ServerName != "" && canonicalHost(info.ServerName) != host {
				return nil, errors.New("TLS ServerName mismatch")
			}
			peer.certificateSelected = true
			return &leaf, nil
		},
	})
	handshake, cancel := context.WithTimeout(s.ctx, 12*time.Second)
	err = secure.HandshakeContext(handshake)
	cancel()
	if err != nil {
		if policy.ctx.Err() == nil && s.ctx.Err() == nil {
			entry := metadata.entry(now, "TLS", host)
			entry.TLS, entry.DurationMS = "handshake_failed", time.Since(now).Milliseconds()
			if peer.rejectedCertificate(err) && s.controller.rememberPinned(host, metadata.sourceApp) {
				entry.TLS, entry.PassthroughReason = "pinned_skipped", "certificate_rejected"
			}
			// No HTTP response exists for a failed client TLS handshake.
			s.controller.record(s, entry)
		}
		return true
	}
	s.serveHTTPStream(secure, client.done, upstream, metadata, "https", &tls.Config{
		MinVersion: tls.VersionTLS12, ServerName: host, RootCAs: s.controller.upstreamRoots,
	})
	return true
}
