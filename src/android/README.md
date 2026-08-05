# NetProxy Android Manager

这里是 [NetProxy](../../README.md) Android 管理器的源码目录。管理器通过 Root 权限调用模块提供的 `netproxyctl` JSON 接口，负责服务、节点、订阅、分应用代理、配置和日志等交互。

> 当前项目处于 8.0 Alpha 阶段，模块与管理器接口仍可能调整。

## 获取应用

- 推荐通过 [Google Play](https://play.google.com/store/apps/details?id=com.fanjv.netproxy) 安装和更新。
- NetProxy 完整模块包内置带广告的管理器 APK，供无法使用 Google Play 的设备安装。
- Lite 模块包不包含管理器 APK。

模块内置 APK 是独立维护的发行产物，不由仓库 CI 自动编译，也不会在本地构建后自动覆盖 `src/module/NetProxy.apk`。

## 功能

- 查看服务状态、流量、CPU、内存和当前节点
- 导入单节点、订阅与本地配置
- 管理订阅流量、更新周期和更新历史
- 切换节点、自动测速和出站模式
- 配置分应用代理与多用户应用
- 编辑并校验 sing-box 配置
- 查看、脱敏和导出诊断日志
- 通过快捷设置磁贴控制服务

## 运行要求

- Android 12 或更高版本
- `arm64-v8a` 设备
- Magisk、KernelSU 或 APatch Root 环境
- 已安装兼容版本的 NetProxy 模块

管理器不包含代理核心，不能脱离 NetProxy 模块单独工作。

## 项目结构

```text
src/android/
├── app/                   # Android 应用
├── gradle/                # 依赖版本与 Gradle Wrapper
├── third_party/scripta/   # 配置编辑器源码快照
├── ARCHITECTURE.md        # Android 分层与边界
└── settings.gradle.kts
```

应用代码按功能域组织：

```text
app/src/main/java/com/fanjv/netproxy/
├── core/                 # 命令契约、依赖容器、模块路径和共享 UI
├── feature/              # dashboard、nodes、subscriptions 等功能域
├── navigation/           # Navigation3 路由与主导航状态
├── MainActivity.kt       # Android 与 Compose 入口
└── NetProxyApplication.kt
```

主要数据流：

```text
Compose UI -> ViewModel -> Repository -> NetProxyCtlClient -> netproxyctl
```

运行状态由模块管理接口和 sing-box API 提供。Android 管理器不直接修改 Catalog 或 sing-box 运行时文件。详细边界见 [ARCHITECTURE.md](ARCHITECTURE.md)。

## 本地构建

准备 Android SDK 37 和 JDK 21，然后从仓库根目录执行：

```bash
cd src/android
./gradlew testDebugUnitTest lintDebug assembleDebug
```

Windows PowerShell：

```powershell
cd src/android
.\gradlew.bat testDebugUnitTest lintDebug assembleDebug
```

调试 APK 位于 `app/build/outputs/apk/debug/`。发布版本需要在本地提供签名材料，签名文件和密钥配置不得提交到仓库。

## 第三方源码

`third_party/scripta` 是 [Scripta](https://github.com/YuKongA/scripta) 的固定源码快照，并包含 NetProxy 配置编辑器所需的移动端工具栏与补全扩展。来源、版本和许可证见 [NETPROXY.md](third_party/scripta/NETPROXY.md)。

## 参与贡献

提交修改前请阅读仓库根目录的 [CONTRIBUTING.md](../../CONTRIBUTING.md)。安全问题请遵循 [SECURITY.md](../../SECURITY.md)，不要在公开 Issue 中提交订阅地址、节点凭据或完整日志。

## 许可证

NetProxy 使用 [GNU General Public License v3.0](../../LICENSE)。第三方组件保留各自许可证，详见 [NOTICE](../../NOTICE)。
