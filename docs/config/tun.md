---
description: NetProxy TUN 入站的原生参数、接口筛选、DNS 接管、路由范围与 pre-match 绕过规则。
---

# TUN 原生参数

TUN 是 sing-box 的透明代理入站，在 NetProxy 中通过 Root 权限运行 `auto_route` 与 `auto_redirect`，接管本机及共享网络流量。参数保存在 `config/inbound/inbound.json` 的 `tun` 对象；切换方法见 [入站配置](/config/inbound)。

## 默认参数

```json
{
  "tun": {
    "type": "tun",
    "tag": "netproxy-in",
    "interface_name": "netproxy",
    "address": ["172.19.0.1/30", "fdfe:dcba:9876::1/126"],
    "auto_route": true,
    "auto_redirect": true,
    "dns_mode": "hijack"
  }
}
```

`type`、`tag`、`auto_route: true` 与 `auto_redirect: true` 是受管 TUN 约束。接口名和有效地址前缀可调整，避免与设备网络冲突。移除 IPv6 地址只改变入站接管，不关闭系统 IPv6。

默认不固定 `mtu`、`strict_route`、marks、NFQUEUE 编号、路由表或规则编号，沿用锁定内核的原生默认；不输出 `stack`，也不提供 system/gvisor/mixed 选择器。主配置默认使用 `route.auto_detect_interface: true` 防止回环，修改后必须满足其他有效的出口绑定条件。`auto_redirect` 与 `default_mark` / `routing_mark` 的真实冲突会在组合检查时失败。

## 常用范围

| 原生字段 | 用途与边界 |
|---|---|
| `dns_mode` | 默认 `hijack`；`disabled` 关闭接管，`native` 留给完整编辑，不代表 Android 系统全局 DNS |
| `dns_address` | 默认由上游推导 |
| `include_interface` / `exclude_interface` | 按实际接口先包含、再排除；不建立 local/shared/hybrid 别名 |
| `route_address` / `route_address_set` | 接管目标 CIDR 或 IP 规则集 |
| `route_exclude_address` / `route_exclude_address_set` | 绕过目标 CIDR 或 IP 规则集 |
| `include_mac_address` / `exclude_mac_address` | 下游 MAC 筛选，两者互斥 |
| `strict_route` | 按原生行为控制严格路由，也影响绑定接口流量，不只是防泄漏 |
| `mtu` / `udp_timeout` | 高级参数，默认不额外指定 MTU |

共用应用策略按用户解析 UID 后合并到 TUN 原生 UID 字段，不承诺用本机 UID 筛选热点客户端。CIDR、规则集、UID/range、MAC、NAT 等参数都使用当前核心的原生字段，不使用 `TUN_*` 别名。

`"include_interface": ["lo"]` 仅接管手机本机流量，不接管 USB 或热点转发；`lo` 不是只代理 loopback 目标。`include_interface` 未设置或为空时，接管范围也包括 USB 和热点，仍受其他原生筛选条件约束。电脑已有代理时，可改用 `"exclude_interface": ["rndis0"]` 排除 USB 共享，避免代理套娃；接口名以设备实际名称为准。电脑使用手机共享 DNS 时，手机代发的 DNS 查询仍属于本机流量。默认模板保持不限制接口。

端口绕过在主配置 `route` 中使用 `action: "bypass"` 与 pre-match，规则必须放在会终止 pre-match 的 sniff 等规则之前；它不是 TUN inbound 字段，不会自动注入隐藏规则或生成第四份运行时配置。

## 检查与排障

```sh
su -c '/data/adb/modules/netproxy/netproxyctl config read inbound/tun'
su -c '/data/adb/modules/netproxy/netproxyctl config check'
su -c '/data/adb/modules/netproxy/netproxyctl service status'
su -c '/data/adb/modules/netproxy/netproxyctl logs show core 100'
```

没有 `tun status` 命令。配置 check 通过不等于设备具备 TUN、iptables/ip6tables、NFQUEUE 或 IPv6 TPROXY 能力，实际启动与清理仍需在目标设备验证。启动失败不会自动改用 eBPF；停止后仅 PID 消失也不能证明路由、TUN 或接管规则已清理。
