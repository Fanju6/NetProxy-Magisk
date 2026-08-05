package provider

import (
	"context"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/service"
)

type outboundOptionsRegistry struct{}

func (outboundOptionsRegistry) OptionTypes() []string {
	return []string{
		C.TypeAnyTLS,
		C.TypeHTTP,
		C.TypeHysteria,
		C.TypeHysteria2,
		C.TypeNaive,
		C.TypeShadowsocks,
		C.TypeShadowTLS,
		C.TypeSnell,
		C.TypeSOCKS,
		C.TypeSSH,
		C.TypeTor,
		C.TypeTrojan,
		C.TypeTUIC,
		C.TypeVLESS,
		C.TypeVMess,
	}
}

func (outboundOptionsRegistry) CreateOptions(outboundType string) (any, bool) {
	switch outboundType {
	case C.TypeAnyTLS:
		return new(option.AnyTLSOutboundOptions), true
	case C.TypeHTTP:
		return new(option.HTTPOutboundOptions), true
	case C.TypeHysteria:
		return new(option.HysteriaOutboundOptions), true
	case C.TypeHysteria2:
		return new(option.Hysteria2OutboundOptions), true
	case C.TypeNaive:
		return new(option.NaiveOutboundOptions), true
	case C.TypeShadowsocks:
		return new(option.ShadowsocksOutboundOptions), true
	case C.TypeShadowTLS:
		return new(option.ShadowTLSOutboundOptions), true
	case C.TypeSnell:
		return new(option.SnellOutboundOptions), true
	case C.TypeSOCKS:
		return new(option.SOCKSOutboundOptions), true
	case C.TypeSSH:
		return new(option.SSHOutboundOptions), true
	case C.TypeTor:
		return new(option.TorOutboundOptions), true
	case C.TypeTrojan:
		return new(option.TrojanOutboundOptions), true
	case C.TypeTUIC:
		return new(option.TUICOutboundOptions), true
	case C.TypeVLESS:
		return new(option.VLESSOutboundOptions), true
	case C.TypeVMess:
		return new(option.VMessOutboundOptions), true
	default:
		return nil, false
	}
}

type endpointOptionsRegistry struct{}

func (endpointOptionsRegistry) OptionTypes() []string {
	return []string{C.TypeTailscale, C.TypeWireGuard}
}

func (endpointOptionsRegistry) CreateOptions(endpointType string) (any, bool) {
	switch endpointType {
	case C.TypeTailscale:
		return new(option.TailscaleEndpointOptions), true
	case C.TypeWireGuard:
		return new(option.WireGuardEndpointOptions), true
	default:
		return nil, false
	}
}

// Context registers only the option types required to parse provider documents.
// It intentionally avoids importing the sing-box runtime and protocol engines.
func Context(ctx context.Context) context.Context {
	ctx = service.ContextWith[option.OutboundOptionsRegistry](ctx, outboundOptionsRegistry{})
	ctx = service.ContextWith[option.EndpointOptionsRegistry](ctx, endpointOptionsRegistry{})
	return ctx
}
