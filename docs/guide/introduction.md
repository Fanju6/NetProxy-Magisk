# NetProxy 8.0

NetProxy 8.0 是面向 Android Root 设备的 sing-box 透明代理模块。它以 reF1nd sing-box 为核心，使用 eBPF 入站接管本机与共享网络流量，并把节点、订阅、路由、DNS、分应用代理和运行状态统一到一套模块目录与命令契约中。

当前 8.0 仍处于测试阶段。稳定性、内核兼容性和 Android 管理器功能会继续迭代；遇到问题时请同时提供模块版本、设备内核信息和脱敏诊断包。

## 支持环境

- Magisk、KernelSU 或 APatch
- Android 12 及以上
- `arm64-v8a` 设备
- 支持 BPF、cgroup v2 和 cgroup socket attach 的 Root 内核
- 共享网络代理还需要 TC eBPF 能力

本版本使用 sing-box eBPF 入站，不再提供 TPROXY / REDIRECT 回退。内核不具备 eBPF 能力时，sing-box 无法启动透明代理入站。

## 管理入口

1. **Android 管理器**：日常使用的原生界面，管理服务、节点、订阅、分应用代理、配置和日志。
2. **模块 WebUI**：从 KernelSU、Magisk 或 APatch 的模块页面进入统一入口，可打开 NetProxy WebUI、zashboard 和 sing-box Service API Dashboard。
3. **CLI**：通过 `netproxyctl` 进行终端操作、脚本自动化和故障排查。
4. **Clash API / zashboard**：查看代理组、连接和延迟，并执行运行时切换。
5. **Service API Dashboard**：查看 sing-box 原生服务状态和运行数据。

三类控制接口使用固定的本机监听：

| 接口 | 地址 | 用途 |
|------|------|------|
| NetProxy CLI | `/data/adb/modules/netproxy/netproxyctl` | 模块持久配置与生命周期 |
| Service API | `127.0.0.1:9090` | sing-box 原生状态、连接、节点组和测速 |
| Clash API | `127.0.0.1:9999` | zashboard 和第三方 Clash 客户端 |

默认密钥为 `singbox`，两个 API 默认只监听 loopback。除非你已经单独配置鉴权、CORS、TLS 和访问范围，否则不要把端口暴露到局域网。

## 目录与事实源

```text
/data/adb/modules/netproxy/
├── bin/                         # sing-box、netproxyctl、netproxy-native
├── config/
│   ├── module.conf              # 模块启动、选择和出站模式
│   ├── ebpf/ebpf.conf           # eBPF 入站与分应用设置
│   └── singbox/
│       ├── confdir/             # 静态 sing-box 配置片段
│       └── rules/               # 本地规则与内置远程 SRS
├── data/catalog/                # 节点与订阅的持久事实源
│   ├── default/                 # 单链接和文件导入的本地组
│   ├── <group-id>/              # 订阅分组
│   └── staging/                 # 临时事务目录
├── runtime/                     # 启动时生成的配置
├── logs/                        # service.log 与 sing-box.log
└── webroot/                     # 模块 WebUI、zashboard、Service Dashboard
```

Catalog 中每个分组使用 `meta.json` 和 `provider.json`。`provider.json` 是节点内容事实源，客户端通过 `netproxyctl` 访问，不直接依赖文件扫描。`runtime/` 只保存生成的 `providers.json`、`outbounds.json` 和 `ebpf.json`，不应手动编辑。

## 选择与模式

- `ACTIVE_GROUP_ID` 保存当前活动分组。
- `SELECTOR_MODE=urltest` 使用 `Auto/<group>` 自动测速。
- `SELECTOR_MODE=manual` 使用 `<group-id>/<tag>` 手动选择。
- `OUTBOUND_MODE` 支持 `rule`、`global`、`direct` 和 `AllowAds`。

自动模式不会保存某个测速结果作为手动节点，也不会在失败时静默切换到 `direct`。eBPF 提前绕过规则集可能在内核侧直接放行 CIDR；需要严格 Global 测试时，应清空 `EBPF_BYPASS_RULE_SET` 后重启服务。

## 推荐阅读顺序

1. [安装与升级](/guide/installation)
2. [快速开始](/guide/quick-start)
3. [节点与订阅](/guide/nodes-subscriptions)
4. [eBPF 透明代理](/guide/transparent-proxy)
5. [CLI 使用](/guide/cli)
6. [配置参考](/config/module)

旧版本迁移说明保留在 [V7 历史升级指南](/guide/upgrade-v7)，不适合作为 8.0 的安装说明。
