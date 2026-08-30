<p align="center">
  <img src="docs/public/N.svg" alt="NetProxy Logo" width="120" />
</p>

<h1 align="center">NetProxy</h1>

<p align="center">
  <strong>Android 系统级 sing-box 透明代理模块</strong><br>
  支持 eBPF、TCP / UDP、分应用代理、节点订阅与双控制 API
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
  <a href="https://github.com/Fanju6/NetProxy-Magisk/releases">下载模块</a> ·
  <a href="https://www.netproxy.store/">使用文档</a> ·
  <a href="https://play.google.com/store/apps/details?id=com.fanjv.netproxy">Android 管理器</a> ·
  <a href="src/android/">管理器源码</a> ·
  <a href="https://t.me/NetProxy_Magisk">Telegram</a>
</p>

<p align="center">
  中文 | <a href="README_EN.md">English</a>
</p>

---

## 项目简介

NetProxy 8.0 是面向已 Root Android 设备的系统级透明代理模块。模块以内置 sing-box 为代理核心，通过 cgroup 与 TC eBPF 接管本机及共享网络流量，并提供 Android 管理器、模块 WebUI、CLI、Service API Dashboard 与 zashboard 等入口。

支持 **Magisk、KernelSU 与 APatch**。节点、订阅、路由、DNS 和透明代理配置均保存在模块目录中，不依赖 VPN 模式运行。

## 源码结构

```text
src/module/          模块安装与运行文件
src/native/netproxy/ 节点、订阅与 Catalog 原生组件
src/webui/           模块 WebUI
src/android/         Android 管理器
```

Android 管理器与模块共用 `netproxyctl` 的 `schema=1` JSON 契约，但保持独立的本地构建流程。仓库 CI 不编译或发布管理器，推荐通过 Google Play 安装更新；另提供含管理器 APK 的模块包，供无法使用 Google Play 的设备安装。Android 构建说明见 [管理器源码](src/android/)。

## 管理入口

| 入口 | 适合场景 |
|------|----------|
| [**Android 管理器**](https://play.google.com/store/apps/details?id=com.fanjv.netproxy)（[源码](src/android/)） | 日常使用，管理服务、节点、订阅、分应用代理、配置与日志 |
| **模块 WebUI** | 从 KernelSU、Magisk 或 APatch 的模块页面进入 NetProxy、zashboard 与 sing-box Dashboard |
| **CLI** | 终端操作、自动化和故障排查 |
| **Clash API + zashboard** | 查看代理组、连接与延迟，进行运行时控制 |

Clash API 默认配置：

- Controller：`http://127.0.0.1:9999`
- zashboard：`http://127.0.0.1:9999/ui/`
- sing-box Service API Dashboard：`http://127.0.0.1:9090/dashboard/`
- Secret：`singbox`

Clash API 与 Service API 默认只监听本机。需要从其他设备访问时，请显式配置监听地址，并同时调整访问控制、密钥和 TLS；不要直接暴露默认端口。

## 界面预览

<div align="center">
  <img src="docs/public/Screenshot.jpg" width="60%" alt="NetProxy Android 管理器界面预览" />
</div>

## 核心能力

- 使用 cgroup eBPF 接管本机 TCP、UDP 与 DNS 流量
- 不修改 iptables、nftables 或策略路由
- 分应用黑名单 / 白名单、热点和 USB 共享代理
- 单节点链接、节点文件、Clash YAML 与订阅导入
- 手动节点选择与 URLTest 自动测速
- Rule、Global、Direct、AllowAds 出站模式
- 按 WiFi SSID 在基础模式与 Direct 之间自动切换
- Clash API、zashboard、连接管理与节点测速
- 订阅定时更新和规则集提前绕过
- 自动清理 eBPF 程序、Map 与 TC 挂载

## 安装

Release 页面提供以下两个版本：

| 版本 | 文件名 | 包含内容 | 适用设备 |
|------|--------|----------|----------|
| **标准包** | `NetProxy_<版本>_<构建号>.zip` | sing-box、NetProxy 原生组件、模块 WebUI、zashboard、CLI 与 eBPF | 默认推荐；通过 Google Play 安装管理器，或使用 CLI / WebUI |
| **含管理器包** | `NetProxy_<版本>_<构建号>_with-manager.zip` | 标准包全部内容，以及刷入时可选安装的 Android 管理器 APK | 无法使用 Google Play、需要随模块安装管理器的设备 |

两个包的代理能力完全一致。标准包也是模块自更新的默认下载目标；只有需要随模块刷入管理器时才选择**含管理器包**。

> [!IMPORTANT]
> eBPF 入站需要内核启用 BPF、cgroup v2 与 cgroup socket attach 能力。热点共享还需要可用的 TC eBPF 支持；不满足要求的内核无法启动本版本。

1. 从 [Releases](https://github.com/Fanju6/NetProxy-Magisk/releases) 下载最新模块 ZIP。
2. 在 Magisk、KernelSU 或 APatch 中刷入模块。
3. 更新已有模块时，按安装提示选择“保留现有数据”或“全新安装”；超时默认保留现有数据。
4. 含管理器包会提供随附 APK 的安装选项；标准包不显示该步骤。
5. 已开机刷入会在后台应用新版本，无需重启；Recovery 刷入完成后需要重启设备。
6. 导入并选择节点，再通过管理器、模块 WebUI 或 CLI 启动服务。

模块默认 `AUTO_START=0`。确认节点与配置可用后，可在管理器中启用开机启动，或将 `config/module.conf` 中的 `AUTO_START` 改为 `1`。

## 快速开始

以下命令均需要 Root 权限。

```sh
# 查看服务状态
su -c '/data/adb/modules/netproxy/netproxyctl service status'

# 导入单个节点链接
su -c '/data/adb/modules/netproxy/netproxyctl node add "vless://..."'

# 导入节点文本或 Clash YAML
su -c '/data/adb/modules/netproxy/netproxyctl node import /sdcard/clash.yaml'

# 查看并选择节点
su -c '/data/adb/modules/netproxy/netproxyctl node list'
su -c '/data/adb/modules/netproxy/netproxyctl node use <分组 ID>/<节点标签>'

# 启动服务
su -c '/data/adb/modules/netproxy/netproxyctl service start'

# 查看或切换出站模式
su -c '/data/adb/modules/netproxy/netproxyctl mode'
su -c '/data/adb/modules/netproxy/netproxyctl mode rule'
```

添加订阅（名称可省略，留空时自动获取）：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl sub add https://example.com/sub'
su -c '/data/adb/modules/netproxy/netproxyctl sub list'
su -c '/data/adb/modules/netproxy/netproxyctl sub update <分组 ID>'
```

## 节点配置格式

推荐使用 Android 管理器或 CLI 导入节点。模块内置的原生组件会把单链接、节点文件、Clash YAML 和订阅转换为模块需要的 sing-box Provider。

### 手写节点文件

手写节点文件必须是一个完整的 sing-box 配置片段，顶层使用 `outbounds` 数组。不能直接把单个 outbound 对象作为文件根节点。

下面是 SOCKS5 节点示例：

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

将节点文件导入 `default` 分组：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl node import /sdcard/fr-socks.json'
```

然后执行：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl node list'
su -c '/data/adb/modules/netproxy/netproxyctl node use default/fr-socks'
```

注意事项：

- sing-box 出站协议字段是 `type`，不是 Xray 配置中的 `protocol`。
- 本地文件导入后会将节点追加到 `default` 本地配置组；建议保证 `tag` 在该组中唯一。
- 不要使用 `direct`、`block`、`Proxy` 或 `Auto-Fastest` 作为节点标签。
- `data/catalog/` 中的 `provider.json` 由 Catalog 管理，不建议手动编辑；格式错误会导致对应分组无法加载。协议字段请以 [sing-box Outbound 文档](https://sing-box.sagernet.org/configuration/outbound/) 为准。

## CLI 命令

```text
netproxyctl [--json] [--timeout <秒|时长>] service status|start|stop|restart|reload|check|toggle
netproxyctl [--json] catalog list|show <分组>
netproxyctl [--json] node list|current|show|add|import|export|edit|remove|use|delay
netproxyctl [--json] sub list|show|add|edit|update|update-all|activate|remove|history|cancel
netproxyctl [--json] mode [rule|global|direct|AllowAds]
netproxyctl [--json] network evaluate --type <wifi|not_wifi> [--ssid <名称>]
netproxyctl [--json] app list|mode|add|remove|enable|disable
netproxyctl [--json] ebpf status [configured|all|local|shared] [--raw]
netproxyctl [--json] config list|read|check|validate|apply
netproxyctl [--json] logs show|clear|export
```

节点引用固定为 `<分组 ID>/<节点标签>`。自动模式用 `node use auto [分组]`，分组测速用 `node delay auto [分组]`。`sub add` 可省略名称（`sub add <URL>`），此时按 Profile-Title、文件名、URL 主机名的顺序自动取名。所有命令默认有超时，订阅变更由下载超时控制，也可使用 `--timeout` 覆盖。

查看完整中文帮助：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl help'
```

## 配置与日志

| 路径 | 用途 |
|------|------|
| `config/module.conf` | 开机启动、出站模式、当前节点、选择模式和订阅调度 |
| `config/ebpf/ebpf.conf` | eBPF 入站、分应用、共享网络与 Map 容量 |
| `config/singbox/confdir/` | 通用 sing-box 配置，包括 DNS、路由和 Clash API |
| `data/catalog/<分组 ID>/` | 节点与订阅分组，含 `meta.json` 与 `provider.json` |
| `runtime/` | 启动时生成的 Provider、出站和 eBPF 配置，不应手动编辑 |
| `config/singbox/rules/local/` | 可编辑的本地路由规则集 |
| `config/singbox/rules/remote/` | 由远程 Provider 管理的内置 SRS 规则资源 |
| `logs/service.log` | 模块服务、订阅更新与透明代理日志 |
| `logs/sing-box.log` | sing-box 核心日志 |

常用默认值：

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `AUTO_START` | `0` | 默认不随开机启动 |
| `OUTBOUND_MODE` | `rule` | 规则分流 |
| `SELECTOR_MODE` | `urltest` | 自动测速选择 |
| `ACTIVE_GROUP_ID` | `default` | 当前生效的节点分组 |
| `EBPF_NETWORK` | 空 | 同时接管 TCP 与 UDP |
| `EBPF_LOCAL_DNS_MODE` | `hijack` | 本机数据路径的 DNS 处理模式 |
| `EBPF_SHARED_DNS_MODE` | `hijack` | 共享网络数据路径的 DNS 处理模式 |
| `EBPF_MODE` | `local` | eBPF 数据路径：local、shared 或 hybrid |
| `EBPF_LOCAL_IPV6` | `1` | 接管本机 IPv6 流量 |
| `EBPF_SHARED_IPV6` | `1` | 接管共享网络 IPv6 流量 |
| `EBPF_LOCAL_BYPASS_PRIVATE_ADDRESS` | `1` | 本机流量默认绕过私网与特殊用途地址 |
| `EBPF_SHARED_BYPASS_PRIVATE_ADDRESS` | `1` | 共享网络流量默认绕过私网与特殊用途地址 |
| `EBPF_BYPASS_RULE_SET` | `direct,cn-ip` | 在内核侧提前绕过可提取 CIDR 的规则集，多个规则集使用英文逗号分隔 |
| `WIFI_AUTO_SWITCH` | `0` | 默认关闭 WiFi SSID 自动切换 |

## 排障

```sh
# 服务与核心日志
su -c '/data/adb/modules/netproxy/netproxyctl logs show service 100'
su -c '/data/adb/modules/netproxy/netproxyctl logs show core 100'

# 诊断包（包含运行时配置摘要，敏感信息会脱敏）
su -c '/data/adb/modules/netproxy/netproxyctl logs export /sdcard/Download/netproxy-diagnostics.tar.gz'
```

启动失败时优先检查 `sing-box.log`。出现 eBPF 加载错误时，请检查内核 BPF / cgroup 能力、Root 授权与 `ebpf.conf`；手写节点无法加载时，重点检查顶层是否为 `outbounds`、协议字段是否为 `type`、JSON 语法是否正确，以及节点标签是否冲突。

更完整的安装、配置和排障说明请访问 [NetProxy 文档](https://www.netproxy.store/)。

## 鸣谢

| 项目 | 用途 |
|------|------|
| [reF1nd/sing-box](https://github.com/reF1nd/sing-box) | 当前代理核心 |
| [SagerNet/sing-box](https://github.com/SagerNet/sing-box) | 上游 sing-box 项目 |
| [Proxylink](https://github.com/Fanju6/Proxylink) | NetProxy 内部节点转换能力的原始项目 |
| [AsteriskNG](https://github.com/Asterisk4Magisk/AsteriskNG) | Android eBPF 实现参考 |
| [zashboard](https://github.com/Zephyruso/zashboard) | Clash API 控制面板 |
| [v2rayNG](https://github.com/2dust/v2rayNG) | 节点解析实现参考 |

---

### 历史鸣谢

以下项目曾为 NetProxy 的早期版本提供核心能力或实现参考。

| 项目 | 历史用途 |
|------|----------|
| ~~[CHIZI-0618/sing-box](https://github.com/CHIZI-0618/sing-box)~~ | 曾使用的 sing-box 分支 |
| ~~[Xray-core](https://github.com/XTLS/Xray-core)~~ | 曾使用的代理核心 |
| ~~[AndroidTProxyShell](https://github.com/CHIZI-0618/AndroidTProxyShell)~~ | TPROXY / REDIRECT 透明代理实现参考 |
| ~~[IPSET_LKM](https://github.com/TanakaLun/IPSET_LKM)~~ | IPSET 内核模块与兼容性支持 |
| ~~[KsuWebUIStandalone](https://github.com/KOWX712/KsuWebUIStandalone)~~ | WebUI 独立运行方案参考 |

## 交流与贡献

- [贡献指南](CONTRIBUTING.md)
- [架构说明与编码代理约束](AGENTS.md)
- [Telegram 群组](https://t.me/NetProxy_Magisk)
- [提交 Issue](https://github.com/Fanju6/NetProxy-Magisk/issues)
- [提交 Pull Request](https://github.com/Fanju6/NetProxy-Magisk/pulls)

## 许可证

[GPL-3.0 License](LICENSE)

## Star

[![Star History Chart](https://star-history.dera.page/svg?repos=Fanju6/NetProxy-Magisk&type=date&legend=top-left)](https://star-history.dera.page/#Fanju6/NetProxy-Magisk&type=date&legend=top-left)
