package traffic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
	"golang.org/x/net/netutil"
	"golang.org/x/net/proxy"
)

const maxEntries = 200

var (
	ErrNotRunning = errors.New("抓包代理未启动")
	ErrNoUpstream = errors.New("需要运行中的 sing-box 本机免认证 mixed-in 入站")
)

type Entry struct {
	Time       time.Time `json:"time"`
	Method     string    `json:"method"`
	Host       string    `json:"host"`
	Status     int       `json:"status"`
	Rewrite    string    `json:"rewrite,omitempty"`
	DurationMS int64     `json:"duration_ms"`
}

type Status struct {
	Running bool   `json:"running"`
	Listen  string `json:"listen,omitempty"`
	Count   int    `json:"count"`
}

type Access struct {
	Listen   string `json:"listen"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type Controller struct {
	mu        sync.Mutex
	rulesPath string
	rules     []compiledRule
	session   *session
	entries   [maxEntries]Entry
	count     int
	next      int
}

func New(rulesPath string) *Controller {
	return &Controller{rulesPath: rulesPath}
}

func (c *Controller) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	status := Status{Count: c.count}
	if c.session != nil {
		status.Running, status.Listen = true, c.session.access.Listen
	}
	return status
}

func (c *Controller) Access() (Access, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == nil {
		return Access{}, ErrNotRunning
	}
	return c.session.access, nil
}

func (c *Controller) Entries() []Entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	entries := make([]Entry, c.count)
	for i := range entries {
		entries[i] = c.entries[(c.next+maxEntries-1-i)%maxEntries]
	}
	return entries
}

func (c *Controller) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clearLocked()
}

func (c *Controller) record(owner *session, entry Entry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == owner {
		c.entries[c.next] = entry
		c.next = (c.next + 1) % maxEntries
		c.count = min(c.count+1, maxEntries)
	}
}

func (c *Controller) Rules() ([]Rule, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadRules(); err != nil {
		return nil, err
	}
	rules := make([]Rule, len(c.rules))
	for i := range rules {
		rules[i] = c.rules[i].Rule
	}
	return rules, nil
}

func (c *Controller) ReplaceRules(rules []Rule) error {
	compiled, err := compileRules(rules)
	if err != nil {
		return err
	}
	content, err := json.Marshal(rules)
	if err != nil || len(content) > MaxRulesBytes {
		return errors.New("重写规则超过大小限制")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := provider.WriteAtomic(c.rulesPath, content, 0o600); err != nil {
		return errors.New("无法保存重写规则")
	}
	c.rules = compiled
	return nil
}

func (c *Controller) loadRules() error {
	if c.rules != nil {
		return nil
	}
	file, err := os.Open(c.rulesPath)
	if errors.Is(err, os.ErrNotExist) {
		c.rules = []compiledRule{}
		return nil
	}
	if err != nil {
		return errors.New("无法读取重写规则")
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, MaxRulesBytes+1))
	var rules []Rule
	if err != nil || len(content) > MaxRulesBytes || json.Unmarshal(content, &rules, json.RejectUnknownMembers(true)) != nil || rules == nil {
		return errors.New("无法读取重写规则")
	}
	c.rules, err = compileRules(rules)
	return err
}

func (c *Controller) snapshotRules() []compiledRule {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rules
}

func (c *Controller) Start(upstream string) error {
	address, err := netip.ParseAddrPort(upstream)
	if err != nil || !address.Addr().IsLoopback() || address.Port() == 0 {
		return ErrNoUpstream
	}
	dialer, err := proxy.SOCKS5("tcp", upstream, nil, &net.Dialer{Timeout: 10 * time.Second})
	if err != nil {
		return ErrNoUpstream
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != nil {
		return nil
	}
	if err := c.loadRules(); err != nil {
		return err
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return errors.New("无法生成抓包代理凭据")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return errors.New("无法启动本机抓包代理")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &session{
		controller: c, access: Access{listener.Addr().String(), "netproxy", hex.EncodeToString(token[:])},
		cancel: cancel, slots: make(chan struct{}, 32), scriptSlots: make(chan struct{}, 4),
		dial: dialer.(proxy.ContextDialer).DialContext,
	}
	s.transport = &http.Transport{
		DialContext: s.dial, MaxIdleConns: 16, IdleConnTimeout: time.Minute, ResponseHeaderTimeout: 30 * time.Second,
		MaxResponseHeaderBytes: 64 << 10,
	}
	s.server = &http.Server{
		Handler: s, BaseContext: func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 2 * time.Minute,
		IdleTimeout: time.Minute, MaxHeaderBytes: 32 << 10,
	}
	c.clearLocked()
	c.session = s
	go func() {
		_ = s.server.Serve(netutil.LimitListener(listener, 64))
		s.close()
		c.mu.Lock()
		if c.session == s {
			c.session = nil
		}
		c.mu.Unlock()
	}()
	return nil
}

func (c *Controller) Stop() {
	c.mu.Lock()
	s := c.session
	c.session = nil
	c.mu.Unlock()
	if s != nil {
		s.close()
	}
}

func (c *Controller) clearLocked() {
	c.entries, c.count, c.next = [maxEntries]Entry{}, 0, 0
}

func CoreUpstream(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", ErrNoUpstream
	}
	var config struct {
		Inbounds []struct {
			Type   string           `json:"type"`
			Tag    string           `json:"tag"`
			Listen string           `json:"listen"`
			Port   uint16           `json:"listen_port"`
			Users  []jsontext.Value `json:"users"`
		} `json:"inbounds"`
	}
	if json.Unmarshal(content, &config) != nil {
		return "", ErrNoUpstream
	}
	for _, inbound := range config.Inbounds {
		if inbound.Tag == "mixed-in" && inbound.Type == "mixed" && inbound.Port != 0 && len(inbound.Users) == 0 {
			ip, err := netip.ParseAddr(inbound.Listen)
			if err == nil && ip.IsLoopback() {
				return net.JoinHostPort(ip.String(), strconv.Itoa(int(inbound.Port))), nil
			}
		}
	}
	return "", ErrNoUpstream
}
