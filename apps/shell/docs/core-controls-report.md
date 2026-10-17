# M4.2：节点选择、出站模式与测速

验收日期：2026-10-16 至 2026-10-17。分支：`feat/v9-webui-foundation`。
开发基线：`ab590d9cd780e77970e0b720d8beab7ed7abad85`。

本轮不修改 Go 业务、V8 管理器、终端 WebUI、模块配置结构、安装和发布流水线。
没有实现订阅编辑、节点编辑/删除、配置保存、应用策略或桌面端；不合并 main，不发布正式 APK。

## 调用链与权限

```text
Nodes / Dashboard -> 领域 Hook / TanStack Query -> ApiClient / Zod
  -> NativeTransport -> Tauri Capability
  -> Rust Resource / NodeTarget / 固定命令
  -> Kotlin ModuleApiBridge 固定方法、路径、正文和独立校验
  -> 直接复用 V8 LocalHttpTransport
  -> 私有 Provider、精确证书 SHA-256、主机名及 Bearer
  -> Go HTTP API -> 原有领域函数
```

| JS 操作能力                             | 原生入口                                | 固定 Go 接口                               |
| --------------------------------------- | --------------------------------------- | ------------------------------------------ |
| info.read / service.read                | read(info/service)                      | GET /info、/service                        |
| nodes.read / selection.read / mode.read | read(nodes/selection/mode)              | GET /nodes/snapshot、/selection、/mode     |
| service.start / service.stop            | service_action(start/stop)              | 原有 POST /service/start、/service/stop    |
| selection.write                         | select_node(target)                     | PUT /selection                             |
| mode.write                              | change_mode(mode)                       | PUT /mode                                  |
| nodes.delay                             | test_delay(target)                      | POST /nodes/delay                          |
| 原有事件                                | open_events / next_event / close_events | status/groups/subscriptions 及附属 traffic |

所有 HTTP 路径都有固定 `/api/v1` 前缀。目标参数严格为 `{group_id,node_tag}`；
分组 ID 禁止路径穿越与 staging，原始 tag 保留 Unicode、空格和斜杠，空 tag 表示组内 Auto。
Rust 使用类型化参数及未知字段拒绝，Kotlin 再独立检查字段和类型，不接收前端任意 URL、
方法、路径或设备文件。模式名非空，其是否可用由 Go 的原生模式列表最终校验。

Secret、证书指纹和私有交付资料不进入 JS；没有新增 Root、Shell、文件或通用 HTTP 权限。
请求与响应继续受 4 MiB 上限约束。Kotlin 复用原有 TLS、Bearer、Provider、取消和非重放
实现；共享写入准入槽防止并发原生写入，其他读取继续有界。未复制安全传输源码。

## 功能矩阵

| 功能            | 本轮实现                                           | 验证范围                                      |
| --------------- | -------------------------------------------------- | --------------------------------------------- |
| 持久分组和节点  | GET 完整快照，保留搜索、折叠/展开、网格/列表       | 自动化多组/多节点；真机一组一节点             |
| 手动选择        | 请求持久 group_id 与原始 tag，显示已确认选择       | 真机 UI、Go 选择回读及自动化                  |
| 组内 Auto       | 空 node_tag，区分持久 Auto 与实际节点              | 真机 UI、空组无 Auto、多组 Mock               |
| 运行时关联      | 使用 group.runtime_tag/tag，不用 UUID 拼运行时标签 | 匿名夹具及真机；长名、斜杠和 Unicode 自动化   |
| 模式可选项      | 来自 GET /mode.available，支持非固定模式名         | 自定义模式组件测试；真机原生模式              |
| 默认与实际模式  | 分开显示；网络策略差异不直接认定失败               | 组件测试及真实模式读写                        |
| 停服保存        | 节点、Auto、默认模式仍可保存                       | 真机通过，服务保持 stopped                    |
| 单节点测速      | 校验返回 target、group 与 item 的运行时引用        | 真机及无效响应回归                            |
| 分组测速        | 空 tag，返回组内最近可用延迟                       | 真机 UI、桌面/手机 Playwright                 |
| 离线测速        | 调用现有 Go 隔离测试能力，不启动受管服务           | 真机通过，受管服务仍 stopped                  |
| 测速状态        | 等待、失败、未测/无结果、有效延迟与缓存说明        | 单测、组件、浏览器与真机                      |
| 待确认与副作用  | 不重放，保留错误及 persisted，GET/SSE 复核         | 真实取消已接受请求；错误组合自动化            |
| 服务 start/stop | 保留 M4.1 语义及停止确认                           | 38 项既有服务控制回归，真机停启               |
| 尚未授权写入    | 配置、订阅、节点编辑删除及其他服务动作仍拒绝       | Rust/Kotlin/Transport 测试、真机 IPC 越权检查 |

## 状态与恢复

- GET selection 的手动 `selected` 为持久引用，PUT selection 的同名字段为运行时标签。
  PUT 只用于校验操作响应，不能当完整节点快照发布。选择结果从 selection/nodes 读取。
- 节点和模式共用一个短生命周期操作槽，分别读取各自权威资源。不扩大为通用业务状态机，
  不新增全局 Store、Repository、业务数据库或重连框架。
- 同步登记意图防止跨组件重复点击；mutation 显式 `retry=false`、`networkMode=always`。
  页面离开不撤销已接受的持久写入，后台取消原生等待也不等于 Go 回滚。
- 401/403/409/422 等明确拒绝不重发；断线、超时、无效响应及 502/504 等显示待确认。
  150 秒观察窗口覆盖 Go 120 秒与原生 135 秒预算，不是回滚或自动重试计时器。
- `node.runtime_sync_failed`、`mode.runtime_sync_failed` 和 `persisted=true` 保留；已保存
  数据不回滚，不因 GET 已读到新值就吞掉明确的业务错误。用户可以只读复核，不补发写入。
- 模式应用可能重建核心，先 GET service 再读取 mode/nodes/selection，避免正确的新核心
  读取被开始时的旧身份保护误丢弃。旧请求仍由操作 epoch、取消和核心身份隔离。
- 新 SSE 连接也可能重放存活 Hub 的旧配置缓存。routing 操作后先消耗完整初始快照，再
  进行权威读取；期间不发布旧 status/groups/mode，不把初始重放当确认。复核中观察到核心
  或服务状态变化且与 GET 不一致时，交给既有有界重连重新校准，不能直接宣告 live。
- 核心身份切换清除旧运行时选择、流量历史和本页测速结果。初始 groups 先于 status 时
  暂存，不能在未确认实例身份时直接发布。已有服务启停流程不改为另一套重连模型。
- 测速响应可能是本轮或旧缓存，`url_test_time` 单位为 Unix 秒。未得到有效结果不声称
  全部测完；请求失败不会清空此前有效延迟。每页同一等待只发一次，取消、离页和后台后
  忽略迟到结果；取消等待不撤回 Go 已接受的有界测速，之后只允许用户显式发起新测试。

## 实际设备验证

已授权专用 Mi 10，Android 16 / API 36。固定官方签名、同包名 `com.fanjv.netproxy`，
版本码 1096 -> 1097 覆盖安装成功，没有卸载或清除应用数据。1097 是测试覆盖计数，
没有修改正式 V8 版本或发布包。最终构建包重新安装并重复核心操作验收。

| 场景                 | 实际结果                                                                                             |
| -------------------- | ---------------------------------------------------------------------------------------------------- |
| 手动/Auto 与连续点击 | UI 同步连点三次，选择经真实 GET 复核；切回 Auto 成功                                                 |
| 出站模式             | 从真实 available 列表选择另一模式，GET mode 确认保存及运行时状态                                     |
| 单节点/分组测速      | 原生单节点调用和 UI 分组按钮成功，target/items 与真实标签一致                                        |
| 停服操作             | 服务 stopped 时手动/Auto、模式保存和离线测速均成功；测速未启动受管服务                               |
| 已接受选择后丢失等待 | 暂持生命周期锁，原生请求等待时取消本次 Invoke；返回 api.cancelled，释放锁后选择仍完成；未补发写入    |
| Worker 中断恢复      | Worker 停止时读取失败，恢复后新快照与节点控件可用，核心启动身份不变                                  |
| 前后台               | 连续三轮后台/前台，节点和模式重新读取成功                                                            |
| Activity 真正重建    | 测试 APK 两次 Activity.recreate，旧实例销毁；每次 info/nodes/selection/mode 读取成功，重建前存在 SSE |
| 真机视觉             | 实际屏幕核对默认/运行时模式，节点页浅深色截图正常；恢复原主题和语言偏好                              |
| 实际业务拒绝         | 不存在的模式返回 422/mode.invalid；非法目标、未授权读取和 reload IPC 被拒绝                          |
| 数据恢复             | 原始选择、模式与 ready 状态恢复；逐项比较 module.json 与主配置的完整 JSON，与备份相等                |

匿名测试日志、脚本、截图和签名 APK 留在忽略的本地运行目录，原始设备资料不提交。
设备备份留在 Root 私有测试目录。没有在本轮篡改证书、关闭钉扎或向 JS 注入 Secret。

## 自动化与构建

主要源码范围：Web 的 `api/contracts.ts`、`native.ts`、`queries.ts`、`events.tsx`、新增
`routing-control.ts` 与 `node-delay.ts`；`features/nodes.tsx`、`dashboard.tsx`、中英文本和
匿名 Mock；原生插件 `src/lib.rs`、`build.rs`、权限生成物、`ModuleApiPlugin.kt` 与
`ModuleApiBridge.kt`。对应契约/组件/E2E、Kotlin/Rust 测试、Activity 探针和文档同步。
完整文件清单以本轮源码提交 diff 为准，V8 与 Go 生产源码没有改动。

本机 pnpm 全局 shim 不适合子进程，实际用 `npx --yes --package=pnpm@12.11.2 pnpm`
执行相同锁定版本；CI 直接使用 pnpm。

| 实际命令                                                                                                       | 结果                                                                        |
| -------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| apps/web：pnpm check                                                                                           | TypeScript、ESLint、Prettier、11 文件 208 项 Vitest 通过                    |
| apps/web：pnpm test:e2e                                                                                        | 桌面/手机、普通生产/Native 浏览器隔离，12 项通过                            |
| pnpm build / build:native / build:demo                                                                         | 三种构建与安全/资源检查通过；前两者也由 Playwright 实际执行                 |
| cargo fmt --all --check                                                                                        | 通过                                                                        |
| cargo test -p tauri-plugin-module-api --locked                                                                 | 4 项通过                                                                    |
| cargo clippy -p tauri-plugin-module-api --all-targets --locked -- -D warnings                                  | 通过                                                                        |
| 插件 testDebugUnitTest、lintDebug；应用 lintUniversalDebug                                                     | 105 项 JVM 测试及 Lint 通过，包含共享 V8 安全传输回归                       |
| pnpm tauri android build --debug --apk --target aarch64 --config src-tauri/tauri.upgrade.conf.json -- --locked | arm64 最终 APK 构建通过并签名实装                                           |
| assembleUniversalDebugAndroidTest / am instrument                                                              | 测试 APK 构建；真机 recreated=2、nodes_mode_after_recreate=true、成功码 -1  |
| Go：go test ./...、go vet ./...                                                                                | 通过；额外目标包详细回归 194 通过、14 跳过                                  |
| sh tests/verify.sh quick webui                                                                                 | Go、Shell/Catalog/安装/发布契约、36 项工作流测试、13 项旧终端测试与构建通过 |
| V8 Gradle：app:testDebugUnitTest、app:lintDebug、app:assembleRelease、platform:test、platform:assembleDex      | 376 项应用测试通过，Release 构建；平台 18 项及 DEX 复用 UP-TO-DATE          |
| git diff --check                                                                                               | 通过                                                                        |

## 限制与未验证

1. 真机仅有一个非空分组和一个节点，未改动 Catalog 来伪造更多用户数据。跨组、多节点、
   重名/长名等由匿名契约、组件和浏览器测试覆盖，不能视为多组真机验收。
2. 真实取消验证“失去等待不等于未执行”，没有穷举 TCP/TLS 断裂时序。本轮没有人为破坏
   正式配置来制造 persisted 失败；401/403/409/422/502/504、超时和副作用组合主要由自动化覆盖。
3. Go 没有跨资源 revision、事件全序号或操作 task_id。当前边界避免旧会话和初始 Hub
   重放覆盖本次确认，但不承诺多个外部客户端同时修改时的线性一致性。进程重建只读取
   当前状态，不恢复或重放已失去关联的写入。
4. Windows Go/Shell 的权限、flock 及真实 sing-box 相关断言有明确跳过；Android 平台单测
   和 DEX 本轮复用缓存，不声称重新执行。V8 Lint 原有 25 警告、1 提示未清理。
5. 没有重做全部 ROM、Root 框架、错误签名和证书替换验收；既有证据仍见 M3。上游 Tauri
   的旧 Gradle DSL 和 Activity 引用限制仍保留，不宣称长期零泄漏或全平台兼容。
6. 普通浏览器生产默认仍无可信连接；订阅、配置及其他高级写入留待独立后续里程碑。

## 远端验证记录

源码提交：`c9f79585ce02feb0cdf38bc7bae39a5cf20b6603`。

- [V9 WebUI CI](https://github.com/Fanju6/NetProxy-Magisk/actions/runs/38988000306)：成功，耗时 1 分 34 秒，远端 Playwright 12 项通过。
- [V9 Android Shell CI](https://github.com/Fanju6/NetProxy-Magisk/actions/runs/38988000234)：成功，耗时 5 分 20 秒，Rust、Kotlin、Lint、隔离 arm64 APK、身份/签名及锁文件检查通过。

远端验证不读取正式签名 Secrets；上传的隔离 Debug APK 不是正式版或同包覆盖验收替代品。
