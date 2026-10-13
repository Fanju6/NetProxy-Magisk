# HTTP API

模块 HTTP API 管理持久配置、Catalog、订阅和服务生命周期。它与 CLI 使用同一套 Go 业务和事务，核心停止时仍可读取、编辑和修复配置。

sing-box Service API 位于 9090，负责核心运行时；模块 HTTP API 默认位于 9091，两者的凭据与生命周期独立。

## 连接

在手机终端执行：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl api status'
su -c '/data/adb/modules/netproxy/netproxyctl api access'
```

`api access` 的 `data.address` 是地址，`data.secret` 是私有凭据。请求通过 `Authorization: Bearer <secret>` 鉴权，不将凭据放入网址。

安装器会初始化 `config/api.json`。手动部署程序后，可执行：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl api init'
su -c '/data/adb/modules/netproxy/netproxyctl api start'
```

`init` 不覆盖已有或损坏配置；`start` 检查实际 HTTP 就绪状态，不只检查进程 PID。代理服务停止不会关闭这个接口。

电脑管理已连接手机时，在电脑终端执行：

```sh
adb forward tcp:9091 tcp:9091
```

随后请求电脑的 `http://127.0.0.1:9091/api/v1`。这只转发管理接口，不启用电脑本机透明代理。

## 基础请求

下面的 JavaScript 可用于同源客户端或非浏览器客户端，`secret` 来自刚才的 Root 读取。浏览器不允许无关网页跨域控制模块。

```js
const base = 'http://127.0.0.1:9091/api/v1'
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
| `GET /info` | 模块版本、API 版本和后台实例身份 |
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
| `GET/POST /subscriptions` | 订阅列表或添加订阅 |
| `GET/PUT/DELETE /subscriptions/{id}` | 查询、编辑或删除订阅；`?private=true` 读取私有设置 |
| `POST /subscriptions/{id}/update` | 更新一个订阅 |
| `POST /subscriptions/update-all` | 更新全部订阅 |
| `GET /subscriptions/{id}/history`、`progress` | 历史与当前进度 |
| `POST /subscriptions/{id}/cancel` | 请求取消下载 |
| `GET /configs` | 配置目标与可编辑状态 |
| `GET/PUT /config?target=...` | 读取、保存配置 |
| `POST /config/validate?target=...` | 仅校验候选，不保存 |
| `GET /network/wifi-list` | 已保存的 Wi-Fi 名称 |
| `GET /app-policy` | 当前应用策略；修改使用 `inbound/app` 配置目标 |
| `POST /diagnostics/ebpf` | 能力预检，正文含 `mode/raw` |
| `GET /logs?kind=service&lines=200` | 脱敏日志，类型为 `service/core`，最多 2000 行 |
| `DELETE /logs?kind=service` | 清空日志 |
| `GET /diagnostics/export` | 下载完整脱敏诊断包 |
| `GET /events?topics=status,groups,subscriptions` | 实时事件流 |

订阅添加与编辑支持 `name/url/user_agent/hwid/custom_headers/auto_update/update_interval/update_via_proxy/include/exclude/allow_insecure/download_timeout`。`update_interval` 与 `download_timeout` 使用秒；下载方式为 `auto/always/never`，分别对应自动、代理、直连。编辑时省略字段保留原值，清空请求头使用空对象，而不是 null。

接口不接受手机的输入或输出文件路径。节点导入传文件内容；诊断导出直接下载，不能指定设备写入位置。

## 实时订阅

事件采用 SSE，使用带 Authorization 的 `fetch` 读取 `response.body`，不要为 EventSource 把 secret 加到 URL。

- `status`：模块服务状态。
- `traffic`：核心实时状态，属于 `status` 主题；`available=false` 表示核心不可用。
- `groups`：节点快照与实际选择。
- `subscriptions`：订阅状态与更新进度。

连接后先发快照，再发送变化。核心重载后重新订阅新实例；关闭核心不关闭模块事件入口。断线重新获取完整快照，不依赖历史事件回放。

页面进入后台时取消订阅，恢复前台再连接。没有订阅者时停止文件监听和核心高频订阅。慢客户端会断开，避免无限堆积数据。

文件变化按主题合并刷新；文件事件溢出时重新校准快照，监听失效时断开连接，客户端重新连接后获取完整状态。

## 错误与限制

JSON 响应使用与 CLI 一致的 `schema=1`、`ok/code/message/data`。同时检查 HTTP 状态与 `ok`，不要仅按中文文案判断错误。

接口仅支持列表中声明的方法，`HEAD` 返回 `405`，不会执行资源读取或创建事件订阅。

- `400`：请求格式或参数错误。
- `401/403`：凭据、Host 或 Origin 不允许。
- `404`：接口或目标不存在。
- `409`：revision 冲突或操作忙。
- `422`：已识别的配置或参数校验错误。
- `502`：保存成功后运行时同步失败。
- `504`：操作超时。
- `500`：其他内部错误。

错误带有 `persisted=true` 时，数据已保存，不能当作未保存自动重试。客户端断线不会取消已接受的写入；重新连接后读取真实状态。这个同步接口不提供持久任务队列或 exactly-once 保证。

普通 JSON 正文最大 8 MiB，导入正文最大 32 MiB，同时最多 8 个事件客户端。耗时操作有独立时限和并发限制，不阻塞普通状态读取。下载诊断包时检查传输是否完整，不把中断的文件当作有效包。

`config/api.json` 仅允许 loopback IP，文件权限为 `0600`，secret 为随机 32 字节的十六进制编码。凭据不进入日志、诊断包或遥测；不要粘贴到聊天、网址、公开脚本或 Git 仓库。
