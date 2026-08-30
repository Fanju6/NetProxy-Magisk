<p align="center">
  <img src="docs/public/N.svg" alt="NetProxy Logo" width="120" />
</p>

<h1 align="center">NetProxy</h1>

<p align="center">
  <strong>System-wide sing-box transparent proxy module for Android</strong><br>
  eBPF, TCP / UDP, per-app routing, subscriptions, and dual control APIs
</p>

<p align="center">
  <a href="https://github.com/Fanju6/NetProxy-Magisk/releases">
    <img src="https://img.shields.io/github/v/release/Fanju6/NetProxy-Magisk?style=flat-square&label=Release&color=blue" alt="Latest Release" />
  </a>
  <a href="https://github.com/Fanju6/NetProxy-Magisk/releases">
    <img src="https://img.shields.io/github/downloads/Fanju6/NetProxy-Magisk/total?style=flat-square&color=green" alt="Downloads" />
  </a>
  <img src="https://img.shields.io/badge/Core-sing--box-blueviolet?style=flat-square" alt="sing-box Core" />
</p>

<p align="center">
  <a href="https://github.com/Fanju6/NetProxy-Magisk/releases">Releases</a> ·
  <a href="https://www.netproxy.store/">Documentation</a> ·
  <a href="https://play.google.com/store/apps/details?id=com.fanjv.netproxy">Android Manager</a> ·
  <a href="src/android/">Manager Source</a> ·
  <a href="https://t.me/NetProxy_Magisk">Telegram</a>
</p>

<p align="center">
  <a href="README.md">中文</a> | English
</p>

---

## Overview

NetProxy 8.0 is a system-wide transparent proxy module for rooted Android devices. Its embedded sing-box core captures local and shared-network traffic through cgroup and TC eBPF, and can be managed through the Android app, module WebUI, CLI, Service API Dashboard, or zashboard.

Supported root environments: **Magisk, KernelSU, and APatch**.

## Source Layout

```text
src/module/          Module installation and runtime files
src/native/netproxy/ Native node, subscription, and Catalog component
src/webui/           Module WebUI
src/android/         Android Manager
```

The Android Manager and module share the `schema=1` `netproxyctl` JSON contract while keeping separate local build workflows. Repository CI does not build or publish the manager; Google Play is the recommended installation and update channel. A module package with the manager APK is also available for devices without Google Play access. See the [manager source](src/android/) for local build instructions.

## Management

| Interface | Purpose |
|-----------|---------|
| [**Android Manager**](https://play.google.com/store/apps/details?id=com.fanjv.netproxy) ([source](src/android/)) | Service, nodes, subscriptions, per-app rules, configuration, and logs |
| **Module WebUI** | Open the NetProxy portal from the KernelSU, Magisk, or APatch module page |
| **CLI** | Terminal management, automation, and diagnostics |
| **Clash API + zashboard** | Runtime groups, connections, delay tests, and mode control |

Default local endpoints:

- Clash Controller: `http://127.0.0.1:9999`
- zashboard: `http://127.0.0.1:9999/ui/`
- sing-box Service API Dashboard: `http://127.0.0.1:9090/dashboard/`
- Secret: `singbox`

Both APIs listen on loopback by default. LAN access requires an explicit listener, access-control, secret, and TLS review.

## Screenshot

<div align="center">
  <img src="docs/public/Screenshot.jpg" width="60%" alt="NetProxy Android Manager screenshot" />
</div>

## Features

- cgroup eBPF interception for local TCP, UDP, and DNS traffic
- No iptables/nftables rules or policy routing
- Per-app blacklist / whitelist routing
- Wi-Fi hotspot and USB tethering support
- Node links, node files, Clash YAML, and subscriptions
- Manual selector and URLTest automatic selection
- Rule, Global, Direct, and AllowAds modes
- Wi-Fi SSID based switching between the configured mode and Direct
- Clash API, zashboard, connection control, and delay tests
- Scheduled subscription updates and rule-set bypass
- Automatic cleanup of eBPF programs, maps, and TC attachments

## Installation

Each release provides two packages:

| Package | Filename | Contents | Recommended for |
|---------|----------|----------|-----------------|
| **Standard** | `NetProxy_<version>_<build>.zip` | sing-box, the NetProxy native component, zashboard, CLI, eBPF, and the module WebUI | The default choice when the manager is installed separately |
| **With manager** | `NetProxy_<version>_<build>_with-manager.zip` | Everything in Standard plus the optional manager APK | Devices without Google Play or users who want to install the APK during module installation |

Both packages have identical proxy capabilities. The manager APK is an independent release asset; normal Android builds do not overwrite it.

> [!IMPORTANT]
> The eBPF inbound requires kernel BPF support, cgroup v2, and cgroup socket attachment. Shared-network proxying additionally requires usable TC eBPF support. Unsupported kernels cannot start this version.

1. Download the latest ZIP from [Releases](https://github.com/Fanju6/NetProxy-Magisk/releases).
2. Flash it with Magisk, KernelSU, or APatch.
3. On an existing installation, choose **Keep existing data** or **Fresh installation**. Timeout keeps existing data.
4. If the package includes an APK, choose whether to install it; otherwise install the manager from Google Play when available.
5. A live installation is applied without a reboot. Recovery installation still requires a reboot.
6. Import and select a node before starting the service.

`AUTO_START` is disabled by default. Enable it from the manager after confirming that your node and configuration work, or set `AUTO_START=1` in `config/module.conf`.

## Quick Start

All commands require root privileges.

```sh
# Import a node link
su -c '/data/adb/modules/netproxy/netproxyctl node add "vless://..."'

# Import a node list or Clash YAML
su -c '/data/adb/modules/netproxy/netproxyctl node import /sdcard/clash.yaml'

# Select a node and start the service
su -c '/data/adb/modules/netproxy/netproxyctl node list'
su -c '/data/adb/modules/netproxy/netproxyctl node use <group-id>/<tag>'
su -c '/data/adb/modules/netproxy/netproxyctl service start'

# Inspect status and runtime mode
su -c '/data/adb/modules/netproxy/netproxyctl service status'
su -c '/data/adb/modules/netproxy/netproxyctl mode'
```

Subscriptions (the name is optional and derived automatically when omitted):

```sh
su -c '/data/adb/modules/netproxy/netproxyctl sub add https://example.com/sub'
su -c '/data/adb/modules/netproxy/netproxyctl sub list'
su -c '/data/adb/modules/netproxy/netproxyctl sub update <group-id>'
```

## Node Configuration Format

Using the Android Manager or CLI importer is recommended. NetProxy's bundled native component converts node links, text files, Clash YAML, and subscriptions into sing-box Providers.

### Manually written node files

A manual node file must be a complete sing-box configuration fragment with a top-level `outbounds` array. A raw outbound object cannot be used as the document root.

SOCKS5 example:

```json
{
  "outbounds": [
    {
      "type": "socks",
      "tag": "fr-socks",
      "server": "proxy.example.com",
      "server_port": 1080,
      "version": "5",
      "username": "user",
      "password": "password"
    }
  ]
}
```

Import the file into the `default` group:

```sh
su -c '/data/adb/modules/netproxy/netproxyctl node import /sdcard/fr-socks.json'
```

Then select it:

```sh
su -c '/data/adb/modules/netproxy/netproxyctl node list'
su -c '/data/adb/modules/netproxy/netproxyctl node use default/fr-socks'
```

Important rules:

- sing-box uses `type`; the Xray-style `protocol` field is invalid here.
- File imports are appended to the `default` local Catalog group; use a unique `tag` within that group.
- Do not use `direct`, `block`, `Proxy`, or `Auto-Fastest` as node tags.
- File imports are appended to the `default` local Catalog group. Catalog `provider.json` files are managed by NetProxy and should not be edited manually.
- Refer to the official [sing-box Outbound documentation](https://sing-box.sagernet.org/configuration/outbound/) for protocol fields.

## CLI Overview

```text
netproxyctl [--json] [--timeout <seconds|duration>] service status|start|stop|restart|reload|check|toggle
netproxyctl [--json] catalog list|show <group>
netproxyctl [--json] node list|current|show|add|import|export|edit|remove|use|delay
netproxyctl [--json] sub list|show|add|edit|update|update-all|activate|remove|history|cancel
netproxyctl [--json] mode [rule|global|direct|AllowAds]
netproxyctl [--json] network evaluate --type <wifi|not_wifi> [--ssid <name>]
netproxyctl [--json] app list|mode|add|remove|enable|disable
netproxyctl [--json] ebpf status [configured|all|local|shared] [--raw]
netproxyctl [--json] config list|read|check|validate|apply
netproxyctl [--json] logs show|clear|export
```

Node references are always `<group-id>/<tag>`. Use `node use auto [group]` for automatic mode and `node delay auto [group]` for group latency tests. The name argument of `sub add` is optional (`sub add <URL>`); it is then derived from Profile-Title, the response filename, or the URL host. Commands use a 30-second default timeout, except `service start` (120 seconds); subscription mutations use their download timeout.

```sh
su -c '/data/adb/modules/netproxy/netproxyctl help'
```

## Configuration and Logs

| Path | Purpose |
|------|---------|
| `config/module.conf` | Startup, mode, selected node, selector, and subscription scheduling |
| `config/ebpf/ebpf.conf` | eBPF inbound, per-app rules, shared networks, and map capacities |
| `config/singbox/confdir/` | Shared sing-box DNS, route, and Clash API configuration |
| `data/catalog/<group-id>/` | Node and subscription groups (`meta.json` + `provider.json`) |
| `runtime/` | Generated Provider, outbound, and eBPF files; do not edit manually |
| `config/singbox/rules/local/` | Editable local route rule sets |
| `config/singbox/rules/remote/` | Built-in SRS rule resources managed by remote providers |
| `logs/service.log` | Module service, subscription updates, and transparent proxy logs |
| `logs/sing-box.log` | sing-box core logs |

Key defaults:

- `AUTO_START=0`
- `OUTBOUND_MODE=rule`
- `SELECTOR_MODE=urltest`
- `ACTIVE_GROUP_ID=default`
- `EBPF_NETWORK=""` (TCP and UDP)
- `EBPF_LOCAL_DNS_MODE=hijack`
- `EBPF_SHARED_DNS_MODE=hijack`
- `EBPF_MODE=local`
- `EBPF_LOCAL_IPV6=1`
- `EBPF_SHARED_IPV6=1`
- `EBPF_LOCAL_BYPASS_PRIVATE_ADDRESS=1`
- `EBPF_SHARED_BYPASS_PRIVATE_ADDRESS=1`
- `EBPF_BYPASS_RULE_SET="direct,cn-ip"` (comma-separated rule-set tags)
- `WIFI_AUTO_SWITCH=0`

For startup failures, inspect the core log first:

```sh
su -c '/data/adb/modules/netproxy/netproxyctl logs show core 100'
```

See the [NetProxy documentation](https://www.netproxy.store/) for complete installation, configuration, and troubleshooting guidance.

## Acknowledgments

| Project | Role |
|---------|------|
| [reF1nd/sing-box](https://github.com/reF1nd/sing-box) | Current proxy core |
| [SagerNet/sing-box](https://github.com/SagerNet/sing-box) | Upstream sing-box project |
| [Proxylink](https://github.com/Fanju6/Proxylink) | Original project behind NetProxy's internal node conversion support |
| [AsteriskNG](https://github.com/Asterisk4Magisk/AsteriskNG) | Android eBPF implementation reference |
| [zashboard](https://github.com/Zephyruso/zashboard) | Clash API dashboard |
| [v2rayNG](https://github.com/2dust/v2rayNG) | Node parsing reference |

---

### Historical Acknowledgments

The following projects powered or inspired earlier NetProxy releases. Their contributions remain acknowledged even though the related implementations have since been replaced.

| Project | Historical role |
|---------|-----------------|
| ~~[CHIZI-0618/sing-box](https://github.com/CHIZI-0618/sing-box)~~ | Previously used sing-box branch |
| ~~[Xray-core](https://github.com/XTLS/Xray-core)~~ | Previous proxy core |
| ~~[AndroidTProxyShell](https://github.com/CHIZI-0618/AndroidTProxyShell)~~ | TPROXY / REDIRECT implementation reference |
| ~~[IPSET_LKM](https://github.com/TanakaLun/IPSET_LKM)~~ | IPSET kernel module and compatibility support |
| ~~[KsuWebUIStandalone](https://github.com/KOWX712/KsuWebUIStandalone)~~ | Standalone WebUI implementation reference |

## Community and Contributing

- [Contributing guide](CONTRIBUTING.md)
- [Architecture and coding agent guide](AGENTS.md)
- [Telegram group](https://t.me/NetProxy_Magisk)
- [Issues](https://github.com/Fanju6/NetProxy-Magisk/issues)
- [Pull requests](https://github.com/Fanju6/NetProxy-Magisk/pulls)

## License

[GPL-3.0 License](LICENSE)

## Star

[![Star History Chart](https://star-history.dera.page/svg?repos=Fanju6/NetProxy-Magisk&type=date&legend=top-left)](https://star-history.dera.page/#Fanju6/NetProxy-Magisk&type=date&legend=top-left)
