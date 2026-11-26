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
	"sync"
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

func sniffTLS(ctx context.Context, client *bufferedStream, metadata flow) (host string, offered []string, reason string) {
	first, err := client.reader.Peek(1)
	if err != nil {
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return "", nil, "sniff_timeout"
		}
		return "", nil, "protocol_unidentified"
	}
	if first[0] != 22 {
		return "", nil, "non_tls"
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
			offered = slices.Clone(hello.SupportedProtos)
			switch {
			case len(offered) != 0 && !slices.Contains(offered, "http/1.1") && !slices.Contains(offered, "h2"):
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
	return host, offered, reason
}

type handshakePeer struct {
	net.Conn
	certificateSelected bool
	certificateFlight   bool
}

func (p *handshakePeer) Write(b []byte) (int, error) {
	n, err := p.Conn.Write(b)
	if p.certificateSelected && !p.certificateFlight && err == nil && n == len(b) {
		p.certificateFlight = true
	}
	return n, err
}

func remoteCertificateReason(err error) string {
	var peerError *net.OpError
	if errors.As(err, &peerError) && peerError.Op == "remote error" {
		// crypto/tls wraps TCP peer alerts in its private alert type.
		switch peerError.Err.Error() {
		case tls.AlertError(48).Error():
			return "client_ca_untrusted"
		case tls.AlertError(42).Error(), tls.AlertError(46).Error(), tls.AlertError(49).Error():
			return "certificate_rejected"
		}
	}
	return ""
}

type initialTLSRead struct {
	*tls.Conn
	once     sync.Once
	rejected func(error)
}

func (c *initialTLSRead) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n != 0 || err != nil {
		c.once.Do(func() {
			if n == 0 && err != nil {
				c.rejected(err)
			}
		})
	}
	return n, err
}

func tlsHandshakeReason(err error, upstream bool) string {
	var netErr net.Error
	timeout := errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) && netErr.Timeout()
	if upstream {
		if timeout {
			return "upstream_handshake_timeout"
		}
		return "upstream_handshake_failed"
	}
	if timeout {
		return "client_handshake_timeout"
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return "client_handshake_closed"
	}
	return "client_handshake_failed"
}

func (s *session) intercept(client *bufferedStream, upstream net.Conn, metadata flow, host string, offered []string, policy *mitmState, now time.Time) bool {
	if policy.ctx.Err() != nil {
		return false
	}
	leaf, err := policy.certificate(host, now)
	if err != nil {
		return false
	}
	defer context.AfterFunc(policy.ctx, func() { _ = client.Close() })()
	origin := tls.Client(upstream, &tls.Config{
		MinVersion: tls.VersionTLS12, ServerName: host, RootCAs: s.controller.upstreamRoots, NextProtos: offered,
	})
	originHandshake, cancelOrigin := context.WithTimeout(s.ctx, 10*time.Second)
	defer context.AfterFunc(policy.ctx, cancelOrigin)()
	originErr := origin.HandshakeContext(originHandshake)
	cancelOrigin()
	protocol := origin.ConnectionState().NegotiatedProtocol
	if alpnMismatch(offered, protocol, originErr) {
		if s.ctx.Err() == nil && policy.ctx.Err() == nil {
			s.controller.rememberALPN(host, metadata.sourceApp)
			entry := metadata.entry(now, "TLS", host)
			entry.TLS, entry.PassthroughReason = "handshake_failed", "alpn_incompatible"
			entry.DurationMS = time.Since(now).Milliseconds()
			s.controller.record(s, entry)
		}
		return true
	}
	if originErr != nil {
		metadata.upstreamFailure = tlsHandshakeReason(originErr, true)
		if len(offered) != 0 && !slices.Contains(offered, "http/1.1") {
			s.recordHandshakeFailure(metadata, host, policy, now, metadata.upstreamFailure)
			return true
		}
		protocol = ""
	}
	var downstream []string
	if protocol != "" {
		downstream = []string{protocol}
	}
	peer := &handshakePeer{Conn: client}
	secure := tls.Server(peer, &tls.Config{
		MinVersion: tls.VersionTLS12, NextProtos: downstream,
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
		reason := metadata.upstreamFailure
		if reason == "" {
			reason = tlsHandshakeReason(err, false)
			if rejection := remoteCertificateReason(err); peer.certificateFlight && rejection != "" {
				reason = rejection
			}
		}
		s.recordHandshakeFailure(metadata, host, policy, now, reason)
		return true
	}
	var downstreamConn net.Conn = secure
	if originErr == nil && peer.certificateFlight {
		downstreamConn = &initialTLSRead{Conn: secure, rejected: func(err error) {
			if reason := remoteCertificateReason(err); reason != "" {
				s.recordHandshakeFailure(metadata, host, policy, now, reason)
			}
		}}
	}
	if protocol == "h2" {
		s.serveH2(downstreamConn, origin, metadata, host, originErr)
	} else {
		s.serveHTTPStream(downstreamConn, client.done, origin, metadata, host, originErr)
	}
	return true
}

func (s *session) recordHandshakeFailure(metadata flow, host string, policy *mitmState, now time.Time, reason string) {
	if policy.ctx.Err() != nil || s.ctx.Err() != nil {
		return
	}
	entry := metadata.entry(now, "TLS", host)
	entry.TLS, entry.PassthroughReason, entry.DurationMS = "handshake_failed", reason, time.Since(now).Milliseconds()
	if reason == "certificate_rejected" || reason == "client_ca_untrusted" {
		entry.CAStores = observeCAStores(metadata.sourceApp, policy.ca.public)
		if s.controller.rememberPinned(host, metadata.sourceApp) {
			entry.TLS = "pinned_skipped"
		}
	}
	s.controller.record(s, entry)
}

func alpnMismatch(offered []string, protocol string, err error) bool {
	if err == nil {
		switch protocol {
		case "":
			return len(offered) != 0 && !slices.Contains(offered, "http/1.1")
		case "h2", "http/1.1":
			return !slices.Contains(offered, protocol)
		default:
			return true
		}
	}
	var peerError *net.OpError
	return errors.As(err, &peerError) && peerError.Op == "remote error" && peerError.Err.Error() == tls.AlertError(120).Error() ||
		err.Error() == "tls: server selected unadvertised ALPN protocol" || err.Error() == "tls: server advertised unrequested ALPN extension"
}
