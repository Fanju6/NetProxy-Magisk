# 安装与升级

## 安装前准备

- 设备已安装 Magisk、KernelSU 或 APatch，并拥有 Root 权限。
- 设备为 Android 12 或更高版本，架构为 `arm64-v8a`。
- 内核支持 BPF、cgroup v2 和 cgroup socket attach；共享网络还需要 TC eBPF。
- 已准备可用节点或订阅。

NetProxy 8.0 使用 sing-box eBPF 入站，不提供 TPROXY / REDIRECT 回退。内核能力不满足要求时，服务无法启动透明代理。

Android 管理器可从 [Google Play](https://play.google.com/store/apps/details?id=com.fanjv.netproxy) 安装。模块源码仓库也包含管理器源码；标准模块包不包含 APK，`_with-manager` 包额外携带可选安装的 APK。

## 选择模块包

Release 中的两个包代理能力完全相同：

| 包 | 文件名 | 内容 |
|----|--------|------|
| 标准包 | `NetProxy_<版本>_<构建号>.zip` | 模块、sing-box、Native、CLI、WebUI、zashboard 和规则资源，不含 APK |
| 含管理器包 | `NetProxy_<版本>_<构建号>_with-manager.zip` | 标准包全部内容，另含安装阶段可选的管理器 APK |

已经通过 Google Play 安装管理器，或只使用 CLI / WebUI 的用户，选择标准包即可。没有 Google Play 的用户可以选择含管理器包，并在安装阶段决定是否安装 APK。

## 刷入模块

1. 从 [Releases](https://github.com/Fanju6/NetProxy-Magisk/releases) 下载对应模块包。
2. 在 Magisk、KernelSU 或 APatch 中刷入。
3. 更新已有模块时选择：
   - **保留现有数据**：保留节点、订阅、规则和模块设置。
   - **全新安装**：使用模块包默认配置，清除现有用户数据。
4. 安装模式等待超时默认选择“保留现有数据”。
5. 含管理器包会显示 APK 安装选项；标准包会跳过该步骤。
6. 已开机环境下，模块更新会在后台应用，不需要重启；Recovery 环境刷入后仍需按管理器要求重启。

安装完成后，先导入节点或订阅，再启动服务。模块默认 `AUTO_START=0`，确认配置可用后再在管理器中开启开机启动。

## 安装管理器

- 推荐渠道：[Google Play](https://play.google.com/store/apps/details?id=com.fanjv.netproxy)
- 应用包名：`com.fanjv.netproxy`
- 模块内置 APK：仅在含管理器包中提供，适用于无法使用 Google Play 的设备
- 管理器不能脱离 NetProxy 模块单独提供代理核心

模块安装结束不会自动打开 Google Play，也不会强制用户安装管理器。可以使用 Android 管理器、模块 WebUI 或 CLI 完成全部基础配置。

## 8.0 与旧版本

8.0 是一次新的模块架构。它以 Catalog 保存节点和订阅，以 Go Native 组件负责配置、Provider、订阅事务、服务生命周期和唯一后台 Worker；Shell 只保留模块开机桥接。

不要把旧版本的命令入口、配置目录或防火墙规则复制到 8.0。旧版本背景与历史迁移说明见 [V7 历史升级指南](/guide/upgrade-v7)；该页面不是 8.0 的当前操作手册。
