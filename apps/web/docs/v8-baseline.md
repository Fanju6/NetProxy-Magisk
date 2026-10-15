# V8 功能与 API 迁移基线

源码基线：`v8.6.1`，`3fd2a1cb9710b59f2723d3c0bb2a01c7090e0998`。此文记录已有行为，不把 Mock 通过等同于真实设备迁移成功。

## 功能矩阵

所有 HTTP 路径前缀为 `/api/v1`；除明确说明外均需鉴权。

| V8 领域          | 已有 API / 事实源                                                                | 本批 V9                                    | 后续迁移验收                                     |
| ---------------- | -------------------------------------------------------------------------------- | ------------------------------------------ | ------------------------------------------------ |
| 模块身份与连接   | `GET /info`；Android `ModuleApiClient`、连接 Provider                            | DTO、拒绝未配置 Transport、Mock 信息       | 固定签名、Provider、TLS pin、401 及重连          |
| 服务             | `GET /service`；`POST /service/start,stop,restart,reload,check`                  | 状态、启停确认、Mock 启停                  | 真实六态、失败诊断、重载及中断恢复               |
| 实时状态         | `GET /events?topics=status,groups,subscriptions`                                 | 单条可取消会话、完整快照、启动身份门禁     | 原生 SSE 长连、前后台、心跳及背压                |
| 流量与资源       | status/traffic 事件                                                              | 速率、累计、内存、连接、会话内 30 点图     | CPU 增量、真实采样性能                           |
| 出站模式         | `GET/PUT /mode`，主配置 `default_mode`                                           | 原生模式列表与 Mock 修改                   | Wi-Fi 实际/配置模式、部分成功                    |
| Catalog 与选节点 | `GET /catalog`、`/nodes/snapshot`；`GET/PUT /selection`                          | 展开分组、网格/列表、搜索、Auto/手选、详情 | 离线大目录、运行时组及真实选择                   |
| 节点维护         | `/nodes`、`/nodes/get,export,import,delay`                                       | 不提供虚假编辑按钮                         | 内容导入导出、编辑/删除、测速                    |
| 订阅             | `/subscriptions`、`/{id}`、`/update-all`、`/{id}/update,history,progress,cancel` | 本地/URL 列表、详情、计划摘要              | URL/Header 编辑、取消、进度、失败保留旧 Provider |
| 开机自启         | `/config?target=module/auto_start`                                               | 读取 revision、Mock 保存                   | 只影响下次开机，不重载                           |
| 入站与 Root 策略 | `/config?target=inbound/*`                                                       | 设置结构中明确列为未迁移                   | 原生 eBPF/TUN、独立 Root、分区事务               |
| 应用代理         | `/app-policy`、`/android/apps`、`/android/icons/{key}`                           | 未实现                                     | 多用户、图标、搜索、离页/后台合并提交            |
| 网络匹配         | `/config?target=module/wifi`、`/network/wifi-list`                               | 未实现                                     | 双名单、精确 SSID、实际网络策略                  |
| 路由规则         | `/configs`、`/config?target=singbox/rules/local/<tag>.json`                      | 未实现                                     | proxy/direct/block、单/多条件、revision          |
| 内核配置         | `/configs`、`/config`、`/config/validate`                                        | 未实现                                     | 分区编辑、Schema、冲突、JSON 保真                |
| 日志与诊断       | `/logs`、`/logs/export`、`/diagnostics/ebpf`                                     | 未实现                                     | 结构化服务日志、核心文本、分享和脱敏             |
| 主题与语言       | Android 原生资源/外观偏好                                                        | 明/暗/系统、中英、仅外观 localStorage      | 俄语、系统变化、真实 WebView 无障碍              |
| Android 平台能力 | Tile、文件分享/导出、返回键、安装身份                                            | 未实现                                     | Tauri 原生 PoC 单独核验，不视为浏览器功能        |

## 关键契约

### Envelope 与错误

`{schema:1, ok, code, message, data}`。同时检查 HTTP 状态和 `ok`；schema 不符、数据形状错误、非 JSON 响应必须失败。`message` 不用于机器分支；未知字段允许透传，不能猜测缺失的关键字段。

| 场景               | 当前后端语义                                         | 前端要求                                       |
| ------------------ | ---------------------------------------------------- | ---------------------------------------------- |
| 非法/未知请求参数  | `usage.invalid`，400                                 | 修正请求，不重发写操作                         |
| 超过 4 MiB         | `api.body_too_large`，413                            | 按 UTF-8 字节检查；最终仍由后端强制            |
| 身份失败           | `api.unauthorized`，401                              | 原生层恢复可信凭据，不回退 Root/明文           |
| 配置冲突           | `config.conflict`，409                               | 保留草稿，明确重新读取，不借新 revision 重试   |
| 已保存但运行时失败 | `node.runtime_sync_failed` 等，`data.persisted=true` | 区分“保存失败”和“运行时应用失败”，读取权威状态 |
| 无专用契约错误     | `command.failed`，通常 500                           | 不捏造更细的错误码；不输出原始敏感异常         |

Mock 的错误码来自当前源码；它不是完整后端校验器。例如并发文件锁、回滚失败和所有原生配置错误只能由 Go 回归测试与后续集成验证。

### 读取与写入

- `GET /service` 的 data 是状态本身；启停结果为 `{action,status}`。
- 服务状态仅为 `stopped/preparing/starting/ready/stopping/failed`。`active_backend`、`pid` 可为 null。不能从配置后端推断正在运行的后端。
- `/nodes/snapshot` 包含 `groups:[{group,nodes}]`、`selection`、可选 `runtime_groups`。持久节点始终来自 Catalog，不能因为服务停止而隐藏。
- `PUT /selection` 请求 `{group_id,node_tag}`，空 tag 代表同组 Auto；响应仅为 `{group_id,mode,selected}`，不得当完整快照发布。tag 本身可以含 `/`。
- `/subscriptions` 包含本地组；空组没有可选择的 Auto。URL 设置的写入口与本地组读取必须区分。
- `/config` 的 content 为 **JSON 字符串**，不是 JSON 对象；携带读取时的 opaque revision。成功码为 `config.apply`，返回 `{target,revision}`。
- 读取可取消。已接受写入的结果可能因断线而未知，客户端不盲目重试；先重新读取确认。Query mutation 不因页面销毁而自动重放。

### 事件

- SSE 先给完整主题快照，才可消费后续状态；不是 JSON Patch。流量属于 status 主题。
- `traffic.available=false` 时清空流量；可用帧的 `started_at_millis` 必须匹配本会话收到的 ready 状态的 `core_started_at_millis`。
- 断线、后台、重新连接清空旧流量和历史；generation 丢弃旧连接回调。事件快照发布前取消旧读取，避免慢 GET 覆盖新事件。
- 当前没有每帧 Worker instance_id；原生 Transport 必须关闭旧 Call 并隔离回调。不能把 Mock 的会话隔离当作已验证真实 SSE。
- 本批固定订阅三个已使用主题，前台共用一条流；失败明确提示并由用户重新连接，不使用轮询兜底。动态主题和自动退避留待真实 Transport 实现。

### 安全边界

当前 API 是每安装独立 TLS 证书 + 随机 Bearer Secret，仅监听 loopback，并校验 Host/Origin。浏览器不能自然信任该证书，也不能调用 Root 私有 Provider。普通前端页面没有真实连接能力是有意的安全默认。

Secret、证书私钥、订阅 URL/Header、完整配置不能进入前端构建、localStorage、错误日志或截图。只有主题和语言保存在浏览器。所有演示地址均为 `.example.invalid`，不提供真实代理能力。

## 核对源码

- `src/android/ARCHITECTURE.md` 与 `src/android/app/src/main/java/` 各 feature、core/data。
- `src/native/netproxy/internal/httpapi/{server,resources,events,events_core}.go`：路由、认证与事件。
- `src/native/netproxy/internal/service/control.go`、`internal/catalog/summary.go`、`internal/module/`：DTO、选择与事务。
- `docs/config/http-api.md`：公开 API；实际源码与测试优先于未实现草案。
- `src/webui/src/exec.ts`、`src/webui/package.json`：V8 终端 Root 桥接与产物目录。
- `.github/workflows/{verify,build-module,ci,release}.yml`、`tests/verify.sh`：V8 验证及发布边界。

这些是仓库相对路径；V9 本批没有移动或替换以上实现。
