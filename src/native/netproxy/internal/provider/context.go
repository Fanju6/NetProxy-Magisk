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

type runtimeOutboundOptionsRegistry struct{}

func (runtimeOutboundOptionsRegistry) OptionTypes() []string {
	return append(outboundOptionsRegistry{}.OptionTypes(),
		C.TypeDirect,
		C.TypeBlock,
		C.TypeSelector,
		C.TypeURLTest,
	)
}

func (runtimeOutboundOptionsRegistry) CreateOptions(outboundType string) (any, bool) {
	switch outboundType {
	case C.TypeDirect:
		return new(option.DirectOutboundOptions), true
	case C.TypeBlock:
		return new(option.StubOptions), true
	case C.TypeSelector:
		return new(option.SelectorOutboundOptions), true
	case C.TypeURLTest:
		return new(option.URLTestOutboundOptions), true
	default:
		return outboundOptionsRegistry{}.CreateOptions(outboundType)
	}
}

type runtimeProviderOptionsRegistry struct{}

func (runtimeProviderOptionsRegistry) OptionTypes() []string {
	return []string{C.ProviderTypeLocal}
}

func (runtimeProviderOptionsRegistry) CreateOptions(providerType string) (any, bool) {
	if providerType == C.ProviderTypeLocal {
		return new(option.ProviderLocalOptions), true
	}
	return nil, false
}

// Context registers only the option types required to parse provider documents.
// It intentionally avoids importing the sing-box runtime and protocol engines.
func Context(ctx context.Context) context.Context {
	ctx = service.ContextWith[option.OutboundOptionsRegistry](ctx, outboundOptionsRegistry{})
	ctx = service.ContextWith[option.EndpointOptionsRegistry](ctx, endpointOptionsRegistry{})
	return ctx
}

// RuntimeContext 注册 NetProxy 生成运行时 Provider 和分组出站所需的类型。
func RuntimeContext(ctx context.Context) context.Context {
	ctx = Context(ctx)
	ctx = service.ContextWith[option.OutboundOptionsRegistry](ctx, runtimeOutboundOptionsRegistry{})
	ctx = service.ContextWith[option.ProviderOptionsRegistry](ctx, runtimeProviderOptionsRegistry{})
	return ctx
}
