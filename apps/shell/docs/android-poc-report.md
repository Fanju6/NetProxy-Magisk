# Android 原生通信 PoC 验收

日期：2026-10-16。分支：`feat/v9-webui-foundation`。开发基线：`1f334a79cdb21f292730b0f988ce24c6181b035a`。

## 结论

只读纵向链路已在真实 Android 设备打通：Tauri 承载同一份 V9 WebUI，经 Kotlin 精确证书验证和 Bearer 读取既有 Go API，并接收真实 SSE。固定签名、同包名递增版本覆盖、私有凭据保留与 Provider 再次交付均有设备证据。

这是通信 PoC 的结果，不是全功能管理器迁移完成，也不是可直接发布 V9 的结论。首次安装的可信配对仍未闭环；现有磁贴、文件操作和分享未迁移；不宣称 Windows/Linux 支持。

## 实现及文件

| 范围 | 主要文件与变化 |
| --- | --- |
| 独立壳 | `apps/shell/package.json`、锁文件、`src-tauri/Cargo.toml`、`Cargo.lock`、Rust 入口与 Tauri 配置；Android 工程由官方 CLI 生成后维护 |
| 权限 | `src-tauri/capabilities/default.json` 和插件 `permissions/default.toml`；只授权白名单读取、事件与可见性 |
| Rust | `plugins/module-api/src/lib.rs`；资源枚举、ID/主题边界、异步官方移动插件调用、脱敏错误 |
| Kotlin | 插件 `android/.../ModuleApiPlugin.kt`、`ModuleApiBridge.kt`；私有凭据、共用 HTTPS、容量 1 的事件队列、请求取消及原生生命周期 |
| Web | `apps/web/src/api/native.ts`、Transport/ApiClient、事件生命周期、Query、只读页面、Native 构建与测试 |
| V8 | `LocalHttpTransport.kt` 仅增加默认 true 的 `allowProvisioning` 参数及拒绝分支；现有调用行为不变，PoC 显式关闭未验证 TLS 的配对引导 |
| 文档 | 根 `AGENTS.md`、V9 工程决策、原生 Transport、壳 README、兼容性基线及本报告 |

未修改 Go 业务、API 路由、模块数据结构、安装行为、正式版本号、V8 CI、签名或模块打包流程。实际新增文件以本主题提交的 `git show --stat` 为准，构建目录、签名材料和设备证据不提交。

## 调用链与安全

`React -> TanStack Query -> ApiClient/Zod -> NativeTransport -> Tauri Capability -> Rust -> Kotlin -> LocalHttpTransport -> Go HTTPS`。

- 只有 GET info、GET service 与 SSE；没有任意 URL、Shell、su、设备路径或业务写入接口。节点/模式/服务等控件明确禁用，配置读取同样不越出本轮白名单。
- Kotlin 直接编译 V8 安全包源码，不复制客户端，不实例化带自身重连循环的 ModuleApiClient。
- 主 Secret 只从 Android 私有资料进入原生 HTTP Header，不返回 JS。API 业务错误的原始 message/data 不进入桥接错误，日志不记录凭据。
- HTTP 状态与 schema=1 必须同时有效；Kotlin 和 JS 都有 4 MiB 上限与类型检查。证书 SHA-256 和主机名校验继续复用原实现，不信任任意自签证书。
- Kotlin 保留一个订阅、容量 1 的队列和一个在途 pull，读取最多 8 项。新文档替换遗留订阅，旧关闭不影响新会话；取消先到的请求也不会遗漏取消。
- 原生可见性与 document 可见性合并；后台取消原生任务，前台重新等待完整快照。JS 保留既有有界重连与 core_started_at_millis 隔离，不补发业务写入。
- 普通生产包继续 fail closed，Mock 仍需明确演示构建；只有显式 Native 构建且处于 Tauri 环境才接入 IPC。

## 本地自动化

| 实际命令 / 工作目录 | 结果 |
| --- | --- |
| `pnpm install --frozen-lockfile` / `apps/web`、`apps/shell` | 锁文件安装通过；本机损坏的全局 pnpm shim 通过固定 `npx --package=pnpm@12.11.2 pnpm` 执行 |
| `pnpm check` / `apps/web` | TypeScript、ESLint、Prettier、Vitest **83 项通过** |
| `pnpm test:e2e` / `apps/web` | Playwright **10 项通过**，覆盖移动/桌面、普通生产拒绝连接和 Native 构建浏览器隔离 |
| `pnpm build`、`pnpm build:demo`、`pnpm build:native` / `apps/web` | 独立构建与产物隔离通过；Native 输出 dist-native |
| `cargo fmt --manifest-path apps/shell/src-tauri/Cargo.toml --all --check` | 通过 |
| `cargo test --manifest-path apps/shell/src-tauri/Cargo.toml -p tauri-plugin-module-api --locked` | Rust **2 项通过**，验证资源枚举和有界参数 |
| `cargo clippy --manifest-path apps/shell/src-tauri/Cargo.toml -p tauri-plugin-module-api --locked -- -D warnings` | 通过 |
| `gradlew.bat :tauri-plugin-module-api:testDebugUnitTest :tauri-plugin-module-api:lintDebug --no-daemon` / 壳 Android 根 | Kotlin **28 项通过**，Lint 通过；覆盖只读边界、严格 Envelope、限长、背压、取消、旧关闭、文档替换、前后台及销毁 |
| `pnpm android:build` / `apps/shell` | 隔离 Debug APK 构建通过 |
| `pnpm android:build:upgrade` / `apps/shell` | 同包 Release APK、R8、arm64 构建通过；另运行同包 Debug 构建进行 WebView 观察 |
| `gradlew.bat testDebugUnitTest lintDebug assembleDebug --no-daemon` / `src/android` | V8 **372 项通过**，Lint、Debug 构建通过；另构建固定签名回退所需 Release APK |
| `sh tests/verify.sh quick webui` / 仓库根 | Go test/vet、Shell 契约、打包契约、旧终端 13 项测试及资源构建通过 |
| `git diff --check` | 通过 |

没有把 JVM 的 MockWebServer 测试当作 Android TLS 失败实测。V8 原有 TLS/store/codec 安全测试独立运行，不复制到新插件。Windows 上的 NTFS 权限断言和缺少 flock 的互斥分支由原脚本明确跳过，不能据此认定这些 Linux 专属断言已通过。

## 真实设备

测试机：Mi 10、Android 16 / API 36，已获同包覆盖测试授权。应用 target/compile SDK 37、min SDK 26，仅包含 arm64-v8a。APK 元数据和签名均通过 Android SDK 工具检查，证书与 Go 辅助程序固定公开指纹一致。

| 项目 | 实际结果 |
| --- | --- |
| 原地覆盖 | 原 V8 `1085` -> PoC `1091`，同 applicationId、固定签名，`adb install -r` 成功，没有卸载或清数据 |
| 数据 | 私有连接文件安装前后 SHA-256 校验一致；CE no_backup 与 SharedPreferences 已在设备 Root 私有目录备份，未导出 Secret |
| Debug / Release | 两种包均可启动、显示真实 Go 版本和服务状态；Release 已经 R8，不以 Debug 成功代替 Release |
| 真实 GET | info/service 返回 HTTP 200、schema=1；返回对象没有 Secret、证书指纹或认证字段 |
| 真实 SSE | status、groups、subscriptions 和后续 available traffic 均到达，流量启动身份匹配当前服务身份；初始 unavailable traffic 被正确区别处理 |
| 越权探针 | config 资源、业务写入命令、通用 HTTP 和 FS 调用均被拒绝 |
| 私有交付 | 验收主机通过 Root 在内存取得可信指纹，以精确钉扎 HTTPS 请求既有 auth/provision；Go 返回 api.provisioned，新 Provider 成功接收，连接文件内容仍一致。此测试不把 Secret 交给 JS，也不是 PoC 自动首次配对 |
| 前后台 | 原生 visibility 在后台返回 false，即使 document 仍为 visible；后台读取被拒绝，回前台恢复新快照 |
| Worker 中断 | 停止并重新启动 Worker，旧服务快照显示非实时，接口恢复后自动收齐快照；核心身份未变，没有为此重启核心或修改配置 |
| 重建 | WebView reload 和应用 force-stop 后重新启动均能再次读取和接收 SSE；Activity 同进程销毁/重建的完整组合仍未充分验证 |
| 横竖屏 | 实测 CSS 视口约 392x851 与 851x392，无横向页面溢出；旋转走 manifest 的 configChanges，不当作 Activity 重建证据 |
| 键盘 / 返回 | 节点搜索临时输入，系统确认输入法显示；未修改节点。系统返回可关闭键盘并按 Hash 历史导航；更复杂 IME/手势和其他系统仍需覆盖 |
| 明暗与安全区 | 真实 WebView 浅色/深色截图、横屏与输入场景已检查，关键导航可用；不将单机结果扩展成所有 ROM 兼容结论 |

原始截图、测试脚本与产物位于忽略目录 `.agents/runs/2026-10-16-tauri-poc-m2/`；包含设备数据的证据不纳入 Git。本报告只保存脱敏结论。

验收后以相同固定签名、版本码 1091 的 V8 构建覆盖恢复管理器，未卸载或清数据；私有连接资料再次校验一致，服务仍为 ready/TUN。原始 1085 APK 已留存，但不是通过降级恢复；临时 ADB WebView/API 端口转发已移除。最终 PoC 签名产物单独保留供审查，不写入模块打包目录。

## 未完成与后续门槛

1. **首次安装可信配对**：PoC 不调用未经验证 TLS 的引导，没有凭据时安全失败。必须先完成独立安全决策，不能为实现一键连接而接受任意证书或向 JS 暴露主 Secret。
2. **构建上游限制**：Tauri 2.12.4 仍要求旧 AGP DSL / 独立 Kotlin 兼容设置，存在未来 AGP 10 移除警告；详见 README。保留官方稳定机制，没有篡改下载的框架或隐藏警告。这是相对“无弃用配置”要求的明确差异。
3. **组件迁移**：MainActivity 与私有 Provider 身份保留，旧 Compose Application 不照搬；Tile、FileProvider、导入导出和分享未启用。APK 安装/沙箱连续性通过，不代表这些组件已完整兼容。
4. **更多真机安全测试**：本轮未替换设备服务证书、制造真实 401、重启核心或对多个 Root 框架逐一测试。证书错误和旧核心帧有自动化测试，但不能替代这些设备场景。
5. **长期资源测试**：没有完成小时级后台、渲染进程崩溃、低内存与每种 Activity 重建路径压测；有界队列和回归测试不等于已经证明所有宿主生命周期无泄漏。

下一步仅建议先独立审查安全边界与首次配对门槛，通过后再安排受控的组件和业务迁移。本轮不合并 main、不发布正式版本，也不开始桌面实现。
