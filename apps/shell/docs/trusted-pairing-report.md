# 第三里程碑：可信配对与原生工程验证

日期：2026-10-16。开发分支：`feat/v9-webui-foundation`。
基线：`2ef8f11d5d5873dc154f8dde48bd33fc37c9bdc3`。

## 范围与调用链

[ADR 001](adr-001-trusted-pairing.md) 定义威胁模型、双向信任根、初始触发与恢复。
本轮没有新增业务 API、Go 配置字段、安装规则或真实写入 IPC。

```text
React -> ApiClient/Zod -> NativeTransport -> 现有 Tauri Capability/Rust 白名单
  -> Kotlin 复用 V8 LocalHttpTransport
  -> 匿名固定 provision 触发（不采信响应）
  -> Go Worker -> Root DEX verifyManager -> 固定签名私有 Provider
  -> 原子写入 ConnectionStore -> 精确证书/主机名验证 + Bearer GET/SSE
```

JS 不能取得 Secret、指纹、Root、Shell、设备路径或任意 URL。隔离 Debug 包仍不能接收
正式目标包的凭据，CI APK 的构建成功不代表真实配对成功。

## 主要变更

| 文件或范围 | 作用 |
| --- | --- |
| `LocalHttpTransport.kt` / 对应 V8 测试 | 初始响应不参与身份与错误判断；独立等待新私有交付，交付后取消慢触发；并发、取消、伪造资料及 401 恢复回归 |
| `httpapi/android.go` / `provision_security_test.go` | 在交付互斥前校验请求正文，避免匿名慢请求阻塞正常交付；验证鉴权边界、无敏感响应、冷却及并发 |
| `platform/VerifyManagerTest.java` / Android 测试桩 | 用户 0、独占 UID、现代与旧 API 签名数量/指纹拒绝用例；不改 DEX 生产验证逻辑 |
| `ModuleApiBridge.kt` / `ModuleApiLifecycle.kt` | Activity 实例归属、代际取消、有界任务与 SSE、销毁后的回调收尾；同一共享安全传输跨宿主恢复 |
| `ModuleApiPlugin.kt` / 插件测试 | 用官方生命周期和 Channel 协议重绑宿主、替换失效的可见性监听；不新增 IPC 命令 |
| `native.ts` / `native.test.ts` | 同步抛错、迟到响应、pagehide、监听注册和订阅取消竞态；复用既有 JS 重连 |
| `LifecycleProbe.java` / Android 测试配置 | 仅测试 APK 使用的 framework instrumentation；真实调用 Activity.recreate，不给生产壳加测试入口或权限 |
| `.github/workflows/v9-shell.yml` / 插件 Gradle | 独立无正式签名 Secrets 的 Rust/Kotlin/Lint/Debug APK 验证，直接编译 V8 共享传输测试 |
| 根 `AGENTS.md`、README、兼容性与 Transport 文档 | 同步首次配对与工程边界，原始 V9 规范不回写 |

## 安全及错误恢复

- 初始请求固定 `{}`，不携带 Bearer/Cookie，不重定向、不自动重试、不解析响应内的证书或 Secret。
- 新交付必须来自 Binder UID 0，并与预期地址匹配。恶意监听能拒绝服务，不能通过伪造成功响应获得身份。
- 有资料时始终精确钉扎证书并检查主机名；身份错误先拒绝该请求，不能直接接受新证书。
- 真实 401 清除本次资料版本，后续连接重新走私有交付，不覆盖并发新资料，不自动重发写请求。
- Go 错误 Envelope 可以省略 `data`。本轮真机发现桥接曾将其误报为 502，已补缺失/空值错误响应回归；成功读取及 SSE 仍必须有有效 `data`。
- 后台、宿主销毁、文档离开会取消自身任务和事件流。恢复等待新完整快照；不使用第二个重连循环或轮询。

## 真实设备证据

设备为已授权的 Mi 10，Android 16 / API 36。测试包保持 `com.fanjv.netproxy`，
固定官方签名，版本码从既有 1091 递增到 1092。测试原始资料保存在 Root 私有备份，
设备日志、截图、APK 和自动化脚本仅留在忽略的 `.agents/runs/2026-10-16-tauri-m3-trust/`。

| 场景 | 实际步骤与结果 |
| --- | --- |
| 首次可信连接 | 清空测试应用数据，确认无连接文件；随后又完成卸载测试包、重新安装固定签名包的全新沙箱验证。无人工注入资料，自动私有交付后 info/service 均为 HTTP 200、schema=1 |
| JS 秘密边界 | 原生 read 返回对象检查没有 Secret、证书指纹、Authorization 或私钥字段 |
| 错误签名 | 在已备份且清空的测试沙箱内安装同包名 Android Debug 签名 APK；自动连接失败、没有私有连接文件。之后卸载该测试包，重新安装固定签名 APK，自动连接成功 |
| 无管理器 | 包不存在时，用设备 Root 的实际 DEX 执行同一 provision，返回 `android.query_failed`，没有扩大交付目标 |
| 非 Root 调用 Provider | 以 UID 2000 调用固定 authority，被 Android 的非导出 Provider 权限检查拒绝 |
| 恶意监听/身份替换 | 停止 Worker，以 ADB reverse 指向合成 HTTPS 监听。既有安装证书钉扎导致 TLS 拒绝；监听只收到匿名 provision，Authorization 始终不存在；伪造成功与伪造连接资料未被采用，恢复后私有资料内容与备份一致 |
| 真实 401 | 临时把应用私有测试副本的 Secret 置为无效值，保留正确安装指纹；最终修正版首次返回 `401 / api.unauthorized`，下一次只读连接经真正 Provider 恢复为 `200 / api.info`；没有修改 Go API 配置 |
| Worker 恢复 | 停止 Worker，读取明确失败；启动后通过既有恢复或主动连接重新读取真实快照，未借此重启核心 |
| 核心重启 | 使用公共 service restart；真实 SSE 恢复 status/groups/subscriptions/traffic，流量身份与新核心一致 |
| Activity 真正重建 | 已签名测试 APK 使用 `Activity.recreate()` 连续两次，确认旧实例 isDestroyed、新对象不相等；每次重建前建立 SSE、重建后 info=200，instrumentation 返回 recreated=2、成功码 -1。401 修正版上再次执行同一探针通过 |
| WebView 文档重载 | 重载实际 WebView 后，info/service 与事件读取再次成功 |
| 前后台 | 后台 native visibility=false，读取返回 api.background；回前台恢复读取与事件，未把 WebView 的 document 可见性当作唯一依据 |
| 旋转/强停 | 横屏下真实读取成功；强制停止后再次启动仍可读取。旋转由 configChanges 处理，不替代 Activity.recreate 证据 |

测试中的 Worker 停启、核心重启和应用数据重建仅在开发机执行，不是普通 UI 获得了
写入权限。完整销毁期间的取消/迟到回调还由单元测试覆盖，不将它们等同于所有 ROM 的实测。

验收后使用固定签名的 V8 Release APK 覆盖恢复，不降级版本码。原始 no_backup、
SharedPreferences 与 WebView 数据从设备 Root 私有归档恢复，按重新安装后的实际 UID
恢复所有权与 SELinux 私有分类。连接文件与备份摘要一致，V8 仪表盘可读、无连接错误，
服务保持 ready/TUN。临时 ADB reverse/forward、instrumentation APK 与测试设置已清理；
备份留在设备 Root 私有目录，没有把原始资料或签名材料加入 Git。

## 自动化验证

以下均为实际执行结果：

| 命令 / 目录 | 结果 |
| --- | --- |
| `pnpm check` / `apps/web` | TypeScript、ESLint、格式检查、Vitest 97 项通过 |
| `pnpm test:e2e` / `apps/web` | Playwright 10 项通过，包含桌面、移动、普通生产及 Native 浏览器隔离 |
| `cargo fmt --all --check`、`cargo test -p tauri-plugin-module-api --locked`、`cargo clippy --all-targets -- -D warnings` / Rust workspace | 通过，Rust 2 项测试 |
| 插件 `testDebugUnitTest`、`lintDebug` 与应用 `lintUniversalDebug` | 初轮 87 项 JVM 测试通过；401 回归后最终 90 项 JVM 测试与插件 Lint 通过；应用 Lint 初轮通过 |
| `pnpm tauri android build --debug --apk --target aarch64 -- --locked` | arm64 Debug APK 构建通过；同包固定签名验证使用显式 upgrade 配置 |
| `assembleUniversalDebugAndroidTest` | 测试 APK 编译并在真实设备执行通过 |
| V8 `:app:testDebugUnitTest :app:lintDebug assembleDebug` | 376 项应用测试、Lint 和构建通过 |
| V8 `:platform:test` | 18 项 Java 测试通过 |
| V8 `:app:assembleRelease :platform:assembleDex` | Release/R8、框架 DEX 构建通过，固定签名回退包已实际安装 |
| `sh tests/verify.sh quick webui` | Go test/vet、Shell/Catalog/安装/打包/发布契约、旧终端 13 项测试与构建通过 |
| `git diff --check` | 通过 |

Windows NTFS 权限位和没有 flock 的互斥断言仍由原脚本明确跳过，没有宣称相应 Linux
断言已在 Windows 通过。旧版 Tauri 仍要求的 AGP 兼容开关保持可见，不隐藏警告。

## 远端 CI

首次执行：[Verify V9 Android Shell](https://github.com/Fanju6/NetProxy-Magisk/actions/runs/38874706706)，
提交 `9e85314f`，实际成功，约 8 分 51 秒。同提交的独立 V9 Web 验证也通过。
最终 401 修正版：[原生 CI](https://github.com/Fanju6/NetProxy-Magisk/actions/runs/38875926562)，
提交 `85fc2a4f`，实际成功，原生任务用时 5 分 17 秒，Rust、Kotlin、应用 Lint、两种
Debug APK 编译、身份/ABI/签名与锁文件检查全部通过。后续提交仅整理文档，不改变该已验证源码。

该流水线只读取源码，无正式签名 Secrets，不进入 V8 发布。Markdown-only 修改不运行
大型原生构建；APK 来源/ABI/Debug 标记和锁文件不变均受检查。CLI 已完成 Rust 构建后，
Gradle 复用同一 arm64 产物，仅排除重复调用 CLI 的 Rust task，不跳过 Kotlin、Lint 或测试 APK 编译。

## 限制与停止边界

- 本轮单机证据不能扩展为 Magisk、KernelSU、APatch 与所有厂商 ROM 的完整矩阵。
- 无 Root 模块场景仅验证了接口不可达/Worker 停止；没有卸载设备 Root 框架。
- 交付超时、并发、迟到回调和慢消费者有自动化测试；没有在设备人为卡死 Binder 15 秒或完成小时级内存/后台压力测试。
- Tauri 2.12.4 的 `PluginManager.plugins -> PluginHandle.instance -> Plugin.private activity` 会保留构造时 Activity；上游 onDestroy 只移除 launcher，load 不重建插件。本插件已重绑自己的 owner 并释放自己的 Job、Call、队列和监听，但不能通过受支持的子类 API 清除该引用，也没有反射修改私有字段。首个 Activity 仍可达，不代表每次重建都会额外累积一个 Activity；保留堆大小未测量。这是需要上游所有权改进的明确工程限制，不能宣称已经完成零泄漏验收。
- 自定义 API 端口的首次发现未新增；无资料时仍使用既有固定 loopback 默认入口。
- Tile、分享、文件导入导出仍是后续迁移项目，没有扩张本轮只读 IPC。
- 不合并 main、不打 tag、不发布、不开发 Windows/Linux，不开始完整业务迁移。
