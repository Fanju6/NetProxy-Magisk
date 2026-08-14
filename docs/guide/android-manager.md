# Android 管理器

NetProxy Android 管理器是 8.0 的原生图形化入口，源码位于仓库的 [`src/android`](https://github.com/Fanju6/NetProxy-Magisk/tree/main/src/android)。普通用户推荐从 [Google Play](https://play.google.com/store/apps/details?id=com.fanjv.netproxy) 安装和更新；含管理器模块包为没有 Google Play 的设备提供备用安装方式。

## 管理器负责什么

- 仪表盘：服务状态、运行时间、流量、CPU、内存、当前分组和节点。
- 节点：本地节点与订阅节点的浏览、选择、测速、编辑、导出和删除。
- 订阅：添加、编辑、更新、流量信息、更新周期和历史记录。
- 代理设置：出站模式、DNS 劫持、IPv6、分应用代理和共享网络。
- 内核设置：编辑和校验 sing-box 静态配置，查看运行时配置。
- 日志：查看 service.log、sing-box.log，导出脱敏诊断包。
- 快捷设置磁贴：控制服务启动和停止。

管理器不会直接读取 `/data/adb`、Catalog、PID 或 Shell 文本来推断业务状态。数据流固定为：

```text
Compose -> ViewModel -> Repository -> NetProxyCtlClient -> netproxyctl
```

持久节点和订阅即使在服务停止时也能显示；流量、实际模式、运行时选中节点和连接等动态数据在服务 ready 后由 sing-box API 补充。

## 运行要求

- Android 12 或更高版本
- `arm64-v8a`
- Magisk、KernelSU 或 APatch Root 环境
- 已安装兼容的 NetProxy 8.0 模块

管理器不包含 sing-box 核心，不能脱离模块单独工作。

## 本地构建

准备 Android SDK 37 和 JDK 21：

```bash
cd src/android
./gradlew testDebugUnitTest lintDebug assembleDebug
```

Windows PowerShell：

```powershell
cd src/android
.\gradlew.bat testDebugUnitTest lintDebug assembleDebug
```

APK 位于 `app/build/outputs/apk/debug/`。本地构建不会覆盖模块内独立维护的 `src/module/NetProxy.apk`。

## 编辑器与第三方源码

内核设置使用内置 sing-box Schema、补全和校验能力；编辑器可以直接处理 reF1nd sing-box 配置字段。`third_party/scripta` 是固定源码快照，来源、许可证和 NetProxy 扩展说明见 [`third_party/scripta/NETPROXY.md`](https://github.com/Fanju6/NetProxy-Magisk/blob/main/src/android/third_party/scripta/NETPROXY.md)。
