package sharelink

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

type Result struct {
	Tag      string `json:"tag"`
	Protocol string `json:"protocol"`
	Link     string `json:"link"`
}

func Export(document provider.Document, tag string) (Result, error) {
	selected, found := provider.Select(document, tag)
	if !found {
		return Result{}, fmt.Errorf("未找到节点标签 %q", tag)
	}
	if len(selected.Outbounds) == 0 {
		return Result{}, errors.New("该节点类型暂不支持导出为分享链接")
	}
	outbound := selected.Outbounds[0]
	link, err := exportOutbound(outbound)
	if err != nil {
		return Result{}, err
	}
	return Result{Tag: outbound.Tag, Protocol: outbound.Type, Link: link}, nil
}

func exportOutbound(outbound option.Outbound) (string, error) {
	switch options := outbound.Options.(type) {
	case *option.VLESSOutboundOptions:
		query := commonQuery(options.TLS, options.Transport)
		query.Set("encryption", "none")
		setIfNotEmpty(query, "flow", options.Flow)
		return buildURL("vless", options.UUID, "", options.ServerOptions, outbound.Tag, query), nil
	case *option.VMessOutboundOptions:
		return exportVMess(outbound.Tag, options)
	case *option.ShadowsocksOutboundOptions:
		query := url.Values{}
		if options.Plugin != "" {
			plugin := options.Plugin
			if options.PluginOptions != "" {
				plugin += ";" + options.PluginOptions
			}
			query.Set("plugin", plugin)
		}
		credentials := base64.RawURLEncoding.EncodeToString([]byte(options.Method + ":" + options.Password))
		return buildURL("ss", credentials, "", options.ServerOptions, outbound.Tag, query), nil
	case *option.TrojanOutboundOptions:
		query := commonQuery(options.TLS, options.Transport)
		if query.Get("security") == "" {
			query.Set("security", "tls")
		}
		return buildURL("trojan", options.Password, "", options.ServerOptions, outbound.Tag, query), nil
	case *option.SOCKSOutboundOptions:
		return buildURL("socks", options.Username, options.Password, options.ServerOptions, outbound.Tag, nil), nil
	case *option.HTTPOutboundOptions:
		scheme := "http"
		if options.TLS != nil && options.TLS.Enabled {
			scheme = "https"
		}
		return buildURL(scheme, options.Username, options.Password, options.ServerOptions, outbound.Tag, nil), nil
	case *option.Hysteria2OutboundOptions:
		query := tlsQuery(options.TLS)
		setIfPositive(query, "up", options.UpMbps)
		setIfPositive(query, "down", options.DownMbps)
		if options.Obfs != nil {
			setIfNotEmpty(query, "obfs", options.Obfs.Type)
			setIfNotEmpty(query, "obfs-password", options.Obfs.Password)
		}
		if len(options.ServerPorts) > 0 {
			query.Set("mport", strings.Join([]string(options.ServerPorts), ","))
		}
		if options.HopInterval > 0 {
			query.Set("mportHopInt", time.Duration(options.HopInterval).String())
		}
		return buildURL("hysteria2", options.Password, "", options.ServerOptions, outbound.Tag, query), nil
	case *option.AnyTLSOutboundOptions:
		return buildURL("anytls", options.Password, "", options.ServerOptions, outbound.Tag, tlsQuery(options.TLS)), nil
	case *option.TUICOutboundOptions:
		query := tlsQuery(options.TLS)
		setIfNotEmpty(query, "congestion_control", options.CongestionControl)
		setIfNotEmpty(query, "udp_relay_mode", options.UDPRelayMode)
		setIfTrue(query, "udp_over_stream", options.UDPOverStream)
		setIfTrue(query, "zero_rtt_handshake", options.ZeroRTTHandshake)
		if options.Heartbeat > 0 {
			query.Set("heartbeat_interval", time.Duration(options.Heartbeat).String())
		}
		return buildURL("tuic", options.UUID, options.Password, options.ServerOptions, outbound.Tag, query), nil
	default:
		return "", fmt.Errorf("协议 %q 暂不支持导出为分享链接", outbound.Type)
	}
}

func exportVMess(tag string, options *option.VMessOutboundOptions) (string, error) {
	type vmessLink struct {
		Version  string `json:"v"`
		Name     string `json:"ps"`
		Server   string `json:"add"`
		Port     string `json:"port"`
		UUID     string `json:"id"`
		AlterID  string `json:"aid"`
		Security string `json:"scy"`
		Network  string `json:"net"`
		Type     string `json:"type"`
		Host     string `json:"host"`
		Path     string `json:"path"`
		TLS      string `json:"tls"`
		SNI      string `json:"sni"`
		ALPN     string `json:"alpn"`
		FP       string `json:"fp"`
		Insecure string `json:"insecure,omitempty"`
	}
	link := vmessLink{
		Version: "2", Name: tag, Server: options.Server, Port: strconv.Itoa(int(options.ServerPort)),
		UUID: options.UUID, AlterID: strconv.Itoa(options.AlterId), Security: options.Security, Network: "tcp",
	}
	if options.Transport != nil {
		switch options.Transport.Type {
		case C.V2RayTransportTypeWebsocket:
			link.Network = "ws"
			link.Path = withEarlyData(options.Transport.WebsocketOptions.Path, options.Transport.WebsocketOptions.MaxEarlyData)
			link.Host = firstHeader(options.Transport.WebsocketOptions.Headers, "Host")
		case C.V2RayTransportTypeHTTP:
			link.Network = "h2"
			link.Path = options.Transport.HTTPOptions.Path
			link.Host = strings.Join([]string(options.Transport.HTTPOptions.Host), ",")
		case C.V2RayTransportTypeGRPC:
			link.Network = "grpc"
			link.Host = options.Transport.GRPCOptions.ServiceName
		case C.V2RayTransportTypeHTTPUpgrade:
			link.Network = "httpupgrade"
			link.Path = options.Transport.HTTPUpgradeOptions.Path
			link.Host = options.Transport.HTTPUpgradeOptions.Host
		default:
			return "", fmt.Errorf("VMess 传输类型 %q 暂不支持导出", options.Transport.Type)
		}
	}
	if options.TLS != nil && options.TLS.Enabled {
		link.TLS = "tls"
		link.SNI = options.TLS.ServerName
		link.ALPN = strings.Join([]string(options.TLS.ALPN), ",")
		if options.TLS.UTLS != nil && options.TLS.UTLS.Enabled {
			link.FP = options.TLS.UTLS.Fingerprint
		}
		if options.TLS.Insecure {
			link.Insecure = "1"
		}
	}
	content, err := json.Marshal(link)
	if err != nil {
		return "", err
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(content), nil
}

func commonQuery(tls *option.OutboundTLSOptions, transport *option.V2RayTransportOptions) url.Values {
	query := tlsQuery(tls)
	if transport == nil {
		query.Set("type", "tcp")
		return query
	}
	query.Set("type", transport.Type)
	switch transport.Type {
	case C.V2RayTransportTypeWebsocket:
		setIfNotEmpty(query, "host", firstHeader(transport.WebsocketOptions.Headers, "Host"))
		setIfNotEmpty(query, "path", withEarlyData(transport.WebsocketOptions.Path, transport.WebsocketOptions.MaxEarlyData))
	case C.V2RayTransportTypeHTTP:
		query.Set("type", "http")
		setIfNotEmpty(query, "host", strings.Join([]string(transport.HTTPOptions.Host), ","))
		setIfNotEmpty(query, "path", transport.HTTPOptions.Path)
	case C.V2RayTransportTypeGRPC:
		setIfNotEmpty(query, "serviceName", transport.GRPCOptions.ServiceName)
		setIfNotEmpty(query, "grpc-service-name", transport.GRPCOptions.ServiceName)
	case C.V2RayTransportTypeHTTPUpgrade:
		setIfNotEmpty(query, "host", transport.HTTPUpgradeOptions.Host)
		setIfNotEmpty(query, "path", transport.HTTPUpgradeOptions.Path)
	}
	return query
}

func tlsQuery(tls *option.OutboundTLSOptions) url.Values {
	query := url.Values{}
	if tls == nil || !tls.Enabled {
		return query
	}
	security := "tls"
	if tls.Reality != nil && tls.Reality.Enabled {
		security = "reality"
		setIfNotEmpty(query, "pbk", tls.Reality.PublicKey)
		setIfNotEmpty(query, "sid", tls.Reality.ShortID)
	}
	query.Set("security", security)
	setIfNotEmpty(query, "sni", tls.ServerName)
	setIfNotEmpty(query, "alpn", strings.Join([]string(tls.ALPN), ","))
	if tls.UTLS != nil && tls.UTLS.Enabled {
		setIfNotEmpty(query, "fp", tls.UTLS.Fingerprint)
	}
	setIfTrue(query, "insecure", tls.Insecure)
	setIfTrue(query, "disable_sni", tls.DisableSNI)
	return query
}

func buildURL(scheme, username, password string, server option.ServerOptions, tag string, query url.Values) string {
	link := &url.URL{
		Scheme:   scheme,
		Host:     net.JoinHostPort(server.Server, strconv.Itoa(int(server.ServerPort))),
		Fragment: tag,
	}
	if username != "" || password != "" {
		if password != "" {
			link.User = url.UserPassword(username, password)
		} else {
			link.User = url.User(username)
		}
	}
	if len(query) > 0 {
		link.RawQuery = query.Encode()
	}
	return link.String()
}

func firstHeader(headers badoption.HTTPHeader, key string) string {
	for name, values := range headers {
		if strings.EqualFold(name, key) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func withEarlyData(path string, maxEarlyData uint32) string {
	if maxEarlyData == 0 {
		return path
	}
	if path == "" {
		path = "/"
	}
	return path + "?ed=" + strconv.FormatUint(uint64(maxEarlyData), 10)
}

func setIfNotEmpty(query url.Values, key, value string) {
	if value != "" {
		query.Set(key, value)
	}
}

func setIfPositive(query url.Values, key string, value int) {
	if value > 0 {
		query.Set(key, strconv.Itoa(value))
	}
}

func setIfTrue(query url.Values, key string, value bool) {
	if value {
		query.Set(key, "1")
	}
}
