# Android 只读 Transport 接入约定

本页只描述 `apps/web` 的桥接实现，不代表 Tauri、APK、TLS、Provider 或真机升级已经验收。原生工程和凭据边界由父任务集成验证。

## 构建与能力

- `pnpm build` → `dist/`：不包含 Mock 或原生 IPC，缺少可信 Transport 时拒绝连接。
- `pnpm build:demo` → `dist/`：显式 Mock，可操作模拟数据。
- `pnpm build:native` → `dist-native/`：不包含 Mock，仅在官方 `isTauri()` 返回真时创建 Native Transport。在普通浏览器打开仍拒绝连接。
- `pnpm exec vite --mode native` 可用于 Tauri 开发，不能回退到 Mock。父工程应将 `frontendDist` 指向 `../web/dist-native`，具体相对路径以壳工程位置为准。
- `ApiTransport.readOnly` 是独立能力，不从平台名称推断。Native 和 Unavailable 为真，Mock 为假。`ApiClient` 在发送前拒绝写入；服务、模式和节点选择控件禁用；配置不读取。节点和订阅只消费 SSE 完整快照。
- 普通 Web CSP 不变。只有 native mode 的 HTML 增加 `ipc:` 和 `http://ipc.localhost`，不允许前端直连 Go HTTPS 或任意 URL；父工程 Tauri CSP 仍须单独限制。

## IPC 协议

全部通过官方 `@tauri-apps/api@2.12.4` 的 `invoke` 和 `addPluginListener`，业务页面不直接调用 IPC。

| 命令                              | 参数                                           | 返回                   |
| --------------------------------- | ---------------------------------------------- | ---------------------- |
| `plugin:module-api\|read`         | `{ requestId, resource: "info" \| "service" }` | `{ status, body }`     |
| `plugin:module-api\|cancel`       | `{ requestId }`                                | 无数据                 |
| `plugin:module-api\|open_events`  | `{ subscriptionId, topics }`                   | 订阅建立后完成         |
| `plugin:module-api\|next_event`   | `{ subscriptionId }`                           | `{ name, body }`       |
| `plugin:module-api\|close_events` | `{ subscriptionId }`                           | 无数据                 |
| `plugin:module-api\|visibility`   | `{}`                                           | `{ visible: boolean }` |

`topics` 是 `status/groups/subscriptions` 的非空子集，`traffic` 仅随 status 接收。HTTP 相对路径只允许 GET `/info`、`/service`，不接受 query/body；API 前缀 `/api/v1` 由原生层固定，JS 不传 URL。Rust 拒绝返回 `{ code, status, message }`；前端保留合法错误码和状态，但不展示原始异常消息。HTTP 响应继续由 `ApiClient` 校验 `schema=1`、HTTP 状态与 Zod 数据类型，不把 IPC 成功当业务成功。

原生层必须在传入 JS 前限制正文和帧大小、移除凭据、校验 TLS 身份，并用自己的白名单与 Capability 拒绝写入。前端也限制桥接帧为 4 MiB，但这不能替代原生读取限制。

本轮原生 PoC 禁止未验证的 TLS 配对引导，`allowProvisioning=false`。只能使用同包升级保留或模块私有 Provider 重新交付的资料；全新安装没有凭据时返回 `api.connection_unavailable`，界面提示缺少可信资料且不会自动配对，不要求粘贴 Secret，也不声称已经连接。

## 订阅及取消

- JS 同时只等待一个 `next_event`，这是原生队列的异步拉取，不是 HTTP 轮询。JS 无事件堆积队列，不另建重连循环。
- Kotlin 负责有界队列和背压、关闭时拒绝 pending next、取消 OkHttp Call，以及 Activity/WebView 销毁清理。Rust 限制 IPC 并传递结构化结果。
- `subscribe()` 立即返回幂等释放函数；重复订阅关闭旧 ID。旧 ID 的响应不会发布或关闭新订阅。
- 若取消发生在 open 完成之前，先发送 close 取消建立；open 后来仍成功时，再按旧 ID 做一次补偿 close。原生 close 必须幂等，关闭旧 ID 不得影响新 ID。open 失败或普通 pending next 取消不重复关闭。
- GET 用唯一 requestId 取消，仅匹配本次请求；已经完成或调用前取消的请求不发送额外 cancel。不重发任何写入。

## 前后台

原生监听先注册 `addPluginListener("module-api", "visibility", ...)`，再查询初值。若查询等待期间收到更新，迟到的初值或查询失败不能覆盖新事件。初值未知时不建立 SSE；监听失败时暂停。

`ModuleEvents` 只有在 document 与原生均可见时建立订阅。重复的初值回调不重复建连；后台关闭订阅，恢复前台等待新的 status/groups/subscriptions 完整快照。原有核心启动身份隔离、历史清理、会话代数和三次有界重连继续生效。

官方 listener 还使用 `register_listener` 与 `remove_listener` 命令，父工程必须包含对应插件注册和权限。取消时初始 listener Promise 尚未完成，也须回收迟到的 listener。

## 验收边界

Vitest 验证可注入 IPC、官方 API 包的 mock 接线、白名单、错误脱敏、取消竞争、单请求事件拉取、可见性及只读 UI。Playwright 验证 Web/演示/原生构建隔离，原生构建在普通浏览器拒绝连接。这些不等同真实证书、Root、Android WebView 或 APK 覆盖升级验证。

官方依据：[Core JavaScript API](https://v2.tauri.app/reference/javascript/api/namespacecore/)、[移动插件开发](https://v2.tauri.app/develop/plugins/develop-mobile/)。
