<p align="center">
  <img src="docs/public/N.svg" alt="NetProxy Logo" width="120" />
</p>

<h1 align="center">NetProxy</h1>

<p align="center">
  <strong>System-wide sing-box transparent proxy module for Android</strong><br>
  eBPF, TCP / UDP, per-app routing, subscriptions, and Clash API
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
  <a href="https://t.me/NetProxy_Magisk">Telegram</a>
</p>

<p align="center">
  <a href="README.md">中文</a> | English
</p>

---

## Overview

NetProxy is a system-wide transparent proxy module for rooted Android devices. Its embedded sing-box core captures local and shared-network traffic through cgroup and TC eBPF, and can be managed through the Android app, CLI, or zashboard.

Supported root environments: **Magisk, KernelSU, and APatch**.

## Management

| Interface | Purpose |
|-----------|---------|
| [**Android Manager**](https://play.google.com/store/apps/details?id=com.fanjv.netproxy) | Service, nodes, subscriptions, per-app rules, configuration, and logs |
| **CLI** | Terminal management, automation, and diagnostics |
| **Clash API + zashboard** | Runtime groups, connections, delay tests, and mode control |

Default Clash API endpoints:

- Controller: `http://<device-ip>:9999`
- zashboard: `http://<device-ip>:9999/ui/`
- Secret: `singbox`

The controller listens on all interfaces by default. Use it only on trusted networks and change the secret when necessary.

## Screenshot

<div align="center">
  <img src="docs/public/Screenshot.jpg" width="60%" alt="NetProxy Android Manager screenshot" />
</div>

## Features

- cgroup eBPF interception for local TCP, UDP, and DNS traffic
- No TUN device, iptables/nftables rules, or policy routing
- Per-app blacklist / whitelist routing
- Wi-Fi hotspot and USB tethering support
- Node links, node files, Clash YAML, and subscriptions
- Manual selector and URLTest automatic selection
- Rule, Global, and Direct modes
- Wi-Fi SSID based switching between the configured mode and Direct
- Clash API, zashboard, connection control, and delay tests
- Scheduled subscription updates and rule-set bypass
- Automatic cleanup of eBPF programs, maps, and TC attachments

## Installation

Each release provides two packages:

| Package | Filename | Contents | Recommended for |
|---------|----------|----------|-----------------|
| **Full** | `NetProxy_<version>_<build>.zip` | sing-box, Proxylink, zashboard, and the Android Manager APK | The default choice when the manager should be installed with the module |
| **Lite** | `NetProxy_<version>_<build>_lite.zip` | The same core, CLI, eBPF, and zashboard features, without the Android Manager APK | Users who install the manager from Google Play or only use CLI / zashboard |

Both packages have identical proxy capabilities. Choose **Full** to bundle the manager installer; Lite users can still install the manager from Google Play.

> [!IMPORTANT]
> The eBPF inbound requires kernel BPF support, cgroup v2, and cgroup socket attachment. Shared-network proxying additionally requires usable TC eBPF support. Unsupported kernels cannot start this version.

1. Download the latest ZIP from [Releases](https://github.com/Fanju6/NetProxy-Magisk/releases).
2. Flash it with Magisk, KernelSU, or APatch.
3. Follow the installer prompt for the bundled manager or Google Play version, then reboot.
4. Import and select a node before starting the service.

`AUTO_START` is disabled by default. Enable it from the manager after confirming that your node and configuration work, or set `AUTO_START=1` in `config/module.conf`.

## Quick Start

All commands require root privileges.

```sh
# Import a node link
su -c '/data/adb/modules/netproxy/scripts/cli node add "vless://..."'

# Import a node list or Clash YAML
su -c '/data/adb/modules/netproxy/scripts/cli node import /sdcard/clash.yaml'

# Select a node and start the service
su -c '/data/adb/modules/netproxy/scripts/cli node list'
su -c '/data/adb/modules/netproxy/scripts/cli node use NodeName'
su -c '/data/adb/modules/netproxy/scripts/cli service start'

# Inspect status and runtime mode
su -c '/data/adb/modules/netproxy/scripts/cli service status'
su -c '/data/adb/modules/netproxy/scripts/cli mode'

# Show the zashboard endpoint
su -c '/data/adb/modules/netproxy/scripts/cli api ui'
```

Subscriptions:

```sh
su -c '/data/adb/modules/netproxy/scripts/cli sub add MySub https://example.com/sub'
su -c '/data/adb/modules/netproxy/scripts/cli sub update MySub'
su -c '/data/adb/modules/netproxy/scripts/cli sub auto on'
```

## Node Configuration Format

Using the Android Manager or CLI importer is recommended. Proxylink converts node links, text files, Clash YAML, and subscriptions into the sing-box fragments expected by NetProxy.

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

Place the file at:

```text
/data/adb/modules/netproxy/config/singbox/outbounds/default/fr-socks.json
```

Then select it:

```sh
su -c '/data/adb/modules/netproxy/scripts/cli node list'
su -c '/data/adb/modules/netproxy/scripts/cli node use fr-socks'
```

Important rules:

- sing-box uses `type`; the Xray-style `protocol` field is invalid here.
- Keep one regular node per file and use a unique `tag` within the directory.
- Do not use `direct`, `block`, `Proxy`, or `Auto-Fastest` as node tags.
- All JSON files in the active node directory are loaded together. A malformed file can prevent the core from starting.
- Refer to the official [sing-box Outbound documentation](https://sing-box.sagernet.org/configuration/outbound/) for protocol fields.

## CLI Overview

```text
cli service {status|start|stop|restart|logs|logs-clear}
cli node {list|current|use|add|import|export|show|remove|delay}
cli mode [rule|global|direct]
cli sub {list|add|update|update-all|remove|auto}
cli api {groups|conns|close|close-all|ui}
cli app {list|mode|add|remove|enable|disable}
cli ebpf {status|reload|dns|ipv6|shared|interface}
cli wifi {status|on|off|mode|add|del|list|clear|cellular}
```

```sh
su -c '/data/adb/modules/netproxy/scripts/cli help'
```

## Configuration and Logs

| Path | Purpose |
|------|---------|
| `config/module.conf` | Startup, mode, selected node, selector, and subscription scheduling |
| `config/ebpf/ebpf.conf` | eBPF inbound, per-app rules, shared networks, and map capacities |
| `config/singbox/confdir/` | Shared sing-box DNS, route, and Clash API configuration |
| `config/singbox/outbounds/` | Local and subscription node directories |
| `config/singbox/source/` | Local route rules and rule sets |
| `logs/service.log` | Module service and transparent proxy logs |
| `logs/sing-box.log` | sing-box core logs |
| `logs/subscription.log` | Node and subscription conversion logs |

Key defaults:

- `AUTO_START=0`
- `OUTBOUND_MODE=rule`
- `SELECTOR_MODE=urltest`
- `CURRENT_CONFIG=""`
- `EBPF_NETWORK=""` (TCP and UDP)
- `EBPF_DNS_MODE=hijack`
- `EBPF_IPV6=1`
- `EBPF_BYPASS_RULE_SETS="direct ChinaIP"`
- `EBPF_SHARED_NETWORK=0`
- `WIFI_AUTO_SWITCH=0`

For startup failures, inspect the core log first:

```sh
su -c '/data/adb/modules/netproxy/scripts/cli service logs core 100'
```

See the [NetProxy documentation](https://www.netproxy.store/) for complete installation, configuration, and troubleshooting guidance.

## Acknowledgments

| Project | Role |
|---------|------|
| [reF1nd/sing-box](https://github.com/reF1nd/sing-box) | Current proxy core |
| [SagerNet/sing-box](https://github.com/SagerNet/sing-box) | Upstream sing-box project |
| [Proxylink](https://github.com/Fanju6/Proxylink) | Node, subscription, and configuration conversion |
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

- [Telegram group](https://t.me/NetProxy_Magisk)
- [Issues](https://github.com/Fanju6/NetProxy-Magisk/issues)
- [Pull requests](https://github.com/Fanju6/NetProxy-Magisk/pulls)

## License

[GPL-3.0 License](LICENSE)

## Star

[![Star History Chart](https://api.star-history.com/svg?repos=Fanju6/NetProxy-Magisk&type=date&legend=top-left)](https://www.star-history.com/#Fanju6/NetProxy-Magisk&type=date&legend=top-left)
