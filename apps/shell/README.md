# NetProxy Android Native PoC

这是 V9 第一阶段第二里程碑的只读原生壳，不是正式管理器替代版本。不修改 Go 业务、模块配置或 V8 发布流水线，不实现桌面平台。

## 工程

```text
apps/shell/
  package.json, pnpm-lock.yaml     # 独立 Tauri CLI
  src-tauri/                      # Rust 入口、锁文件、配置与 Capability
    gen/android/                  # 官方生成后纳入版本控制的 Android 工程
  plugins/module-api/             # 官方移动插件机制
    src/lib.rs                    # 受限 IPC 与错误边界
    android/                      # Kotlin HTTPS/SSE 与生命周期
  docs/                           # 兼容性基线与验收结果
```

使用 `shell` 而非 `desktop`，因为当前唯一目标是 Android；不为未来桌面实现创建空适配层。WebUI 源码始终在 `apps/web`，Native 构建写入独立 `dist-native`，普通浏览器产物不引入 IPC。

固定版本：Tauri Rust/API 2.12.4、CLI 2.12.3、tauri-build 2.7.3、tauri-plugin 2.7.2。Cargo 与 pnpm 锁文件纳入版本控制。Android 使用已在 V8 验证的 AGP 9.4.1、Gradle 9.8.0、Kotlin 2.4.20；JDK 25、SDK 37、NDK 29，最低 Android API 26，仅构建 arm64。

## 构建

先安装 Rust stable、Android SDK/NDK、JDK 25 和 pnpm 12.11.2，设置 `ANDROID_HOME`、`JAVA_HOME`；本轮实际 Rust 版本为 1.94.0。

```sh
cd apps/web
pnpm install --frozen-lockfile
cd ../shell
pnpm install --frozen-lockfile
rustup target add aarch64-linux-android
pnpm android:build
```

默认脚本构建隔离 Debug APK：`com.fanjv.netproxy.v9poc`。它不能读取正式包的沙箱，也不会被 Go 的固定签名交付目标识别。没有可信凭据时连接失败是预期行为，不代表真实 API 已验收。

`src-tauri/gen/android` 已提交，无需重新初始化。需要重新生成时使用官方 `pnpm tauri android init --skip-targets-install`，并审查 manifest、Gradle 和权限差异，不能覆盖已维护配置。

显式覆盖验证入口：

```sh
pnpm android:build:upgrade
```

它构建 `com.fanjv.netproxy` 的 **未签名 Release APK**，版本码 1091 是本轮测试基线，不是后续发布计数器。安装前必须确认高于设备现有版本，使用正式固定密钥签名，并以 `apksigner verify --print-certs` 核对模块辅助程序接受的公开指纹。不得生成替代签名后声称升级成功。密钥和密码不属于本工程，沿用已有安全签名方式，不改 V8 CI。

两种 APK 都使用同一份 React Native-mode 构建产物。`pnpm tauri android dev` 使用本机 1420 端口的显式 Native 模式开发服务，不会退回 Mock。

## 安全与职责

```text
React / TanStack Query / Zod
  -> ApiClient -> NativeTransport
  -> Tauri Capability -> Rust 受限命令
  -> Kotlin ModuleApiPlugin -> ModuleApiBridge
  -> 共用 LocalHttpTransport -> Go loopback HTTPS API
```

- 只读资源枚举只有 `info`、`service`，事件主题只有 `status/groups/subscriptions`，traffic 随 status 到达。没有传入方法、URL、凭据、文件或命令的 IPC。
- Kotlin 通过 Gradle sourceSet 直接编译 V8 `core/api` 源码，不复制或分叉证书、Bearer、Provider 和取消实现。唯一 V8 源码变化是 `allowProvisioning=true` 的默认构造选项；原 V8 行为不变，PoC 显式传 false。
- 凭据仍由非导出、仅接受 Binder UID 0 的 `ConnectionProvider` 写入 `noBackupFilesDir/module-connection.json`。证书精确 SHA-256 和主机名验证、禁用代理/重定向、请求取消继续使用原有实现。
- 无凭据、401 或证书身份失效时，PoC 不使用未经验证的 TLS 引导。首次安装的可信配对仍是后续安全设计门槛，不要求用户粘贴主 Secret。
- Kotlin 检查 schema=1、HTTP 状态、类型和 4 MiB 上限；业务错误保留状态和错误码，丢弃原始异常消息与错误详情。JS 再经现有 ApiClient/Zod 校验。
- SSE 原生队列容量为 1，JS 同时只等待一个 next_event。没有无界事件 IPC、原生重连循环或 HTTP 轮询。慢端背压到已有有界传输；关闭和后台取消对应 Call。
- 新订阅替换遗留文档的会话，同 ID 重复建立被拒绝；旧 ID 关闭不影响新会话。读取最多 8 项并行，取消先到的 ID 有界记录，不因 IPC 重排遗漏取消。
- Kotlin Activity-aware 生命周期是原生可见性事实源；JS 将它与 document 可见性合并，继续负责有界重连、完整快照和核心身份隔离。
- Capability 仅授予本插件六项命令和官方 listener 注册/移除。没有通用 HTTP、FS、Shell 或远程页面授权，CSP 不允许 JS 直连 Go。

## 验证与限制

[兼容性调查](docs/v8-compatibility.md)记录应用身份、Provider、私有数据、Tile 与分享迁移门槛；[本轮验收报告](docs/android-poc-report.md)区分 JVM、浏览器和真实设备证据。

上游 Tauri 2.12.4 Android 框架与官方生成模板仍使用独立 Kotlin 插件和旧 AGP DSL，本工程保留官方要求的两项兼容设置 `android.builtInKotlin=false`、`android.newDsl=false`，构建会提示未来 AGP 10 移除。没有隐藏警告或手工修改下载的 Tauri 源码。它们属于上游工程限制，不是已完成现代 Android 构建迁移的证明；升级 AGP 10 前必须先解决。已删除无必要的旧模板 ProGuard 缺失文件开关。

官方依据：[移动插件](https://v2.tauri.app/develop/plugins/develop-mobile/)、[Capability](https://v2.tauri.app/security/capabilities/)、[Android 签名](https://v2.tauri.app/distribute/sign/android/)、[Android 更新签名要求](https://developer.android.com/studio/publish/app-signing)。
