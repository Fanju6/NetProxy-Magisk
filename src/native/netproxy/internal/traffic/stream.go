package traffic

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var errBridge = errors.New("无效的透明抓包连接")

type flow struct {
	Destination string `json:"destination"`
	Host        string `json:"host"`
	Outbound    string `json:"outbound"`
	Exit        *Exit  `json:"exit,omitempty"`
	sourceApp   *SourceApp
}

type Exit struct {
	Inbound string    `json:"inbound,omitempty"`
	Chain   []ExitHop `json:"chain,omitempty"`
	Remote  string    `json:"remote,omitempty"`
	Package string    `json:"package,omitempty"`
	UID     *uint32   `json:"uid,omitempty"`
	Process string    `json:"process,omitempty"`
}

type ExitHop struct {
	Tag  string `json:"tag"`
	Type string `json:"type"`
}

func (f flow) entry(now time.Time, method, host string) Entry {
	return Entry{Time: now, Method: method, Host: host, Outbound: f.Outbound, Exit: f.Exit, SourceApp: f.sourceApp}
}

type session struct {
	controller  *Controller
	listener    *net.UnixListener
	ctx         context.Context
	cancel      context.CancelFunc
	slots       chan struct{}
	scriptSlots chan struct{}
}

func (s *session) close() {
	s.cancel()
	_ = s.listener.Close()
}

func (s *session) serve() {
	for {
		bridge, err := s.listener.AcceptUnix()
		if err != nil {
			return
		}
		select {
		case s.slots <- struct{}{}:
			go func() {
				defer func() { <-s.slots }()
				defer bridge.Close()
				s.serveBridge(bridge)
			}()
		default:
			_ = bridge.Close()
		}
	}
}

func (s *session) serveBridge(bridge *net.UnixConn) {
	upstream, metadata, err := readBridge(bridge)
	if err != nil {
		return
	}
	defer upstream.Close()
	metadata.sourceApp = s.controller.owners.sourceApp(s.ctx, metadata.Exit)
	if !s.controller.capturesApp(metadata.sourceApp) {
		return
	}
	defer context.AfterFunc(s.ctx, func() {
		_ = bridge.Close()
		_ = upstream.Close()
	})()
	if _, err := bridge.Write([]byte{1}); err != nil {
		return
	}
	_ = bridge.SetDeadline(time.Time{})
	client := &bufferedStream{Conn: bridge, reader: bufio.NewReader(bridge), done: make(chan struct{})}
	now := time.Now()
	_ = client.SetReadDeadline(now.Add(250 * time.Millisecond))
	isHTTP := sniffHTTP(client.reader)
	policy := s.controller.currentMITM()
	host := ""
	if !isHTTP && policy != nil {
		host = sniffTLS(s.ctx, client, metadata)
	}
	_ = client.SetReadDeadline(time.Time{})
	if isHTTP {
		s.serveHTTPStream(client, client.done, upstream, metadata, "http", nil)
		return
	}
	entry := metadata.entry(now, "TCP", metadata.Destination)
	if host != "" && policy.matches(host) {
		if !policy.bypass(host, now) && s.intercept(client, upstream, metadata, host, policy, now) {
			return
		}
		entry.TLS = "bypass"
	}
	s.controller.record(s, entry)
	relayStreams(client, upstream)
}

func sniffHTTP(reader *bufio.Reader) bool {
	for i := 1; i <= reader.Size(); i++ {
		line, err := reader.Peek(i)
		if err != nil {
			return false
		}
		last := line[i-1]
		if last == '\n' {
			method, path, ok := strings.Cut(string(line), " ")
			return ok && method != http.MethodConnect && strings.HasPrefix(path, "/") &&
				(strings.HasSuffix(path, " HTTP/1.1\r\n") || strings.HasSuffix(path, " HTTP/1.0\r\n"))
		}
		if i == 1 && (last < 'A' || last > 'Z') {
			return false
		}
	}
	return false
}

func (s *session) serveHTTPStream(client net.Conn, done <-chan struct{}, upstream net.Conn, metadata flow, scheme string, upstreamTLS *tls.Config) {
	var dialed atomic.Bool
	transport := &http.Transport{
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			if dialed.Swap(true) {
				return nil, io.EOF
			}
			return upstream, nil
		},
		TLSClientConfig: upstreamTLS, TLSHandshakeTimeout: 10 * time.Second, DisableKeepAlives: true,
		ResponseHeaderTimeout: 30 * time.Second, MaxResponseHeaderBytes: 64 << 10,
	}
	defer transport.CloseIdleConnections()
	server := &http.Server{
		BaseContext:       func(net.Listener) context.Context { return s.ctx },
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 2 * time.Minute, MaxHeaderBytes: 32 << 10,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodConnect || r.URL.IsAbs() || r.URL.Host != "" {
				http.Error(w, "Invalid origin request", http.StatusBadRequest)
				return
			}
			if r.Header.Get("Upgrade") != "" {
				if upstreamTLS != nil {
					http.Error(w, "HTTPS protocol upgrade is not supported", http.StatusNotImplemented)
					return
				}
				if conn, rw, err := http.NewResponseController(w).Hijack(); err == nil {
					defer conn.Close()
					if err := r.Write(upstream); err == nil {
						relayStreams(&bufferedStream{Conn: conn, reader: rw.Reader, done: make(chan struct{})}, upstream)
					}
				}
				return
			}
			r.URL.Scheme, r.URL.Host = scheme, r.Host
			if !validTarget(r.URL) {
				http.Error(w, "Invalid origin request", http.StatusBadRequest)
				return
			}
			if upstreamTLS != nil && canonicalHost(r.URL.Hostname()) != upstreamTLS.ServerName {
				http.Error(w, "Request does not match TLS host", http.StatusMisdirectedRequest)
				return
			}
			w.Header().Set("Connection", "close")
			s.proxyHTTP(w, r, time.Now(), transport, metadata)
		}),
	}
	server.SetKeepAlivesEnabled(false)
	_ = server.Serve(&flowListener{conn: client, done: done})
}

type bufferedStream struct {
	net.Conn
	reader *bufio.Reader
	done   chan struct{}
	once   sync.Once
}

func (c *bufferedStream) Read(p []byte) (int, error) { return c.reader.Read(p) }

func (c *bufferedStream) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { close(c.done) })
	return err
}

func (c *bufferedStream) CloseWrite() error {
	if half, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return half.CloseWrite()
	}
	return c.Close()
}

type flowListener struct {
	conn net.Conn
	done <-chan struct{}
	used bool
}

func (l *flowListener) Accept() (net.Conn, error) {
	if !l.used {
		l.used = true
		return l.conn, nil
	}
	<-l.done
	return nil, net.ErrClosed
}

func (l *flowListener) Addr() net.Addr { return l.conn.LocalAddr() }
func (l *flowListener) Close() error   { return l.conn.Close() }

func relayStreams(a, b net.Conn) {
	var wait sync.WaitGroup
	relay := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		if half, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = half.CloseWrite()
		} else {
			_ = dst.Close()
		}
	}
	wait.Go(func() { relay(a, b) })
	wait.Go(func() { relay(b, a) })
	wait.Wait()
}
