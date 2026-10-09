---
description: NetProxy 的透明代理、节点与订阅、分应用代理、共享网络和管理入口。
---

# 项目介绍

NetProxy 是面向 Android Root 设备的 sing-box 透明代理模块。它支持 eBPF 与 TUN 入站，提供 Android 管理器、终端式模块 WebUI 和 CLI，用于管理节点、订阅、分应用代理、Wi-Fi 策略与运行状态。

## 适用环境

- `arm64-v8a` Android 设备；使用管理器需 Android 8.0（API 26）或更高版本
- Magisk、KernelSU 或 APatch
- 满足所选入站要求的内核与网络能力

默认使用 eBPF，也可在入站设置中切换为 [TUN + auto_redirect](/config/tun)。eBPF 按数据平面需要 BPF、cgroup/TC、透明 socket、veth 与策略路由等能力，可通过 [eBPF 能力探测](/config/ebpf#能力探测)检查。TUN 需要设备支持相应路由、iptables/ip6tables 与 NFQUEUE 能力。各入站的要求与切换行为见[入站配置](/config/inbound)。

## 能做什么

- 导入、编辑、导出和测速本地节点
- 添加订阅，配置筛选、请求头、下载超时和自动更新
- 使用规则、代理、直连与规则（允许广告）模式
- 按 Android 用户配置分应用代理
- 接管热点或 LAN 下游设备流量
- 按 Wi-Fi 名称与真实出口自动调整运行模式
- 编辑 sing-box 静态配置并查看生成的运行时配置
- 查看结构化模块日志与 sing-box 核心日志，导出脱敏诊断包

## 管理入口

| 入口 | 适用场景 |
|---|---|
| Android 管理器 | 日常状态、节点、订阅、入站设置、网络匹配、配置和日志 |
| 模块 WebUI | Root 管理器内的终端式命令界面 |
| `netproxyctl` | 终端操作、自动化和故障排查 |
| Service API Dashboard | sing-box 原生状态、连接和可观测性数据 |

持久节点和订阅在服务停止时仍可管理；流量、连接、实际出站模式和运行时选择等信息在核心运行后由 API 提供。

## 从哪里开始

1. 阅读[安装与升级](/guide/installation)。
2. 按[快速开始](/guide/quick-start)完成第一次导入和启动。
3. 需要切换入站、应用筛选或热点接管时阅读[透明代理与分应用策略](/guide/transparent-proxy)。
4. 遇到问题时按[常见问题与诊断](/guide/faq)收集信息。

模块设置、入站参数和配置文件布局见[配置参考](/config/module)。
