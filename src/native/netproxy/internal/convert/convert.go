package convert

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	providerparser "github.com/sagernet/sing-box/provider/parser"
	"gopkg.in/yaml.v3"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

type DiagnosticsError struct {
	Diagnostics []provider.Diagnostic
}

func (e *DiagnosticsError) Error() string {
	if len(e.Diagnostics) == 0 {
		return "no nodes found"
	}
	return fmt.Sprintf("no nodes found: %s", e.Diagnostics[0].Message)
}

func Link(ctx context.Context, link string, allowInsecure bool) (provider.ParseResult, error) {
	outbound, err := parseLink(strings.TrimSpace(link))
	if err != nil {
		diagnostic := provider.Diagnostic{Source: sourceLabel(link), Code: "link.invalid", Message: err.Error()}
		return provider.ParseResult{Diagnostics: []provider.Diagnostic{diagnostic}}, &DiagnosticsError{Diagnostics: []provider.Diagnostic{diagnostic}}
	}
	document := provider.Document{Outbounds: []option.Outbound{outbound}}
	provider.NormalizeTags(&document)
	if allowInsecure {
		applyAllowInsecure(&document)
	}
	if err := provider.Validate(document); err != nil {
		diagnostic := provider.Diagnostic{Source: sourceLabel(link), Code: "link.invalid", Message: err.Error()}
		return provider.ParseResult{Diagnostics: []provider.Diagnostic{diagnostic}}, &DiagnosticsError{Diagnostics: []provider.Diagnostic{diagnostic}}
	}
	return provider.ParseResult{Document: document}, nil
}

func Content(ctx context.Context, content string, allowInsecure bool) (provider.ParseResult, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		diagnostic := provider.Diagnostic{Code: "input.empty", Message: "input is empty"}
		return provider.ParseResult{Diagnostics: []provider.Diagnostic{diagnostic}}, &DiagnosticsError{Diagnostics: []provider.Diagnostic{diagnostic}}
	}
	ctx = provider.Context(ctx)

	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		if outbounds, endpoints, err := providerparser.ParseBoxSubscription(ctx, trimmed); err == nil && len(outbounds)+len(endpoints) > 0 {
			return finish(provider.Document{Outbounds: outbounds, Endpoints: endpoints}, nil, allowInsecure)
		}
		if outbounds, endpoints, err := providerparser.ParseSIP008Subscription(ctx, trimmed); err == nil && len(outbounds)+len(endpoints) > 0 {
			return finish(provider.Document{Outbounds: outbounds, Endpoints: endpoints}, nil, allowInsecure)
		}
	}

	if looksLikeClash(trimmed) {
		return parseClash(ctx, trimmed, allowInsecure)
	}

	return parseRaw(ctx, trimmed, allowInsecure)
}

func parseClash(ctx context.Context, content string, allowInsecure bool) (provider.ParseResult, error) {
	var config providerparser.ClashConfig
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		diagnostic := provider.Diagnostic{Code: "clash.invalid", Message: err.Error()}
		return provider.ParseResult{Diagnostics: []provider.Diagnostic{diagnostic}}, &DiagnosticsError{Diagnostics: []provider.Diagnostic{diagnostic}}
	}
	var diagnostics []provider.Diagnostic
	for index, proxy := range config.Proxies {
		if proxy.SingType == "" {
			diagnostics = append(diagnostics, provider.Diagnostic{
				Index:   index + 1,
				Source:  proxy.Name,
				Code:    "clash.protocol_unsupported",
				Message: fmt.Sprintf("unsupported Clash protocol %q", proxy.Type),
			})
		}
	}
	outbounds, endpoints, err := providerparser.ParseClashSubscription(ctx, content)
	if err != nil {
		diagnostics = append(diagnostics, provider.Diagnostic{Code: "clash.invalid", Message: err.Error()})
		return provider.ParseResult{Diagnostics: diagnostics}, &DiagnosticsError{Diagnostics: diagnostics}
	}
	return finish(provider.Document{Outbounds: outbounds, Endpoints: endpoints}, diagnostics, allowInsecure)
}

func parseRaw(ctx context.Context, content string, allowInsecure bool) (provider.ParseResult, error) {
	if decoded, ok := decodeSubscription(content); ok {
		content = decoded
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	var document provider.Document
	var diagnostics []provider.Diagnostic
	for index, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "# ") {
			continue
		}
		outbound, err := parseLink(line)
		if err != nil {
			diagnostics = append(diagnostics, provider.Diagnostic{
				Index:   index + 1,
				Source:  sourceLabel(line),
				Code:    "link.invalid",
				Message: err.Error(),
			})
			continue
		}
		document.Outbounds = append(document.Outbounds, outbound)
	}
	return finish(document, diagnostics, allowInsecure)
}

func finish(document provider.Document, diagnostics []provider.Diagnostic, allowInsecure bool) (provider.ParseResult, error) {
	provider.NormalizeTags(&document)
	if allowInsecure {
		applyAllowInsecure(&document)
	}
	if len(document.Outbounds)+len(document.Endpoints) == 0 {
		if len(diagnostics) == 0 {
			diagnostics = append(diagnostics, provider.Diagnostic{Code: "input.no_nodes", Message: "no supported nodes found"})
		}
		return provider.ParseResult{Diagnostics: diagnostics}, &DiagnosticsError{Diagnostics: diagnostics}
	}
	if err := provider.Validate(document); err != nil {
		diagnostics = append(diagnostics, provider.Diagnostic{Code: "provider.invalid", Message: err.Error()})
		return provider.ParseResult{Diagnostics: diagnostics}, &DiagnosticsError{Diagnostics: diagnostics}
	}
	return provider.ParseResult{Document: document, Diagnostics: diagnostics}, nil
}

func parseLink(link string) (option.Outbound, error) {
	if link == "" {
		return option.Outbound{}, errors.New("empty link")
	}
	schemeEnd := strings.Index(link, "://")
	if schemeEnd <= 0 {
		return option.Outbound{}, errors.New("missing URI scheme")
	}
	scheme := strings.ToLower(link[:schemeEnd])
	switch scheme {
	case "socks", "socks5":
		return parseSOCKS(link)
	case "http", "https":
		return parseHTTP(link)
	default:
		return providerparser.ParseSubscriptionLink(link)
	}
}

func parseSOCKS(link string) (option.Outbound, error) {
	schemeEnd := strings.Index(link, "://")
	body := link[schemeEnd+3:]
	fragment := ""
	if before, after, found := strings.Cut(body, "#"); found {
		body = before
		decoded, err := url.QueryUnescape(after)
		if err == nil {
			fragment = decoded
		}
	}
	u, err := parseSOCKSURL(body, true)
	if err != nil {
		return option.Outbound{}, err
	}
	port, err := parsePort(u.Port())
	if err != nil {
		return option.Outbound{}, err
	}
	options := &option.SOCKSOutboundOptions{
		ServerOptions: option.ServerOptions{Server: u.Hostname(), ServerPort: port},
		Version:       "5",
	}
	if u.User != nil {
		options.Username = u.User.Username()
		options.Password, _ = u.User.Password()
		if options.Password == "" {
			if decoded, ok := decodeUserInfo(options.Username); ok {
				options.Username, options.Password, _ = strings.Cut(decoded, ":")
			}
		}
	}
	return option.Outbound{Type: C.TypeSOCKS, Tag: fragment, Options: options}, nil
}

func parseSOCKSURL(body string, allowLegacyBase64 bool) (*url.URL, error) {
	u, err := url.Parse("socks://" + body)
	if err == nil && u.Hostname() != "" && u.Port() != "" {
		return u, nil
	}
	if !allowLegacyBase64 {
		if err != nil {
			return nil, fmt.Errorf("invalid SOCKS link: %w", err)
		}
		return nil, errors.New("SOCKS link requires a server and port")
	}
	decoded, ok := decodeBase64(body)
	if !ok || decoded == body {
		if err != nil {
			return nil, fmt.Errorf("invalid SOCKS link: %w", err)
		}
		return nil, errors.New("SOCKS link requires a server and port")
	}
	return parseSOCKSURL(decoded, false)
}

func parseHTTP(link string) (option.Outbound, error) {
	u, err := url.Parse(link)
	if err != nil {
		return option.Outbound{}, fmt.Errorf("invalid HTTP proxy link: %w", err)
	}
	if u.Hostname() == "" {
		return option.Outbound{}, errors.New("HTTP proxy link requires a server")
	}
	port, err := parsePort(u.Port())
	if err != nil {
		return option.Outbound{}, err
	}
	options := &option.HTTPOutboundOptions{
		ServerOptions: option.ServerOptions{Server: u.Hostname(), ServerPort: port},
	}
	if u.User != nil {
		options.Username = u.User.Username()
		options.Password, _ = u.User.Password()
	}
	if strings.EqualFold(u.Scheme, "https") {
		options.TLS = &option.OutboundTLSOptions{Enabled: true, ServerName: u.Hostname()}
	}
	return option.Outbound{Type: C.TypeHTTP, Tag: u.Fragment, Options: options}, nil
}

func parsePort(value string) (uint16, error) {
	port, err := strconv.ParseUint(value, 10, 16)
	if err != nil || port == 0 {
		return 0, fmt.Errorf("invalid server port %q", value)
	}
	return uint16(port), nil
}

func decodeSubscription(content string) (string, bool) {
	compact := strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, content)
	decoded, ok := decodeBase64(compact)
	if !ok || !strings.Contains(decoded, "://") {
		return content, false
	}
	return decoded, true
}

func decodeBase64(value string) (string, bool) {
	encodings := []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	}
	for _, encoding := range encodings {
		decoded, err := encoding.DecodeString(value)
		if err == nil {
			return string(decoded), true
		}
	}
	return "", false
}

func decodeUserInfo(value string) (string, bool) {
	decoded, ok := decodeBase64(value)
	return decoded, ok && strings.Contains(decoded, ":")
}

func sourceLabel(input string) string {
	input = strings.TrimSpace(input)
	if end := strings.Index(input, "://"); end > 0 {
		return strings.ToLower(input[:end]) + "://..."
	}
	return "input"
}

func looksLikeClash(content string) bool {
	return strings.Contains(content, "\nproxies:") || strings.HasPrefix(content, "proxies:")
}

func applyAllowInsecure(document *provider.Document) {
	for index := range document.Outbounds {
		switch options := document.Outbounds[index].Options.(type) {
		case *option.AnyTLSOutboundOptions:
			setInsecure(&options.OutboundTLSOptionsContainer)
		case *option.HTTPOutboundOptions:
			setInsecure(&options.OutboundTLSOptionsContainer)
		case *option.HysteriaOutboundOptions:
			setInsecure(&options.OutboundTLSOptionsContainer)
		case *option.Hysteria2OutboundOptions:
			setInsecure(&options.OutboundTLSOptionsContainer)
		case *option.NaiveOutboundOptions:
			setInsecure(&options.OutboundTLSOptionsContainer)
		case *option.ShadowTLSOutboundOptions:
			setInsecure(&options.OutboundTLSOptionsContainer)
		case *option.TrojanOutboundOptions:
			setInsecure(&options.OutboundTLSOptionsContainer)
		case *option.TUICOutboundOptions:
			setInsecure(&options.OutboundTLSOptionsContainer)
		case *option.VLESSOutboundOptions:
			setInsecure(&options.OutboundTLSOptionsContainer)
		case *option.VMessOutboundOptions:
			setInsecure(&options.OutboundTLSOptionsContainer)
		}
	}
}

func setInsecure(container *option.OutboundTLSOptionsContainer) {
	if container.TLS != nil && container.TLS.Enabled {
		container.TLS.Insecure = true
	}
}
