# 路由与 DNS

NetProxy 8.0 的分流行为由四层共同决定：

1. `OUTBOUND_MODE` 出站模式。
2. sing-box 路由规则与规则集。
3. eBPF 入站的应用、私网和 CIDR 提前绕过。
4. Wi-Fi 自动策略对运行时模式的临时评估。

## 出站模式

### `rule`

默认模式。由 sing-box 路由规则决定哪些流量直连、代理、拒绝或交给指定出站。

### `global`

尽量全部交给代理出站，适合测试节点或判断规则问题。若 eBPF 仍启用 `EBPF_BYPASS_RULE_SET`，命中的 IP 会在进入 sing-box 前直连，因此 Global 不一定代表绝对全代理。

### `direct`

全部直连，常用于临时停用代理。

### `AllowAds`

使用允许广告的路由策略，在保持主要代理分流的同时放行广告规则所匹配的请求。具体行为以当前 `06_route.json` 为准。

## 规则集位置

```text
/data/adb/modules/netproxy/config/singbox/rules/
├── local/     # block.json、direct.json、proxy.json 等用户规则
└── remote/    # Ads_AWAvenue.json 与 MetaCubeX geosite/geoip 规则集
```

`rule` 模式会同时使用静态路由配置和规则集。远程 JSON/SRS 规则由 sing-box 自动更新，用户编辑器不应修改它们；需要自定义规则时修改 `rules/local/`。

## eBPF 提前绕过

```ini
EBPF_BYPASS_RULE_SET="direct,cn-ip"
```

只有可提取纯 IP CIDR 的规则集会被 eBPF 使用。提前绕过的流量不会进入 sing-box，因此不会再经过 Clash 模式和普通路由规则。进行严格 Global 测试时清空该值并重启服务。

应用黑白名单、私网绕过和共享网络来源过滤也可能在进入普通路由前改变流量路径，排障时需要一并确认。

## DNS

`EBPF_LOCAL_DNS_MODE` 与 `EBPF_SHARED_DNS_MODE` 分别控制两条数据路径是否接管 TCP / UDP 53：

- `hijack`：接管 DNS 请求，交给 sing-box DNS 路由。
- `respect_policy`：仅在流量通过对应数据路径的 UID、来源和地址策略后接管。
- `off`：不由 eBPF 入站接管 DNS。

sing-box 侧 DNS 服务器、域名解析策略和 DNS 路由位于 `config/singbox/confdir/03_dns.json`。默认 DNS A/AAAA 查询使用真实的 `dns-proxy` 服务器组，不使用 FakeIP 地址池。DNS 最终出站由 DNS 配置和 `OUTBOUND_MODE` 共同决定；若将兜底 DNS 设置为直连，解析请求可能不经过代理，这是可预期的配置取舍，不等同于核心故障。

## 排查顺序

1. 查看 `service status` 的实际 `outbound_mode`。
2. 确认 `EBPF_BYPASS_RULE_SET`、私网绕过和应用名单。
3. 检查 `rules/local/` 与 `rules/remote/` 是否存在且可读。
4. 检查 `03_dns.json` 的 DNS 服务器和最终出站。
5. 查看 sing-box 核心日志和 Service API Dashboard 的连接结果。
