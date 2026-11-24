# 抓包与脚本重写

全局透明抓包用于调试你有权检查的流量，默认关闭。开启后处理进入受管 TUN 或 eBPF 入站的 TCP 流量；无需配置客户端代理、端口或凭据。sing-box 使用原连接的入站、应用/UID、来源 IP 等元数据匹配既有规则并选择出站，不重新路由。

## 启停与查看

在模块 WebUI 点击工具栏的 **全局抓包** 按钮，或在终端输入：

```text
api traffic toggle
api traffic status
api traffic list
api traffic stop
```

在设备 Shell 中，为同样的命令添加前缀，例如 `su -c '/data/adb/modules/netproxy/netproxyctl api traffic toggle'`。

1. `toggle` 切换开关，`start` 和 `stop` 分别显式开启和关闭。设置会保存；运行中的核心重载一次，可能中断已有连接。尚未启动核心时，设置在下一次启动时生效。
2. `list` 查看最近 200 条记录：时间、方法、目标、选定出站、状态和重写结果。不会记录路径、查询参数、请求头或正文，也不会写入日志、诊断包或磁盘。
3. `stop` 保留列表以便查看；`clear` 可清空记录，再次开启也会清空。Worker 不可用或抓包连接达到上限时，新流量按原有路由透传。

**HTTPS 默认只记录 TCP 元数据**；该分支不解密 TLS。只重写 HTTP/1.x，每个被重写的连接处理一个请求，客户端可正常重连。该功能不是 PCAP 导出，不采集未被受管入站接管的流量、UDP 或 QUIC。

## 编写规则

将以下文件保存到设备上，例如 `/data/local/tmp/netproxy-rewrite.json`。在 WebUI 输入 `api traffic set-rules /data/local/tmp/netproxy-rewrite.json` 应用，或使用 Root CLI 执行同一操作。不需要重启 Worker 或核心。用 `api traffic rules` 查看，应用 `{"rules":[]}` 删除全部规则。

```json
{
  "rules": [
    {
      "phase": "request",
      "pattern": "^http://example[.]org/",
      "script": "return {set_headers: {'X-Debug': 'NetProxy'}};"
    },
    {
      "phase": "response",
      "pattern": "^http://example[.]org/",
      "script": "if (typeof input.body === 'string') return {body: input.body.replace('Example Domain', 'Debug Domain')};"
    }
  ]
}
```

- 按规则顺序，每个阶段仅执行第一条匹配完整 URL 的 Go 正则规则。脚本是函数体，不需要声明函数。
- `input.url`、`input.method` 两个阶段均可用；响应阶段额外提供 `input.status`。在正文可读取时还提供 `input.body`：最大 256 KiB 的 UTF-8 文本或 JSON；不可读取时不提供该字段。
- 两个阶段均可返回 `set_headers` 对象和 `remove_headers` 数组。连接级头、代理认证头和正文长度等由代理管理。
- 请求阶段还可返回 `{redirect: "https://example.org/new"}`（给客户端返回 302）或 `{reject: true}`（403）。响应阶段可返回 `{body: "..."}` 替换可读取的正文。返回空值表示不改写。
- 规则只应使用你信任的本地脚本。JavaScript 不提供文件、网络或命令执行 API。脚本执行错误、超时或返回格式无效时，忽略该阶段的修改。保存时语法或格式无效会保留原来的规则。
