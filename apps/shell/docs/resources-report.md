# M4.3.1：Catalog 与订阅只读管理

验收日期：2026-10-17。分支：`feat/v9-webui-foundation`。
开发基线：`1f10d7a7fb430147e09abee6188abd1943dc7558`。

本轮不修改 Go 业务、V8 管理器、终端 WebUI、持久化结构、安装或发布流水线。
不开放订阅写入，不合并 main，不发布正式版本。

## 契约与能力矩阵

下表路径均带固定 `/api/v1` 前缀。

| 能力 | 原生入口 | Go 接口 | 范围 |
| --- | --- | --- | --- |
| catalog.read | read(catalog) | GET /catalog | 全部 Catalog 分组摘要 |
| subscriptions.read | read(subscriptions) | GET /subscriptions | 本地与 URL 分组，不是 URL 专用列表 |
| subscriptions.detail | subscription_read(detail) | GET /subscriptions/{id} | 公开分组及节点摘要，本地组也可读 |
| subscriptions.history | subscription_read(history) | GET /subscriptions/{id}/history | URL 订阅；本地组返回 read_only |
| subscriptions.progress | subscription_read(progress) | GET /subscriptions/{id}/progress | URL 订阅；本地组返回 read_only |

Go 的类型实际为 `local` / `subscription`。原 V9 Mock 的 `url` 已修正，不增加别名或兼容分支。
`usage` 包含可选的 upload/download/total/expire；expire 是 Unix 秒。缺失值不等于零或无限。
历史记录按 Go 的稀疏字段读取，不要求失败记录包含节点数、revision 或耗时。
进度只包含 download/convert/validate/apply 阶段、时间和分组身份，没有百分比或任务 ID。

## 调用链与安全边界

```text
Resources -> resourceQueries / TanStack Query -> ApiClient / Zod
  -> NativeTransport -> Tauri Capability
  -> Rust Resource / SubscriptionResource / 分组 ID 验证
  -> Kotlin 固定 GET 路径与独立验证
  -> 复用 V8 LocalHttpTransport、私有 Provider、证书钉扎与 Bearer
  -> Go 公开资源 -> Kotlin PublicCatalog 白名单投影 -> WebView
```

- ID 与 Go 规则一致，拒绝路径穿越、staging、query 和多余字段。前端不能传任意 URL、方法或 query。
- 不提供 `private=true`，新增命令只接受 requestId、groupId、resource 三个参数。
- PublicCatalog 在原生边界剔除 URL、Headers、HWID、诊断原文和未知字段；profile 网页地址固定为空。
  历史保留结构化错误码，可能含下载地址的错误和进度原文只保留安全的结果/阶段说明。
- 该投影同时覆盖既有 nodes 快照和 groups/subscriptions SSE，避免绕过详情入口取得私有字段。
- Kotlin 测试把敏感标记注入 GET、SSE、嵌套字段和历史，验证不能穿过投影。
  私有 Metadata 不能伪装成缺少 active/runtime_tag 的公开 Summary。
- Secret、证书身份和连接资料仍不进入 JS。4 MiB 上限、取消、证书精确校验及 Bearer 隔离保持不变。
- 没有新增 Root、Shell、文件系统或通用 HTTP 权限；原有业务写入范围不变。
  订阅新增、编辑、删除、更新、更新全部和取消仍无原生入口。

## 页面与状态

- 导航改名为“资源”，沿用移动端小标题栏和底部导航；保留桌面响应式布局与明暗主题。
- 分开显示本地组和 URL 订阅；显示节点数、活动组、更新时间、自动更新、间隔、下次更新、流量和更新错误。
- 详情使用真实 GET；本地组不请求 URL 专属历史/进度，也不展示未接入的修改按钮。
- 明确区分加载、读取失败、真实空列表、暂停和非实时快照；Native 不使用 Mock 兜底。
- catalog 和 subscriptions 共用一份摘要缓存，不新增全局 Store 或第二套重连器。
- 首个 SSE 快照不能直接覆盖更晚的 GET；先取消在途列表读取，再重新获取权威列表。
  成员增删同样重新 GET；相同成员按分组 revision/updated_at 合并。变化和删除使相关详情失效。
- 会话撤销递增资源 epoch 并取消读取；旧会话或被取消的 GET 不再发布。原有核心启动身份隔离保留。
- 仅正在更新的详情每 5 秒刷新进度，单次观察最多 60 秒；错误、后台、卸载停止。
  用户手动刷新或新前台会话可重新观察，不进行订阅写入或无限重试。
- 进度消失后重读详情和历史，不能据此宣告成功。Mock 包含两本地组、两 URL 订阅及成功/失败/活动进度。

## 源码位置

- `apps/web/src/api/contracts.ts`、`native.ts`：真实 DTO 与能力边界。
- `apps/web/src/api/resources.ts`、`events.tsx`、`queries.ts`：资源查询、SSE 与有界观察。
- `apps/web/src/features/subscriptions.tsx`、`app/shell.tsx`、`app/i18n.ts`：资源页面与双语布局。
- `apps/web/src/mocks/`、API/页面测试及 `e2e/app.spec.ts`：真实结构演示与回归。
- `apps/shell/plugins/module-api/src/lib.rs`、`build.rs`、`permissions/`：受限只读命令。
- `apps/shell/plugins/module-api/android/.../ModuleApiBridge.kt`、`ModuleApiPlugin.kt`、`PublicCatalog.kt`：独立校验和安全投影。
- 对应 Kotlin 测试、Android `LifecycleProbe.java`：拒绝越权、资源释放与实际 Activity 重建。
- `AGENTS.md`、Shell README、Web 工程决策与本报告：实现边界与证据。

## 自动化验证

| 命令 | 实际结果 |
| --- | --- |
| apps/web: `pnpm check` | 类型、ESLint、格式检查通过；13 文件、253 项 Vitest 通过 |
| apps/web: `pnpm test:e2e` | 14 项通过，桌面/手机、双主题、Mock/生产/Native 浏览器隔离；包含生产及 Native 构建 |
| apps/web: `pnpm build:demo` | 演示独立构建通过 |
| Cargo fmt / module-api test / clippy --locked -D warnings | 通过，5 项 Rust 测试 |
| Gradle module-api:testDebugUnitTest / lintDebug / app:lintUniversalDebug | 通过，111 项 Kotlin 单测 |
| Tauri android build --debug --apk --target aarch64 -- --locked | 构建通过；本地同包名覆盖参数单独注入，未改正式版本 |
| `sh tests/verify.sh quick webui` | Go test/vet、Shell/Catalog/模块契约、36 项工作流测试、13 项旧终端测试及构建通过 |
| V8 Gradle app:testDebugUnitTest / app:lintDebug / platform:test | 通过；app 376 项、platform 18 项（平台测试缓存命中） |

Windows 上需 POSIX/flock 或真实内核条件的既有脚本检查按原测试报告跳过，不能视为设备验收。
Tauri/AGP 的现有 deprecated DSL 提示及 V8 既有 lint 警告未在本轮作无关修改。
远端 WebUI 与 Android Shell 工作流必须在本次提交上通过；运行链接随交付提供。

## 实际 Android 验证

专用 Mi 10，Android 16 / API 36。使用既有固定签名和 `com.fanjv.netproxy` 同包名覆盖，
最终本地验证版本码 1100；未卸载、清除数据或重新生成凭据。

- 真正通过 Kotlin/HTTPS 读取 catalog、subscriptions 和公开详情；列表一致，无 Mock 兜底。
- 现场只有一个本地组、一个节点，URL 订阅数为零。
- 真机直接 IPC 越权测试拒绝 6 项非法资源/私有 query/订阅写入请求。
- 浅色本地详情与深色资源列表实际 WebView 截图检查通过。
- 三轮前后台恢复、WebView reload 后重新获取实际资源，等待完整会话恢复，无页面异常。
- instrumentation 实际执行两次 Activity.recreate，验证新的资源读取、已有节点/模式读取及销毁前 SSE。
- 核心 PID 与测试前一致；module.json、inbound.json、sing-box 主配置哈希不变。
  本轮未操作节点、模式、订阅或代理服务，没有重启 Worker/核心。

设备证据、签名测试包与脱敏聚合结果仅保存在 Git 忽略的独立运行目录，不提交设备数据。
URL 订阅的真实流量、历史、失败更新和活动进度 **没有真机样本验证**；仅有真实契约自动化覆盖。
本轮不是新的跨 Root 方案、厂商 ROM 或正式发布 APK 升级认证。

## 限制与 M4.3.2 建议

1. Go 摘要无全局 revision，历史/进度无任务 ID 或单调序号；相同 revision 且同一时间精度内的
   并发元数据变化无法证明绝对先后。成员变化和新会话重新 GET，不能宣称分布式线性一致。
2. 60 秒只限制客户端观察，不限制或取消后台更新。达到上限显示停止自动刷新，保留手动只读刷新。
3. 下一里程碑开放长事务前，应先确认服务端任务身份、幂等性、取消边界以及 persisted/runtime_synced
   语义；不能把空进度当完成，也不能在响应丢失后重发更新。
4. 私有订阅编辑另做威胁模型和字段最小化，本轮不能作为允许私有资料进入 JS 的依据。
5. 下一阶段验收需有受控 URL 订阅，验证活动进度、失败保留上一版 Provider、304、并发变化与后台恢复。

完成本轮后停止，不自动开始 M4.3.2。
