package traffic

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
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

func sniffTLS(ctx context.Context, client *bufferedStream, metadata flow) string {
	first, err := client.reader.Peek(1)
	if err != nil || first[0] != 22 {
		return ""
	}
	var replay bytes.Buffer
	reader := io.TeeReader(io.LimitReader(client.reader, 64<<10), &replay)
	host := ""
	probe := tls.Server(helloProbe{client, reader}, &tls.Config{
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			if len(hello.SupportedProtos) == 0 || slices.Contains(hello.SupportedProtos, "http/1.1") {
				host = hello.ServerName
				if host == "" {
					host, _, _ = net.SplitHostPort(metadata.Destination)
				}
			}
			return nil, errBridge
		},
	})
	_ = probe.HandshakeContext(ctx)
	client.reader = bufio.NewReader(io.MultiReader(&replay, client.reader))
	return canonicalHost(host)
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
	secure := tls.Server(client, &tls.Config{
		MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"},
		GetCertificate: func(info *tls.ClientHelloInfo) (*tls.Certificate, error) {
			if info.ServerName != "" && canonicalHost(info.ServerName) != host {
				return nil, errors.New("TLS ServerName mismatch")
			}
			return &leaf, nil
		},
	})
	handshake, cancel := context.WithTimeout(s.ctx, 12*time.Second)
	err = secure.HandshakeContext(handshake)
	cancel()
	if err != nil {
		if policy.ctx.Err() == nil && s.ctx.Err() == nil {
			policy.rememberFailure(host, time.Now())
			entry := metadata.entry(now, "TLS", host)
			entry.TLS, entry.Status, entry.DurationMS = "handshake_failed", http.StatusBadGateway, time.Since(now).Milliseconds()
			s.controller.record(s, entry)
		}
		return true
	}
	s.serveHTTPStream(secure, client.done, upstream, metadata, "https", &tls.Config{
		MinVersion: tls.VersionTLS12, ServerName: host, RootCAs: s.controller.upstreamRoots,
	})
	return true
}
