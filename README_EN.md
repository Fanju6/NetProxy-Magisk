<p align="center">
  <img src="docs/public/N.svg" alt="NetProxy Logo" width="120" />
</p>

<h1 align="center">NetProxy</h1>

<p align="center">
  <strong>System-wide sing-box transparent proxy module for Android</strong><br>
  TPROXY / REDIRECT, TCP / UDP, per-app routing, subscriptions, and Clash API
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

NetProxy is a system-wide transparent proxy module for rooted Android devices. It runs an embedded sing-box core, redirects device traffic through TPROXY or REDIRECT, and can be managed through the Android app, CLI, or zashboard.

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

- TPROXY with automatic REDIRECT fallback
- TCP, UDP, and DNS transparent proxying
- Per-app blacklist / whitelist routing
- Wi-Fi hotspot and USB tethering support
- Node links, node files, Clash YAML, and subscriptions
- Manual selector and URLTest automatic selection
- Rule, Global, and Direct modes
- Wi-Fi SSID based switching between the configured mode and Direct
- Clash API, zashboard, connection control, and delay tests
- Scheduled subscription updates, QUIC blocking, and CN IP bypass
- Integrated IPSET LKM for additional kernel compatibility

## Installation

Each release provides two packages:

| Package | Filename | Contents | Recommended for |
|---------|----------|----------|-----------------|
| **Full** | `NetProxy_<version>_<build>.zip` | sing-box, Proxylink, zashboard, Android Manager, bundled IPSET LKM drivers, and the `ipset` tool | The default choice for most devices, especially when IPSET support is uncertain |
| **Lite** | `NetProxy_<version>_<build>_lite.zip` | Everything except `bin/IPSET-LKM`; the proxy core and management features are unchanged | Devices that already provide working kernel IPSET support and an `ipset` command, or do not need IPSET-based features |

Choose the **Full** package when unsure. Lite is not a coreless package: it still includes sing-box, Proxylink, and zashboard, and removes only the bundled IPSET drivers and tool.

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
cli tproxy {status|reload|quic|cnip}
cli wifi {status|on|off|mode|add|del|list|clear|cellular}
```

```sh
su -c '/data/adb/modules/netproxy/scripts/cli help'
```

## Configuration and Logs

| Path | Purpose |
|------|---------|
| `config/module.conf` | Startup, mode, selected node, selector, and subscription scheduling |
| `config/tproxy/tproxy.conf` | Ports, transparent proxy, per-app rules, QUIC, CN bypass, and Wi-Fi switching |
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
- `PROXY_TCP_PORT=1536`
- `PROXY_UDP_PORT=1536`
- `DNS_PORT=1536`
- `PROXY_MODE=0` (TPROXY auto-detection with REDIRECT fallback)
- `BLOCK_QUIC=1`
- `BYPASS_CN_IP=0`
- `WIFI_AUTO_SWITCH=0`
- `LOG_TIMESTAMP=0`

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
| [AndroidTProxyShell](https://github.com/CHIZI-0618/AndroidTProxyShell) | Android transparent proxy reference |
| [IPSET_LKM](https://github.com/TanakaLun/IPSET_LKM) | IPSET kernel module and compatibility support |
| [zashboard](https://github.com/Zephyruso/zashboard) | Clash API dashboard |
| [v2rayNG](https://github.com/2dust/v2rayNG) | Node parsing reference |

## Community and Contributing

- [Telegram group](https://t.me/NetProxy_Magisk)
- [Issues](https://github.com/Fanju6/NetProxy-Magisk/issues)
- [Pull requests](https://github.com/Fanju6/NetProxy-Magisk/pulls)

## License

[GPL-3.0 License](LICENSE)

## Star

[![Star History Chart](https://api.star-history.com/svg?repos=Fanju6/NetProxy-Magisk&type=date&legend=top-left)](https://www.star-history.com/#Fanju6/NetProxy-Magisk&type=date&legend=top-left)
