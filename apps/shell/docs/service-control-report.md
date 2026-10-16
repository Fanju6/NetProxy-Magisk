# M4.1：真实服务控制与能力边界

日期：2026-10-16。分支：`feat/v9-webui-foundation`。
开发基线：`be0a4d9ace59188d6cfc46f7c782b98c1384f4d4`。

本轮只增加真实服务启动和停止。未修改 Go 业务、持久化格式、模块安装或 V8 发布流程，
没有开放节点、模式、配置、订阅写入，也未实现 restart/reload/check。

## 调用链与授权

```text
Dashboard -> useServiceAction / TanStack Query -> ApiClient / Zod
  -> NativeTransport -> service_action(start|stop)
  -> Tauri Capability -> Rust ServiceAction 枚举
  -> Kotlin serviceAction -> 固定 POST 路径 + {}
  -> 直接复用 V8 LocalHttpTransport
  -> 私有 Provider / 精确证书钉扎 / 主机名 / Bearer
  -> Go HTTP API -> ManageService -> 生命周期锁 -> sing-box
```

| 层 | 已开放 | 仍拒绝 |
| --- | --- | --- |
| NativeTransport / ApiClient | info.read、service.read、service.start、service.stop；原有 SSE | 其他请求操作、路径、query 和前端正文 |
| Rust | Info/Service 资源枚举、Start/Stop 操作枚举；既有取消、事件和可见性命令 | 任意方法、URL、命令、文件及其他服务操作 |
| Kotlin | 固定 GET /info、GET /service、POST /service/start、POST /service/stop；status/groups/subscriptions 与附属 traffic | 其他路径、动作和并发服务写入 |
| UI | 真实状态、启动、停止确认 | 模式、节点选择、配置写入等尚未迁移的控件 |

细粒度 `capabilities` 取代 `readOnly`，前端标记不是安全授权来源。即使直接调用 IPC，
Rust 与 Kotlin 仍执行独立白名单。Mock 保留既有演示行为，普通 Web 和普通浏览器中的
Native 构建继续失败关闭。未新增 Root、Shell、FS、通用 HTTP 或远程页面权限。

Secret、安装证书与其指纹不进入 JS。原生错误消息统一脱敏；业务错误仅保留原状态码、
错误码和布尔 `persisted`，不转发其他错误详情。成功和错误继续校验 schema=1，正文
上限仍为 4 MiB，服务操作正文由 Kotlin 固定为空对象。

## 状态与恢复

- 同步在 QueryClient 登记意图，跨组件和同步连续点击只接受一项操作；不用新的全局 Store。
- 页面离开不取消已接受写入。后台/销毁可以取消原生等待，但 Go 已接受的操作可能继续完成。
- 显式关闭 mutation retry，使用 `networkMode=always`。loopback info/service GET 同样不受浏览器在线标记阻塞，不能把用户操作排队到重新联网后再发送。
- 不把 POST 返回的状态直接发布为乐观结果。操作结束后使旧事件代际立即失效，先 GET /service，再建立新 SSE；保留原有核心实例与流量隔离。
- Go 没有跨 GET/SSE 的事件序号。必须通过新的事件会话建立观察顺序，不能凭到达时间认定旧帧更新。复核 GET 与 SSE 收齐快照分别有原生请求时限和独立 30 秒快照窗口。
- 401/403/409/429 等明确拒绝不会被自动重放。通信故障、取消、无效响应、502/504 等先显示“结果待确认”，复核只发 GET。保留主动事件重连和已有三次有界自动重连。
- 有效错误 Envelope 表明后端已返回，但不表明没有副作用。已返回操作的终态与目标状态经权威读取确认；保留明确后端错误和 `persisted=true`，不会因服务已达到目标就吞掉错误。
- 没有业务响应时，临时 failed 不能证明启动结束。目标状态可以解除等待；其他终态需要 150 秒观察窗口后再次观察才解除保护。窗口覆盖现有 Go 120 秒预算和原生 135 秒调用等待并留余量，不代表后端回滚或所有内部清理步骤的硬上限。没有倒计时轮询或超时补发。
- 完整 WebView/进程重建会丢失旧页面的短时操作记录；新页面只读取 Go 状态和完整事件快照，不恢复或重放旧写入。没有后端 task_id，无法跨进程精确关联某一条已丢失响应的操作。

## 代码范围

| 路径 | 变化 |
| --- | --- |
| `apps/web/src/api/contracts.ts`、`client.ts`、`native.ts` | 操作能力、方法/路径校验、受限 start/stop IPC、保留有效业务响应来源 |
| `apps/web/src/api/service-control.ts`、`queries.ts`、`events.tsx` | 小型操作元数据、重复提交保护、只读复核、GET/SSE 代际边界和完整快照窗口 |
| `apps/web/src/features/dashboard.tsx`、`nodes.tsx`、`settings.tsx` | 真实停止确认、待确认状态、各操作独立禁用；没有扩大其他页面写入范围 |
| `apps/web/src/app/i18n.ts`、`shell.tsx`、`components/states.tsx` | 中英文能力提示、真实操作文案与错误分类，移除过时的只读/不自动配对描述 |
| `apps/web/src/mocks/transport.ts`、相关单测与组件测试 | Mock 能力对齐；重复操作、断线/超时/取消、持久副作用、旧帧、离线标记和慢 GET 回归 |
| `apps/shell/plugins/module-api/src/lib.rs`、`build.rs`、`permissions/` | Start/Stop 枚举和单一受限命令；官方构建生成权限清单 |
| 插件 `ModuleApiPlugin.kt`、`ModuleApiBridge.kt`、两份对应测试 | 固定 POST 映射、单写入准入、生命周期取消、安全错误元数据；直接复用 V8 传输 |
| `apps/shell/src-tauri/tauri.upgrade.conf.json` | 本地覆盖验证版本码递增至 1096，不改正式模块或 V8 版本流程 |
| 根 AGENTS、Web/Shell README、架构/Transport 文档、安全 ADR | 同步 M4.1 的能力边界和恢复决策；不回写原始目标规范 |

## 真机验证

设备为已授权的 Mi 10，Android 16 / API 36；仅在专用开发机执行。
本地脚本、APK、截图与 XML 留在忽略的 `.agents/runs/2026-10-16-v9-m41-service/`，
不将原始设备资料或签名材料提交。

| 场景 | 实际结果 |
| --- | --- |
| ready -> stopped -> ready | 通过真实仪表盘停止确认和启动操作，Go 状态与 UI 一致；最终包再次通过 |
| 连续点击与切页 | 同一启动按钮同步点三次，立即进入节点页再返回；服务日志只新增一次启动、一次对应停止，未重复执行 |
| 停止确认取消 | 取消对话框，服务保持 ready，没有发出停止操作 |
| 直接 IPC 越权 | restart、reload、check、toggle、伪造路径/URL 和 config 读取均被拒绝；模式控件保持禁用 |
| 操作中后台/前台 | 停止和启动期间进入后台再恢复，原生可见性正确变化，最终状态恢复；没有补发写入 |
| 已接受启动后取消等待 | 先确认 Go 已记录启动，再取消对应原生请求；返回 api.cancelled，后台继续达到 ready，日志只记录一次启动 |
| Worker 中断 | 停止 Worker 时读取失败，重新启动后连接恢复；核心启动身份未变，没有借机重启 sing-box |
| 后端明确拒绝 | 测试占用真实生命周期文件锁后从 UI 启动，后端拒绝且 UI 保留错误；没有新增启动执行，释放锁后显式启动成功 |
| 真正 Activity 重建 | framework instrumentation 连续两次 Activity.recreate，旧实例确实销毁，每次可读取真实 info；重建前有 SSE |
| 启动未返回时销毁/重建 | Go 已接受启动且原始 IPC 仍待返回时运行探针；实际重建两次后读取成功，服务最终 ready，日志仅一次启动 |
| 同包覆盖与 V8 回归 | 固定官方签名、同包名，按 V8 1093 -> V9 1094 -> V8 1095 -> V9 1096 递增覆盖；不卸载、不清除数据。V8 仪表盘读取及实际停止/启动均成功，最后保留 V9 |

本轮没有通过改证书、关闭钉扎或给 JS Secret 来构造故障。真实取消已接受 POST 验证了
“失去响应不等于服务失败”；没有宣称已穷举 TCP/TLS 断裂的每一种时序。401/403/409/
502/504 的 UI/IPC 分类、写入不重发与证书安全由自动化覆盖；错误签名和证书替换的
既有真机证据仍见 M3 报告，不把本轮未重复执行的安全场景称为新设备证据。

## 自动化与构建

| 实际命令 | 结果 |
| --- | --- |
| `pnpm check` / `apps/web` | TypeScript、ESLint、Prettier、Vitest 149 项通过 |
| `pnpm test:e2e` / `apps/web` | Playwright 桌面/手机、普通生产和 Native 浏览器隔离 10 项通过 |
| `cargo fmt --all --check`、`cargo test -p tauri-plugin-module-api --locked`、`cargo clippy --all-targets --locked -- -D warnings` | 格式、3 项 Rust 测试及 Clippy 通过；Android 分支另经 APK 构建编译 |
| 插件 `testDebugUnitTest`、`lintDebug`，应用 `lintUniversalDebug` | 100 项 JVM 测试与 Lint 通过；包含直接编译的 V8 安全传输回归 |
| `pnpm tauri android build --debug --apk --target aarch64 --config src-tauri/tauri.upgrade.conf.json -- --locked` | 最终 arm64 APK 构建通过，固定签名后实际覆盖安装 |
| `assembleUniversalDebugAndroidTest` 与设备 `am instrument` | 测试 APK 编译通过，真实设备探针返回 recreated=2、read_after_recreate=true、成功码 -1 |
| V8 `:app:testDebugUnitTest :app:lintDebug :app:assembleRelease :platform:test :platform:assembleDex` | 376 项应用测试、18 项平台测试（缓存复用）、Lint、Release/R8 与 DEX 构建通过 |
| `sh tests/verify.sh quick webui` | Go test/vet、Shell/Catalog/安装/发布契约、36 项工作流测试、旧终端 13 项测试与构建通过 |
| `git diff --check` | 通过 |

V8 覆盖包的 1095 仅由忽略目录中的本地 Gradle init script 注入，不修改 V8 源码或
正式 CI 版本逻辑。Windows 上原脚本明确跳过的 Linux 权限位/flock 断言不算通过。
本机全局 pnpm shim 不能供 Playwright 子进程使用，因此通过固定版本 pnpm 启动测试；
未为本机环境修改仓库工作流或测试配置。

远端验证使用现有独立 `v9-shell.yml` 与 `verify-v9-web.yml`，不读取正式签名 Secrets。
实际运行链接与结果将在推送后补充，不能以本地通过替代远端验收。

## 限制与停止边界

- 单机测试不代表所有 ROM、Root 框架和长期后台压力矩阵。未增加服务端任务 ID 或持久操作队列，也没有声称取消等同回滚。
- Tauri 2.12.4 保留首次 Activity 引用、AGP 旧 DSL 兼容开关等已知上游限制仍在，见 M3 报告；本轮没有反射修改框架或宣称零泄漏。
- Native 新增写入严格止于 start/stop。未合并 main、未发布正式 APK、未删除 V8、未开发 Windows/Linux，也不进入 M4.2。
