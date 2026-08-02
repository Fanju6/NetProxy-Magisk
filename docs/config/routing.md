# 路由与 DNS

NetProxy 的分流行为由三层共同决定：

1. `OUTBOUND_MODE`
2. sing-box 路由规则与规则集
3. eBPF 入站的规则集与 UID 提前绕过

## 出站模式

### `rule`

默认模式。
由 sing-box 的路由规则决定哪些流量直连、哪些流量代理、哪些流量拦截或交给特定出站。

### `global`

尽量全局走代理，适合测试节点或快速确认是否为规则问题。

### `direct`

全局直连，常用于临时停用代理但保留模块与规则结构。

## 规则集位置

```text
/data/adb/modules/netproxy/config/singbox/source/
```

这里存放规则集和相关资源，`rule` 模式下会被 sing-box 路由配置引用。

## 与透明代理层的关系

eBPF 入站先在内核侧判断需要提前绕过的 CIDR 与 UID，sing-box 再决定其余流量如何分流。

典型例子：

- `APP_PROXY_ENABLE` 控制是否启用分应用代理
- `APP_PROXY_MODE` 决定应用名单是黑名单还是白名单
- `EBPF_BYPASS_RULE_SETS` 指定可提取 IP CIDR 并在内核侧提前绕过的规则集

提前绕过的流量不会进入 sing-box，因此也不会再经过 Clash 模式和普通路由规则。需要严格 Global 行为时，应清空 `EBPF_BYPASS_RULE_SETS` 后重启服务。

## DNS 相关

`ebpf.conf` 中的 `EBPF_DNS_MODE` 决定 eBPF 入站是否优先接管 TCP / UDP 53；sing-box 侧的解析和分流行为仍由 `confdir/` 中的 DNS 配置控制。

如果出现域名能解析但分流异常，请同时检查：

1. 当前 `OUTBOUND_MODE`
2. `source/` 中的规则集是否正确
3. `EBPF_DNS_MODE` 与 sing-box DNS 配置
4. 当前节点和代理组是否正常
