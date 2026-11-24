# 抓包与脚本重写

抓包代理用于调试你有权检查的流量。默认关闭；只接收显式配置了代理地址和凭据的本机客户端，不会改变 eBPF、TUN 或全局代理设置。转发仍经过 sing-box 的路由策略。

## 启停与查看

先启动 sing-box。抓包代理需要生效配置中标签为 `mixed-in` 的本机免认证 `mixed` 入站（出厂配置为 `127.0.0.1:7080`）。

在终端式模块 WebUI 输入：

```text
api traffic start
api traffic access
api traffic list
api traffic stop
```

在设备 Shell 中，为同样的命令添加前缀，例如 `su -c '/data/adb/modules/netproxy/netproxyctl api traffic start'`。

1. `start` 分配一个本机端口；`access` 显示地址、用户名和本次会话的随机密码。凭据只用于抓包代理，不是模块管理 API 凭据。
2. 在同一设备上，为支持代理鉴权的调试客户端设置 HTTP 代理。在 curl 中可使用 `--proxy http://127.0.0.1:端口 --proxy-user netproxy`，在提示时输入密码。
3. `list` 查看最近 200 条记录：时间、方法、目标主机、状态、耗时和重写结果。不会记录路径、查询参数、请求头或正文，也不会写入日志、诊断包或磁盘。
4. `stop` 关闭监听和代理连接，保留本次列表以便查看；`clear` 可清空记录，下一次启动也会清空。Worker 重启后需要重新启动抓包。不要分享代理凭据。

**HTTPS 只记录 CONNECT 的目标和隧道状态**；TLS 仍由客户端与目标直接建立，不安装 CA、不解密，也不能重写 HTTPS 内容。该功能不是 PCAP 导出，不采集其他应用的透明流量、UDP、QUIC 或 WebSocket。

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
