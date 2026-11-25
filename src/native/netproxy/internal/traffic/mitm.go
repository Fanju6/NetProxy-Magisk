package traffic

import (
	"context"
	"crypto/tls"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

const (
	MaxMITMBytes   = 32 << 10
	maxCachedHosts = 128
)

type MITMPolicy struct {
	Enabled     bool     `json:"enabled"`
	Hosts       []string `json:"hosts"`
	BypassHosts []string `json:"bypass_hosts"`
	SystemTrust bool     `json:"system_trust"`
}

type mitmState struct {
	ctx         context.Context
	cancel      context.CancelFunc
	ca          *authority
	hosts       []string
	bypassHosts []string
	mu          sync.Mutex
	leaves      map[string]tls.Certificate
}

func loadPolicy(layout paths.Layout) (MITMPolicy, error) {
	f, err := os.Open(layout.TrafficMITM())
	if errors.Is(err, os.ErrNotExist) {
		return normalizePolicy(MITMPolicy{})
	}
	if err != nil {
		return MITMPolicy{}, errors.New("无法读取 HTTPS 抓包配置")
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, MaxMITMBytes+1))
	var policy *MITMPolicy
	if err != nil || len(content) > MaxMITMBytes || json.Unmarshal(content, &policy, json.RejectUnknownMembers(true)) != nil || policy == nil {
		return MITMPolicy{}, errors.New("HTTPS 抓包配置无效")
	}
	return normalizePolicy(*policy)
}

func normalizePolicy(policy MITMPolicy) (MITMPolicy, error) {
	if len(policy.Hosts) == 0 {
		policy.Hosts = []string{"*"}
	}
	var err error
	if policy.Hosts, err = normalizeHostPatterns(policy.Hosts); err != nil {
		return MITMPolicy{}, err
	}
	if policy.BypassHosts, err = normalizeHostPatterns(policy.BypassHosts); err != nil {
		return MITMPolicy{}, err
	}
	return policy, nil
}

func normalizeHostPatterns(patterns []string) ([]string, error) {
	if len(patterns) > 100 {
		return nil, errors.New("最多允许 100 条 HTTPS 抓包主机规则")
	}
	hosts := make([]string, 0, len(patterns))
	seen := make(map[string]bool, len(patterns))
	for i, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(pattern), "."))
		host := strings.TrimPrefix(pattern, "*.")
		if pattern != "*" && (!validMITMHost(host) || (host != pattern && netipIsIP(host))) {
			return nil, fmt.Errorf("第 %d 条 HTTPS 抓包主机规则无效", i+1)
		}
		if !seen[pattern] {
			seen[pattern] = true
			hosts = append(hosts, pattern)
		}
	}
	return hosts, nil
}

func netipIsIP(host string) bool {
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.Zone() == ""
}

func validMITMHost(host string) bool {
	if netipIsIP(host) {
		return true
	}
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for label := range strings.SplitSeq(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, ch := range label {
			if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-') {
				return false
			}
		}
	}
	return true
}

func newMITM(policy MITMPolicy, ca *authority) *mitmState {
	if !policy.Enabled {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &mitmState{ctx: ctx, cancel: cancel, ca: ca, hosts: policy.Hosts, bypassHosts: policy.BypassHosts,
		leaves: make(map[string]tls.Certificate)}
}

func (s *mitmState) matches(host string) bool {
	return s != nil && validMITMHost(host) &&
		matchesHostPatterns(s.hosts, host) && !matchesHostPatterns(s.bypassHosts, host)
}

func matchesHostPatterns(patterns []string, host string) bool {
	for _, pattern := range patterns {
		if pattern == "*" || pattern == host || (strings.HasPrefix(pattern, "*.") && strings.HasSuffix(host, pattern[1:])) {
			return true
		}
	}
	return false
}

func (s *mitmState) certificate(host string, now time.Time) (tls.Certificate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if leaf, ok := s.leaves[host]; ok && now.Add(time.Minute).Before(leaf.Leaf.NotAfter) && !now.Before(leaf.Leaf.NotBefore) {
		return leaf, nil
	}
	leaf, err := s.ca.leaf(host, now)
	if err != nil {
		return leaf, err
	}
	if len(s.leaves) >= maxCachedHosts {
		for old := range s.leaves {
			delete(s.leaves, old)
			break
		}
	}
	s.leaves[host] = leaf
	return leaf, nil
}

func (c *Controller) Certificate(initialize bool) (Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var ca *authority
	var err error
	if initialize {
		ca, err = createCA(c.layout.TrafficCA())
	} else {
		ca, err = loadCA(c.layout.TrafficCA())
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Certificate{}, errors.New("尚未初始化抓包 CA")
		}
		return Certificate{}, err
	}
	return ca.public, nil
}

func (c *Controller) MITMPolicy() (MITMPolicy, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return loadPolicy(c.layout)
}

func (c *Controller) SetMITMPolicy(policy MITMPolicy) (MITMPolicy, error) {
	policy, err := normalizePolicy(policy)
	if err != nil {
		return policy, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var ca *authority
	if policy.Enabled || policy.SystemTrust {
		ca, err = createCA(c.layout.TrafficCA())
		if err != nil {
			return policy, err
		}
	}
	content, err := json.Marshal(policy)
	if err != nil || len(content) > MaxMITMBytes {
		return policy, errors.New("HTTPS 抓包配置超出大小限制")
	}
	if err := provider.WriteAtomic(c.layout.TrafficMITM(), content, 0o600); err != nil {
		return policy, errors.New("无法保存 HTTPS 抓包配置")
	}
	if c.session != nil {
		c.replaceMITM(ca, policy)
	}
	return policy, nil
}

func (c *Controller) replaceMITM(ca *authority, policy MITMPolicy) {
	if c.mitm != nil {
		c.mitm.cancel()
	}
	c.mitm = newMITM(policy, ca)
}

func (c *Controller) currentMITM() *mitmState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mitm
}
