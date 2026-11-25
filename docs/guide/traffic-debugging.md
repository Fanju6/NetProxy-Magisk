# 抓包与脚本重写

全局透明抓包用于调试你有权检查的流量，默认关闭。开启后处理进入受管 TUN 或 eBPF 入站的 TCP 流量；无需配置客户端代理、端口或凭据。sing-box 使用原连接的入站、应用/UID、来源 IP 等元数据匹配既有规则并选择出站，不重新路由。

## 启停与查看

在 Android 管理器打开 **设置 → 抓包与重写**。开启 **全局抓包** 后，产生流量并打开 **抓包记录**；右上角可刷新或清空，也可按 URL、方法、状态码和 TLS 筛选。点开记录可切换 **概览 / 请求 / 响应**，并复制内容或生成 cURL。**重写规则** 和 **HTTPS 调试** 分别是独立页面；规则保存时由模块校验正则和脚本。

在模块 WebUI 点击工具栏的 **全局抓包** 按钮，或在终端输入：

```text
api traffic toggle
api traffic status
api traffic list
api traffic stop
```

在设备 Shell 中，为同样的命令添加前缀，例如 `su -c '/data/adb/modules/netproxy/netproxyctl api traffic toggle'`。

1. `toggle` 切换开关，`start` 和 `stop` 分别显式开启和关闭。设置会保存；运行中的核心重载一次，可能中断已有连接。尚未启动核心时，设置在下一次启动时生效。
2. `list` 查看至多 200 条摘要：ID、时间、方法、目标、选定出站、状态和重写结果。终端 WebUI 输入 `api traffic detail <ID>` 查看详情，或执行 `su -c '/data/adb/modules/netproxy/netproxyctl api traffic detail <ID>'`（将 `<ID>` 替换为实际 ID）。普通 HTTP 与解密后的 HTTPS 记录完整 URL（含路径和查询）、原始请求头及转发时读取的请求正文、重写后发送的响应头和正文；未解密的 TLS 仍只有连接信息。
3. `stop` 保留列表以便查看；`clear` 可清空记录，再次开启也会清空。Worker 不可用或抓包连接达到上限时，新流量按原有路由透传。

详情只保存在 Worker 内存中，预算为 8 MiB，超限淘汰最旧记录。请求和响应各保留最多 64 KiB 正文及约 8 KiB 头字段；达到上限时标注截断，提前拒绝或未完整转发的正文也可能不完整。UTF-8 文本直接显示；二进制或仍经 gzip、Brotli 等编码的正文显示 Base64 和原编码。记录不写入日志、诊断包或磁盘。URL、头和正文可能包含敏感数据；仅对有权检查的流量启用抓包，分享前先检查，并在调试结束后清空。

**HTTPS 默认只记录 TCP 元数据**。只重写 HTTP/1.x，每个被重写的连接处理一个请求，客户端可正常重连。该功能不是 PCAP 导出，不采集未被受管入站接管的流量、UDP 或 QUIC。

## 按主机启用 HTTPS 解密

在管理器的 **HTTPS 调试** 区域编辑主机名单（每行一项），开启解密并导出公开调试 CA。导出通过 Android 文件选择器保存，可再从文件管理器分享；私钥始终留在模块内。需要系统级信任时，阅读确认提示后启用 **Android 系统信任** 并重启设备。若模块版本不支持 HTTPS 调试，管理器仍可使用抓包和重写。

**仅调试你有权检查的流量。** 解密后的请求和响应在内存中处理，重写脚本也可访问其内容。系统级信任会扩大该 CA 的信任范围；优先只在测试客户端信任它。不要复制或分享模块的 CA 私钥。

1. 在 WebUI 中运行 `api traffic ca-init` 初始化每次安装独立的 CA（不覆盖已有 CA）。`api traffic ca` 查看公开 PEM、SHA-256 指纹和 Android 的 `<hash>.0` 文件名；`api traffic ca-export /data/local/tmp/netproxy-debug.pem` 导出公开证书。这些命令都不导出私钥。
2. 把下面的 JSON 保存到设备上，例如 `/data/local/tmp/netproxy-mitm.json`，用 `api traffic set-mitm /data/local/tmp/netproxy-mitm.json` 应用，用 `api traffic mitm` 查看配置。启用解密也会初始化 CA。全局抓包开关仍须开启。

```json
{
  "enabled": true,
  "hosts": ["example.org", "*.test.example.org"],
  "system_trust": false
}
```

支持精确主机名、IP、`*.域名`（仅子域名）和 `*`（不建议）。优先匹配 TLS 握手中的服务器名，无服务器名时匹配原目标地址。空列表不解密；不匹配的流量原样透传。匹配的 HTTPS 请求以 `https://` 完整 URL 匹配重写规则，`list` 显示 `decrypted` 请求元数据，不保存正文、请求头或查询参数。上游 TLS 仍校验证书。

如果客户端因证书钉扎等原因拒绝调试证书，当前连接无法恢复，记录为 `handshake_failed`，同一主机的新 TCP 连接在约 10 分钟内透传（`bypass`）。停止抓包、修改解密配置或重启 Worker 会清除临时绕行状态。HTTP/2-only 流量透传；不支持对双向 TLS 或 HTTPS 协议升级进行解密。

### 可选：Android 系统信任

如果测试客户端无法单独配置 CA，可将 `system_trust` 改为 `true`，保存并重启。模块开机时保留已有证书并绑定 `/system/etc/security/cacerts` 以及设备上存在的 `/apex/com.android.conscrypt/cacerts`（Android 14+）。某些系统或挂载隔离配置可能禁止挂载；请查看服务日志，已运行的测试应用也可能需要重启。系统信任不绕过应用证书钉扎。

调试后保存 `{"enabled":false,"hosts":[],"system_trust":false}`，停止抓包并在测试客户端删除手工信任的证书。关闭系统信任或卸载模块时会尝试卸载模块的证书目录；如未完成，重启设备。停止抓包本身不会删除系统信任。

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
