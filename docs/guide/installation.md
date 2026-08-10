# 安装与升级

## 安装前准备

- 设备已具备 Root 环境
- 已安装 **Magisk / KernelSU / APatch** 之一
- 内核支持 BPF、cgroup v2 与 cgroup socket attach；共享网络还需要 TC eBPF
- 已准备可用节点或订阅

如果你准备使用 Android 管理器，可通过 Google Play 安装：[`NetProxy`](https://play.google.com/store/apps/details?id=com.fanjv.netproxy)。当前应用包名为 `com.fanjv.netproxy`，`minSdk` 为 **31**。

## 安装模块

Release 提供两个代理能力相同的包：完整版内置 Android 管理器 APK，Lite 不内置 APK。已从 Google Play 安装管理器或只使用 CLI / zashboard 时可选择 Lite。

1. 从 [NetProxy Releases](https://github.com/Fanju6/NetProxy-Magisk/releases) 下载最新模块 ZIP
2. 在 **Magisk / KernelSU / APatch** 中刷入模块
3. 重启设备
4. 通过 Android 管理器、CLI 或 zashboard 继续配置

## 安装后建议先做的事

1. 导入一个可用节点或订阅
2. 启动服务
3. 确认当前模式为 `rule`
4. 在设备本机访问 `http://127.0.0.1:9999/ui` 检查控制面板是否可用

## Android 管理器

如果你需要图形化管理：

- 下载地址：[`NetProxy - Google Play`](https://play.google.com/store/apps/details?id=com.fanjv.netproxy)
- 应用包名：`com.fanjv.netproxy`
- 当前不提供公开源码仓库
- 推荐将模块与管理器配合使用

## 从 6.x 升级到 7.x

7.x 是大版本迁移，请重点注意：

- 核心从 Xray 切换为 sing-box
- 内置 WebUI 已移除
- 配置目录由 `config/xray` 迁移到 `config/singbox`
- 新的图形化入口是 Android 管理器
- 新的默认控制入口是 Clash API + zashboard

升级细节请继续阅读：[6.x 到 7.x 升级](/guide/upgrade-v7)
