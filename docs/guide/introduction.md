# 项目介绍

NetProxy 是面向 Android Root 设备的 sing-box 透明代理模块。它通过所选的 eBPF 或 Root TUN 入站接管流量，并提供 Android 管理器、终端式模块 WebUI 和 CLI，让节点、订阅、分应用代理、Wi-Fi 策略与日志排查不必依赖手工拼接脚本。

## 适用环境

- Android 12 及以上的 `arm64-v8a` 设备
- Magisk、KernelSU 或 APatch
- 满足所选入站要求的内核与网络能力

初始入站为 eBPF，也可主动选择 [Root TUN + auto_redirect](/config/tun)，不会失败后自动换后端。eBPF 按数据平面需要 BPF、cgroup/TC、透明 socket、veth 与策略路由等能力，请使用 [eBPF 能力探测](/config/ebpf#能力探测)。TUN 需要实际设备支持相应路由、iptables/ip6tables 与 NFQUEUE 能力；配置检查不能代替启动验证，不能只凭 Android 或 Linux 版本判断兼容性。

## 能做什么

- 导入、编辑、导出和测速本地节点
- 添加订阅，配置筛选、请求头、下载超时和自动更新
- 使用规则、全局、直连与允许广告模式
- 按 Android 用户配置分应用代理
- 接管热点或 LAN 下游设备流量
- 按 Wi-Fi 名称与真实出口自动调整运行模式
- 编辑 sing-box 静态配置并查看生成的运行时配置
- 查看结构化模块日志与 sing-box 核心日志，导出脱敏诊断包

## 管理入口

| 入口 | 适用场景 |
|---|---|
| Android 管理器 | 日常状态、节点、订阅、代理设置、配置和日志 |
| 模块 WebUI | Root 管理器内的终端式命令界面 |
| `netproxyctl` | 终端操作、自动化和故障排查 |
| Service API Dashboard | sing-box 原生状态、连接和可观测性数据 |

持久节点和订阅在服务停止时仍可管理；流量、连接、实际出站模式和运行时选择等信息在核心运行后由 API 提供。

## 从哪里开始

1. 阅读[安装与升级](/guide/installation)。
2. 按[快速开始](/guide/quick-start)完成第一次导入和启动。
3. 需要切换入站、应用筛选或热点接管时阅读[透明代理与分应用策略](/guide/transparent-proxy)。
4. 遇到问题时按[常见问题与诊断](/guide/faq)收集信息。

底层文件布局和生成关系集中在[配置参考](/config/module)，不需要在第一次使用前理解。
