# NetProxy V9 Android Prototype

这是可试用的 V9 Android 管理器原型，尚未正式发布。已接入服务控制、原生策略组选择与测速、本地 Catalog 资源/节点与 URL 订阅管理、配置和本地规则、入站、应用/Wi-Fi 策略、日志与诊断。V9 不以兼容 V8 为目标，但继续复用经过验证的 TLS、私有凭据交付和 Go 事务；不修改 V8 发布流水线，不实现桌面平台。

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

它构建 `com.fanjv.netproxy` 的 **未签名 Release APK**，版本码 1097 是 M4.2 覆盖验证基线，不是后续发布计数器。安装前必须确认高于设备现有版本，使用正式固定密钥签名，并以 `apksigner verify --print-certs` 核对模块辅助程序接受的公开指纹。不得生成替代签名后声称升级成功。密钥和密码不属于本工程，沿用已有安全签名方式，不改 V8 CI。

两种 APK 都使用同一份 React Native-mode 构建产物。`pnpm tauri android dev` 使用本机 1420 端口的显式 Native 模式开发服务，不会退回 Mock。

## CI

独立工作流 [v9-shell.yml](../../.github/workflows/v9-shell.yml) 在壳、V9 WebUI、共用 V8 `core/api` 源码与测试或自身配置变化的 push / Pull Request 上运行，也可手动执行。它不调用或修改 V8 验证、签名、模块打包与发布流水线，不读取签名 Secrets。

Ubuntu 24.04 使用 Rust 1.94.0、Node 24、pnpm 12.11.2、JDK 25、SDK / Build Tools 37 与 NDK 29.0.14206865。两个 pnpm 工程均使用 `install --frozen-lockfile`，Rust 单测、Clippy 与 APK 构建均使用 `--locked`，结束时检查三个锁文件未变。

CI 执行 `cargo fmt --all -- --check`，并通过壳的 Cargo 锁文件执行插件 `cargo test --locked -p tauri-plugin-module-api --all-targets` 和 `cargo clippy --locked -p tauri-plugin-module-api --all-targets -- -D warnings`。主壳与 Android 条件分支由下面的 APK 构建编译，不把宿主单测视作 Android 运行验收。

```sh
cd apps/shell
pnpm tauri android build --ci --debug --apk --target aarch64 -- --locked
cd src-tauri/gen/android
bash ./gradlew :tauri-plugin-module-api:testDebugUnitTest \
  :tauri-plugin-module-api:lintDebug :app:lintUniversalDebug \
  :app:assembleUniversalDebugAndroidTest \
  -PabiList=arm64-v8a -ParchList=arm64 -PtargetList=aarch64 \
  -x :app:rustBuildUniversalDebug -x :app:rustBuildArm64Debug \
  --no-daemon --console=plain
```

插件测试直接编译共享 `core/api` 测试目录中的 `LocalHttpTransportTest`、`ModuleConnectionTest`，复用 V8 的 `mockwebserver3:5.5.0`，覆盖 HTTPS、首次配对与私有连接存储。`ModuleApiClientTest`、`ModuleEventStreamTest` 依赖管理器 Repository，`ModuleApiCodecTest` 依赖壳当前 Kotlin Android 编译环境未提供的 JDK 管理模块；它们仍由 V8 验证执行，不引入壳工程。

先运行 Tauri CLI，再运行 Gradle：CLI 根据 Runner 上的 Cargo 路径刷新被忽略的 `tauri.settings.gradle` 和插件依赖，不重新执行 `android init` 或覆盖已维护的 Android 工程。Native 前端由既有 `beforeBuildCommand` 构建到 `apps/web/dist-native`，不写入 V8 模块目录。Cargo 的 `--locked` 通过 CLI 末尾的 `--` 传给构建器，不是 Tauri 自身选项。

后续 Gradle 调用固定 arm64 并显式排除两项已完成的 Rust 构建任务，复用 CLI 产出的 `libapp_lib.so`；CI 先检查该文件存在。不能在 CLI 退出后重新执行生成插件的 Rust 任务，否则其临时 WebSocket 已关闭，且默认 universal 还会尝试未启用的其他 ABI。仪器测试探针只编译为测试 APK，不运行 `connectedAndroidTest`；实际 `Activity.recreate`、SSE 与设备生命周期仍需真机验证。

成功后上传七天有效的 `v9-shell-debug-arm64` APK；单测和 Lint 报告在失败时也保留。上传前核验包名 `com.fanjv.netproxy.v9poc`、Debug 标记、仅 arm64 ABI 与 APK 签名。签名仅使用 Android 自动生成的 Debug 密钥，不使用正式密钥，不生成覆盖升级版或 Release。此产物不能读取正式包的私有凭据，也不证明首次配对、真实 HTTPS/SSE、设备生命周期或 V8 兼容性已验收。

环境与命令依据：[Tauri 前置依赖](https://v2.tauri.app/start/prerequisites/)、[Android CLI 参数](https://v2.tauri.app/reference/cli/#android-build)、[官方 Android CI](https://github.com/tauri-apps/tauri/blob/dev/.github/workflows/test-android.yml)、[Android 签名边界](https://v2.tauri.app/distribute/sign/android/)。SDK 版本已核对 [Google 官方仓库元数据](https://dl.google.com/android/repository/repository2-3.xml) 的稳定频道，Build Tools 37.0.0 提供 Linux 包。

## 安全与职责

```text
React / TanStack Query / Zod
  -> ApiClient -> NativeTransport
  -> Tauri Capability -> Rust 受限命令
  -> Kotlin ModuleApiPlugin -> ModuleApiBridge
  -> 共用 LocalHttpTransport -> Go loopback HTTPS API
```

- 只读资源枚举为 `info/service/nodes/policy_groups/mode/catalog/subscriptions`；`policy_groups` 固定读取 `GET /policy-groups`，`nodes/catalog` 读取持久资源，不承载模块级选择。`subscription_read` 只接受严格分组 ID 与 `detail/history/progress`，禁止 private 查询；`subscription_action(update/cancel)` 和 `subscription_write(add/edit/remove/update_all)` 严格验证操作、字段与 ID。方法与路径仅在原生层构造，不接收删除替代组 query。事件主题只有 `status/groups/subscriptions`，traffic 随 status 到达，groups 是原生策略快照。
- 策略操作为 `policy_select(requestId, groupTag, memberTag)` 与 `policy_delay(requestId, groupTag)`，分别映射固定 POST `/policy-groups/select`、`/policy-groups/delay`。原始标签不修剪或拆分，Rust/Kotlin 独立检查字段、控制字符、UTF-8 与 4 MiB 上限；不再开放 `select_node/test_delay` 或 selection 资源。仅已验证就绪核心可操作，可手动选择的 Selector 各自独立，选择持久化归 sing-box cache_file；测速不承诺所有成员已经完成新测试。写入不重放，未确认结果通过新快照核对。
- `management` 是固定业务枚举，不是通用 HTTP：配置目标仅限 module、inbound、sing-box 主配置/分区、本地规则和只读 runtime；另允许应用/图标、Wi-Fi、日志、eBPF、节点内容操作与固定诊断导出。`module` 只有 `auto_start/wifi`，没有 `module/selection`。本地资源新增 `catalog_create(id,name)`、`catalog_rename(id,name)`、`catalog_remove(id)`，映射 POST `/catalog`、PUT/DELETE `/catalog/{id}`；不允许删除 default，重命名/删除由 Go 限定 local。`service_action` 支持 start/stop/restart/reload/check。请求仍受双层字段、revision、类型及大小校验，不允许访问 api.json 或统计状态。
- Catalog 非空资源投影为稳定 `np/<catalog-id>` Provider，公开摘要提供 `provider_tag`；重命名不改标签，空资源不生成 Provider。资源增删不生成 Auto/Select/Proxy 图，不改主配置 outbounds、route 或 DNS；引用仍存在时拒绝破坏性删除，提交后同步失败保留 persisted。策略编辑复用 `singbox/outbounds` 的 revision 与配置事务，仅修改目标出站，保留其他对象、未知字段和未编辑的数值原文。
- 策略 GET、POST 和 groups SSE 共用 `groups/live/core_started_at_millis/default_outbound` 投影。停止时仅有配置摘要和显式出站引用，不展开 Provider、不伪造选择或测速，也不启动离线测速核心；新核心须等待新身份的完整快照，旧实例结果不得复用。
- 配置与节点原文只在用户主动打开的编辑器中保留；新输入的凭据可经过 JS/IPC，不声称永远不进入 JS。敏感内容不进入 Query/Mutation 缓存、持久存储或日志。诊断包留在原生私有缓存，系统分享仅授予选定接收方读取权限；前端只收到分享启动结果，不收到设备路径。
- 公开 Catalog 的 GET 与 SSE 共用原生字段投影，剔除私有设置、官网 URL、未知扩展与原始异常；前端仍按实际 Go 结构验证 `local/subscription`、流量、稀疏历史及当前阶段，不把进度消失当作成功。
- Kotlin 通过 Gradle sourceSet 直接编译 V8 `core/api` 源码，不复制或分叉证书、Bearer、Provider 和取消实现。首次配对同样复用该传输，安全流程见 [ADR 001](docs/adr-001-trusted-pairing.md)。
- 凭据仍由非导出、仅接受 Binder UID 0 的 `ConnectionProvider` 写入 `noBackupFilesDir/module-connection.json`。证书精确 SHA-256 和主机名验证、禁用代理/重定向、请求取消继续使用原有实现。
- 无凭据时，原生层只发送固定、无凭据的配对触发；不信其响应，必须等到新的 Root 私有交付后才进行精确钉扎业务读取。401 或身份失败先拒绝本次请求，下一次连接经同一可信交付恢复，不重放业务写入。不要求用户粘贴主 Secret。
- Kotlin 检查 schema=1、HTTP 状态、类型和 4 MiB 上限；业务错误保留状态、错误码和布尔 `persisted`，策略组/Catalog 错误另按白名单保留 `accepted/runtime_synced/core_started_at_millis`，丢弃原始异常消息与其他错误详情。`accepted` 不等于 `persisted`，缺少标记也不能证明操作未发生。JS 再经现有 ApiClient/Zod 校验。
- SSE 原生队列容量为 1，JS 同时只等待一个 next_event。没有无界事件 IPC、原生重连循环或 HTTP 轮询。慢端背压到已有有界传输；关闭和后台取消对应 Call。
- 新订阅替换遗留文档的会话，同 ID 重复建立被拒绝；旧 ID 关闭不影响新会话。读取最多 8 项并行，取消先到的 ID 有界记录，不因 IPC 重排遗漏取消。
- Kotlin Activity-aware 生命周期是原生可见性事实源；JS 将它与 document 可见性合并，继续负责有界重连、完整快照和核心身份隔离。
- Capability 仅授予本插件固定命令和官方 listener 注册/移除，实际清单见 [default.toml](plugins/module-api/permissions/default.toml)；本地资源管理复用 management，不增加通用权限。没有通用 HTTP、FS、Shell 或远程页面授权，CSP 不允许 JS 直连 Go。

## 验证与限制

[兼容性调查](docs/v8-compatibility.md)记录应用身份、Provider、私有数据、Tile 与分享迁移门槛；[本轮验收报告](docs/android-poc-report.md)区分 JVM、浏览器和真实设备证据。

第三里程碑的后续结果见 [可信配对与生命周期报告](docs/trusted-pairing-report.md)，包含首次配对 ADR、真实 401、错误签名/证书拒绝、Activity.recreate 和独立原生 CI；第二里程碑报告保留其历史时间点，不代表当前所有验收状态。

M4.1 的 start/stop 白名单、权威状态复核、真实控制与 V8 覆盖回归，以及通过的远端 CI 见 [服务控制验收报告](docs/service-control-report.md)。本阶段止于服务启停，不代表其他业务写入已经迁移。

后续 M4.2 的节点、模式和测速范围见 [核心管理验收报告](docs/core-controls-report.md)。历史报告保留原里程碑的验收时间点，不代表当前能力白名单。

M4.3.1 的资源只读矩阵、敏感字段隔离及真实设备覆盖见 [资源验收报告](docs/resources-report.md)。

M4.3.2 的只写私有输入与事务设计见 [订阅写入 ADR](docs/adr-002-subscription-writes.md)，实际验收见 [订阅管理报告](docs/subscriptions-report.md)。

上游 Tauri 2.12.4 Android 框架与官方生成模板仍使用独立 Kotlin 插件和旧 AGP DSL，本工程保留官方要求的两项兼容设置 `android.builtInKotlin=false`、`android.newDsl=false`，构建会提示未来 AGP 10 移除。没有隐藏警告或手工修改下载的 Tauri 源码。它们属于上游工程限制，不是已完成现代 Android 构建迁移的证明；升级 AGP 10 前必须先解决。已删除无必要的旧模板 ProGuard 缺失文件开关。

官方依据：[移动插件](https://v2.tauri.app/develop/plugins/develop-mobile/)、[Capability](https://v2.tauri.app/security/capabilities/)、[Android 签名](https://v2.tauri.app/distribute/sign/android/)、[Android 更新签名要求](https://developer.android.com/studio/publish/app-signing)。
