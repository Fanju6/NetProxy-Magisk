# 项目介绍

NetProxy 是一个面向 Android Root 环境的透明代理模块，当前版本以 **sing-box** 为核心。
它把透明代理、节点与订阅管理、分应用代理、Clash API 与面板控制整理成同一套运行体系，适用于 **Magisk / KernelSU / APatch** 环境。

## 这套文档面向什么场景

- 第一次安装 NetProxy
- 从 6.x 升级到 7.x
- 通过 Android 管理器或 CLI 日常使用
- 调整透明代理、分应用代理、路由和 DNS
- 使用 Clash API 或 zashboard 做观察与排障

## 当前架构重点

- 使用 **sing-box** 作为代理核心
- 图形化主入口为 **Android 管理器**
- 默认控制面板为 **Clash API + zashboard**
- 配置主目录位于 `/data/adb/modules/netproxy/config/singbox/`
- 节点与订阅统一转换为 sing-box 配置

## 控制入口

NetProxy 现在有三种正式入口：

1. **Android 管理器**
   适合日常用户，负责仪表盘、节点、订阅、模式切换、分应用代理、日志和常用配置编辑。
2. **CLI**
   适合脚本化、终端排障和快速批量操作。
3. **Clash API + zashboard**
   适合查看连接、代理组、延迟、实时切换和跨设备访问。

## 当前架构

### 模块侧

- `bin/sing-box`：核心进程
- `config/module.conf`：模块级默认项
- `config/ebpf/ebpf.conf`：eBPF 入站、分应用与共享网络配置
- `config/singbox/confdir/`：通用 sing-box 配置片段
- `config/singbox/outbounds/`：节点与订阅目录
- `config/singbox/runtime/`：运行时生成配置
- `logs/`：服务、核心、订阅日志

### Android 管理器侧

Android 管理器下载地址：[`NetProxy - Google Play`](https://play.google.com/store/apps/details?id=com.fanjv.netproxy)

当前不提供公开源码仓库。

它当前负责的主要能力包括：

- 服务状态与仪表盘
- 当前节点与出站模式
- 节点 / 订阅导入、切换、测速、导出
- 分应用代理开关与黑白名单
- GMS 修复、自动启动、动态测速等常用项
- sing-box / eBPF / JSON 配置编辑
- 日志查看与导出

## 推荐使用路径

- 日常使用：优先 Android 管理器
- 批量或远程操作：CLI
- 观察代理组、连接和延迟：Clash API / zashboard
- 深度排障：查看 `service.log`、`sing-box.log`、`subscription.log`
