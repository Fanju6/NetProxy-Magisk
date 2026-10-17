# HTTP API

模块 HTTP API 管理持久配置、Catalog、订阅和服务生命周期。它与 CLI 使用同一套 Go 业务和事务，核心停止时仍可读取、编辑和修复配置。

sing-box Service API 位于 9090，负责核心运行时；模块 HTTP API 默认位于 9091，两者的凭据与生命周期独立。

## 连接

官方 Android 管理器会自动连接，无需申请 Root 或复制凭据。以下命令用于开发者接入和独立 CLI 排查。

如果主动修改了接口地址后又清除管理器数据，点击仪表盘的连接失败卡片，填写实际本机 HTTPS 地址即可重新连接，不需要填写凭据。

在手机终端执行：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl api status'
su -c '/data/adb/modules/netproxy/netproxyctl api access'
```

`api access` 的 `data.address` 是 HTTPS 地址，`data.secret` 是私有凭据，`data.certificate_sha256` 是服务证书 SHA-256。客户端必须校验证书指纹，再通过 `Authorization: Bearer <secret>` 鉴权，不将凭据放入网址。

官方管理器与模块就绪探测将安装证书作为固定本机身份，精确校验证书和主机名，不依赖证书日期或设备时钟。更新时保留已有身份，不需要周期性重新配对；第三方客户端同样须校验已取得的指纹，不能直接接受任意自签名证书。

`service` 与 `status` 事件中的 `core_started_at_millis` 是已验证核心身份；未确认时为 `0`。合并实时流量时须与 `traffic.started_at_millis` 一致，不能把旧核心的流量带到新实例。

安装器会初始化 `config/api.json`。手动部署程序后，可执行：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl api init'
su -c '/data/adb/modules/netproxy/netproxyctl api start'
```

`init` 不覆盖已有或损坏配置；`start` 检查实际 HTTP 就绪状态，不只检查进程 PID。代理服务停止不会关闭这个接口。

监听端口临时被占用或 API 配置暂时无法读取时，Worker 会在 1–30 秒内退避重试；修复问题后无需重启代理核心。

电脑管理已连接手机时，在电脑终端执行：

```sh
adb forward tcp:9091 tcp:9091
```

随后请求电脑的 `https://127.0.0.1:9091/api/v1`，仍需校验设备服务证书。这只转发管理接口，不启用电脑本机透明代理。

## 基础请求

下面展示请求格式，执行环境须事先建立服务证书信任；非浏览器客户端应固定校验已取得的指纹，而不是关闭证书验证。`secret` 来自刚才的 Root 读取。浏览器不允许无关网页跨域控制模块。

```js
const base = 'https://127.0.0.1:9091/api/v1'
const secret = '<仅在本地使用的凭据>'

async function request(path, method = 'GET', body) {
  const response = await fetch(base + path, {
    method,
    headers: {
      Authorization: 'Bearer ' + secret,
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' })
    },
    ...(body === undefined ? {} : { body: JSON.stringify(body) })
  })
  const result = await response.json()
  if (result.schema !== 1) throw new Error('接口版本不匹配')
  if (!response.ok || !result.ok) {
    throw Object.assign(new Error(result.message), { code: result.code, data: result.data })
  }
  return result.data
}

const service = await request('/service')
await request('/selection', 'PUT', { group_id: 'default', node_tag: '' })
```

空 `node_tag` 使用同组 Auto。手动选择传原始节点标签，不把节点标签拼到 URL 路径中。

## 配置保存

先读取目标的文本与 revision，再保存同一目标：

```js
const target = encodeURIComponent('module/auto_start')
const current = await request('/config?target=' + target)

await request('/config?target=' + target, 'PUT', {
  revision: current.revision,
  content: JSON.stringify({ auto_start: true })
})
```

HTTP 保存必须提供读取时的 revision。发生 `config.conflict` 时重新读取并让用户处理冲突，不用旧草稿自动重试。候选内容遵循 CLI 的分区格式，例如 `inbound/app` 使用 `{"app": {...}}`。

完整配置损坏时，读取 `module`、`inbound` 或 `singbox/config.json` 的原文和 revision，再提交完整有效内容；不能通过单个分区修复无法解析的整份 JSON。运行时配置保持只读。

## 接口列表

以下路径均相对 `/api/v1`。

| 方法与路径 | 用途 |
| --- | --- |
| `GET /info` | 模块版本、API 版本、后台实例身份与 Go 堆/栈内存字节数 |
| `GET /service` | 服务状态、实际后端、模式与当前节点 |
| `POST /service/start`、`stop`、`restart`、`reload`、`check` | 对应服务操作，无正文或传空对象 |
| `GET /catalog` | 持久分组摘要 |
| `GET /nodes?group_id=...` | 分组节点 |
| `GET /nodes/snapshot?group_id=...` | 持久节点与运行时快照 |
| `GET /nodes/get?group_id=...&node_tag=...` | 节点原生配置 |
| `GET /nodes/export?group_id=...&node_tag=...` | 节点分享内容 |
| `POST /nodes` | 添加节点，正文含 `group_id/content/allow_insecure` |
| `PUT /nodes` | 编辑节点，另含原节点 `node_tag` |
| `DELETE /nodes?group_id=...&node_tag=...` | 删除节点 |
| `POST /nodes/import` | 导入文本，正文含 `content/allow_insecure` |
| `POST /nodes/delay` | 测速，正文含 `group_id/node_tag`；空 tag 测整个分组 |
| `GET/PUT /selection` | 查询或保存节点选择 |
| `GET/PUT /mode` | 查询或保存内核原生模式；正文为 `{"mode":"Rule"}` 等 |
| `GET/POST /subscriptions` | 本地与订阅分组列表，或添加 URL 订阅 |
| `GET/PUT/DELETE /subscriptions/{id}` | 查询分组详情，或编辑、删除 URL 订阅；`?private=true` 仅读取 URL 订阅私有设置 |
| `POST /subscriptions/{id}/update` | 更新一个订阅 |
| `POST /subscriptions/update-all` | 更新全部订阅 |
| `GET /subscriptions/{id}/history`、`progress` | 历史与当前进度 |
| `POST /subscriptions/{id}/cancel` | 请求取消下载 |
| `GET /configs` | 配置目标与可编辑状态 |
| `GET/PUT /config?target=...` | 读取、保存配置 |
| `POST /config/validate?target=...` | 仅校验候选，不保存 |
| `GET /network/wifi-list` | 已保存的 Wi-Fi 名称 |
| `GET /app-policy` | 当前应用策略；修改使用 `inbound/app` 配置目标 |
| `GET /android/apps` | 一次读取全部 Android 用户与应用，返回 `users` 数组；每个用户包含 `id/name/revision/apps`，应用包含 UID、名称、版本与 `icon_key` |
| `GET /android/icons/{icon_key}` | 读取已准备的 128px PNG 图标；`icon_key` 区分用户与资源版本，不再启动平台查询 |
| `POST /diagnostics/ebpf` | 能力预检，正文含 `mode/raw` |
| `GET /logs?kind=service&lines=200` | 脱敏日志，类型为 `service/core`，最多 2000 行 |
| `DELETE /logs?kind=service` | 清空日志 |
| `GET /diagnostics/export` | 下载完整脱敏诊断包 |
| `GET /events?topics=status,groups,subscriptions` | 实时事件流 |

订阅添加与编辑支持 `name/url/user_agent/hwid/custom_headers/auto_update/update_interval/update_via_proxy/include/exclude/allow_insecure/download_timeout`。`update_interval` 与 `download_timeout` 使用秒；下载方式为 `auto/always/never`，分别对应自动、代理、直连。编辑时省略字段保留原值，清空请求头使用空对象，而不是 null。

`subscriptions` 事件与列表接口返回同一组摘要，包含“本地配置”和活动分组状态；更新订阅操作只处理 URL 订阅。

`POST /subscriptions/update-all` 按 Catalog 顺序执行。部分失败仍返回原始错误码和 HTTP 状态，错误 `data` 同时包含 `updated`、`failed` 两个分组 ID 数组及 `first_error`（首个错误原始数据）；首个错误的 `persisted` 若存在也保留在顶层。不能把部分失败当作全部成功，也不能据此推断每个失败组均未保存。单组的当前持久结果继续从公开详情与历史读取。

接口不接受手机的输入或输出文件路径。节点导入传文件内容；诊断导出直接下载，不能指定设备写入位置。

`POST /auth/provision` 是唯一无 Bearer 的引导入口，只允许无 Origin 的空请求或空对象。它不会返回凭据，只会验证用户 0 中官方管理器的签名并向其私有接收组件交付连接资料；其他应用不能借此取得管理权限。

成功交付后冷却 30 秒；连续失败逐步退避，最长 30 秒。窗口内再次请求会返回 `409`，不会重复启动平台查询或延长窗口。短时间内清除管理器数据后，可等待窗口结束再重新连接。

应用名称和图标由模块按需运行的小型 Java 辅助程序调用 Android 框架生成，一次查询全部用户，保留每个用户独立的名称与 UID。图标固定为 128px，客户端按显示尺寸缩放，并使用清单中的 `icon_key` 读取 PNG。应用更新、系统语言或资源配置变化会更新图标键。辅助程序不常驻；模块后台启动不会主动拉起管理器，连接资料仅按客户端请求交付。

## 实时订阅

事件采用 SSE，使用带 Authorization 的 `fetch` 读取 `response.body`，不要为 EventSource 把 secret 加到 URL。

- `status`：模块服务状态。
- `traffic`：核心实时状态及 CPU 计数，属于 `status` 主题；`available=false` 表示核心不可用。
- `groups`：节点快照与实际选择。
- `subscriptions`：订阅状态与更新进度。

连接后先发快照，再发送变化。核心重载后重新订阅新实例；关闭核心不关闭模块事件入口。断线重新获取完整快照，不依赖历史事件回放。

页面进入后台时取消订阅，恢复前台再连接。官方管理器保留 2 秒切页宽限，会话内不因移除主题重连；真正停止后清空快照。没有订阅者时模块停止文件监听和核心高频订阅。慢客户端会断开，避免无限堆积数据。

文件变化按主题合并刷新；文件事件溢出时重新校准快照，监听失效时断开连接，客户端重新连接后获取完整状态。

核心节点变化在固定 250 毫秒窗口内合并为最新快照；实际模式由核心原生模式事件更新。重载前的旧实例事件不会覆盖新实例状态。

## 错误与限制

JSON 响应使用与 CLI 一致的 `schema=1`、`ok/code/message/data`。同时检查 HTTP 状态与 `ok`，不要仅按中文文案判断错误。

接口仅支持列表中声明的方法，`HEAD` 返回 `405`，不会执行资源读取或创建事件订阅。

- `400`：请求格式或参数错误。
- `401/403`：凭据、Host 或 Origin 不允许。
- `404`：接口或目标不存在。
- `409`：revision 冲突或操作忙。
- `413`：请求正文超过上限。
- `422`：已识别的配置或参数校验错误。
- `502`：保存成功后运行时同步失败。
- `504`：操作超时。
- `500`：其他内部错误。

错误带有 `persisted=true` 时，数据已保存，不能当作未保存自动重试。客户端断线不会取消已接受的写入；重新连接后读取真实状态。Worker 关停时会取消正在进行的订阅网络下载，保留上一份 Provider；已经下载完成并进入提交的事务仍完成收尾。这个同步接口不提供持久任务队列或 exactly-once 保证。

官方管理器仅对普通只读 GET 在响应前的连接故障最多恢复一次，并沿用原调用总时限。取消、超时、TLS 验证失败和正文读取错误不重试，写操作不自动重发。重新连接失败会保留原本可用的连接资料，不覆盖新收到的可信交付。

JSON 正文统一最大 4 MiB，包含节点导入；按序拆分较大导入文件后分别提交。同时最多 8 个事件客户端。耗时操作有独立时限和并发限制，不阻塞普通状态读取。下载诊断包时检查传输是否完整，不把中断的文件当作有效包。

`config/api.json` 仅允许 loopback IP，文件权限为 `0600`，secret 为随机 32 字节的十六进制编码。凭据不进入日志、诊断包或遥测；不要粘贴到聊天、网址、公开脚本或 Git 仓库。
