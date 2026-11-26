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

在管理器的 **抓包应用** 中选择 **全部**、**黑名单** 或 **白名单**。黑名单中的应用不抓取；白名单仅抓取选中的应用，空白名单不抓取任何应用。切换模式不会清空名单，保存后对新连接生效。未包含在抓包范围内的应用保持原有代理路径，不记录、不解密、也不重写。**抓包记录**中的应用筛选只影响显示，不改变实际抓包范围。

终端使用 `api traffic apps` 查看模式和名单，使用 `api traffic set-apps <文件>` 保存 JSON，例如 `{"mode":"whitelist","blacklist":[],"whitelist":["0:com.example.app"]}`。条目使用 `<Android 用户 ID>:<包名>`。列表与详情在可用时返回 `source_app`（UID、Android 用户 ID 和包名）。

「抓包时禁用 QUIC」默认开启。它仅在抓包开启时拒绝所选应用经受管入站发送的 UDP 443 流量，让支持回退的应用改用 TCP。无需修改原生路由。可在设置中关闭，或用 `api traffic set-apps <文件>` 保存当前名单和 `"disable_quic":false`；省略该字段保留原值。

1. `toggle` 切换开关，`start` 和 `stop` 分别显式开启和关闭。设置会保存；运行中的核心重载一次，可能中断已有连接。尚未启动核心时，设置在下一次启动时生效。
2. `list` 查看至多 200 条摘要。使用 `api traffic page [批次ID [游标]]` 继续读取已保存的更早记录；Android 向下滚动时会加载更多并在应用私有缓存中保存已取回的历史页面。筛选和分组应用于已加载的记录。终端 WebUI 输入 `api traffic detail <ID>` 查看详情，或执行 `su -c '/data/adb/modules/netproxy/netproxyctl api traffic detail <ID>'`（将 `<ID>` 替换为实际 ID）。普通 HTTP 与解密后的 HTTPS 记录完整 URL（含路径和查询）、原始请求头及转发时读取的请求正文、重写后发送的响应头和正文；未解密的 TLS 仍只有连接信息。
3. 每次启动产生一个本地批次，停止后仍可通过 Android 抓包记录页的批次选择器查看或单独删除。列表默认显示当前或最近批次。终端使用 `api traffic batches` 查看批次，`api traffic list <批次ID>` 查看指定批次，`api traffic delete-batch <批次ID>` 删除已停止的批次，`api traffic clear` 清空所有记录。Worker 不可用或抓包连接达到上限时，新流量按原有路由透传。

会话概览显示入站、匹配的出站以及拨号时实际选定的策略组和节点链，例如 `Proxy → Auto → 节点`。传输对端是内核连接暴露的地址，可能是代理服务器，也可能是目标地址；客户端应用、UID 和进程仅在核心提供时显示。Android 可按应用、出站或是否实际重写筛选。

重写详情区分已执行、无修改、发生错误、前序规则优先和未进入响应阶段。规则仍按原顺序执行，每阶段只执行第一条匹配规则；不会因记录其余匹配而执行更多脚本。详情记录重定向、拦截、变更头字段的前后值、正文大小变化及脚本耗时，不保留脚本异常文本。Android 可从详情跳转到仍未变更的原规则。请求和响应头支持逐条复制名称、值或整条；正文默认折叠，展开后可在 JSON 格式化和原文之间切换。WebUI 中加入 `--raw` 保留机器可读 JSON。

记录和详情会异步保存在仅 root 可读的模块数据目录，重启和保留数据的升级不会清空记录。记录最多保留 32 批、合计 8192 条，记录 JSON 上限 64 MiB；含媒体文件在内的历史总容量上限为 256 MiB。超过容量时优先淘汰最旧的已停止批次。磁盘繁忙时个别记录可能仅保留在当前 Worker 内存中。

每个请求和响应各保留最多 64 KiB 文本预览及约 8 KiB 头字段。gzip、Brotli、deflate 文本预览会在同一大小上限内解压，保留原编码和传输大小。图片、视频、音频和附件等按批次存为最多 16 MiB 的独立正文文件，不进入记录 JSON 或内存环形缓冲区。Android 请求或响应详情按需下载；图片安全缩放预览，完整且兼容的视频和音频可在应用内播放。附件可以保存或分享已保留的字节；截断和服务器返回的 206 分段分别标注，不尝试拼接分段。媒体保留原始字节；压缩编码或不兼容格式可能无法预览。删除或清空批次也会删除其正文文件。WebUI 显示文件信息，历史内嵌二进制预览可切换为十六进制。记录可能含敏感 URL、头和正文，不写入日志或诊断包。仅对有权检查的流量启用抓包，分享前先检查，并在调试结束后清空。

**HTTPS 默认只记录 TCP 元数据**。只重写 HTTP/1.x，每个被重写的连接处理一个请求，客户端可正常重连。该功能不是 PCAP 导出，不采集未被受管入站接管的流量、UDP 或 QUIC。

## 按主机启用 HTTPS 解密

在管理器的 **HTTPS 调试** 区域编辑主机名单（每行一项），开启解密并导出公开调试 CA。导出通过 Android 文件选择器保存，可再从文件管理器分享；私钥始终留在模块内。需要系统级信任时，阅读确认提示后启用 **Android 系统信任**，在下次开机或热更新时应用。若模块版本不支持 HTTPS 调试，管理器仍可使用抓包和重写。

**仅调试你有权检查的流量。** 解密后的请求和响应在内存中处理，重写脚本也可访问其内容。系统级信任会扩大该 CA 的信任范围；优先只在测试客户端信任它。不要复制或分享模块的 CA 私钥。

1. 在 WebUI 中运行 `api traffic ca-init` 初始化每次安装独立的 CA（不覆盖已有 CA）。`api traffic ca` 查看公开 PEM、SHA-256 指纹和 Android 的 `<hash>.0` 文件名；`api traffic ca-export /data/local/tmp/netproxy-debug.pem` 导出公开证书。这些命令都不导出私钥。
2. 把下面的 JSON 保存到设备上，例如 `/data/local/tmp/netproxy-mitm.json`，用 `api traffic set-mitm /data/local/tmp/netproxy-mitm.json` 应用，用 `api traffic mitm` 查看配置。启用解密也会初始化 CA。全局抓包开关仍须开启。

```json
{
  "enabled": true,
  "hosts": ["*"],
  "bypass_hosts": ["*.test.example.org"],
  "system_trust": false
}
```

启用解密后，默认匹配全部 HTTPS 和 WSS 主机；仍须信任调试 CA。用 `hosts` 限制解密范围，或用 `bypass_hosts` 排除主机，排除优先。支持精确主机名、IP、`*.域名`（仅子域名）和 `*`。已有非空名单保留原有范围；空名单使用新默认值。优先匹配 TLS 握手中的服务器名，无服务器名时匹配原目标地址。上游 TLS 仍校验证书。

如果客户端拒绝调试证书，当前连接不能恢复，也没有 HTTP 状态码。能够识别客户端归属时，模块将该应用和主机的组合加入持久跳过列表；仅同一应用在同一 Android 用户下连接该主机时自动透传。可以在管理器的 HTTPS 调试页查看和清除，或运行 `api traffic pinned` 和 `api traffic clear-pinned`。证书拒绝可能来自钉扎，也可能是客户端未信任 CA；上游 TLS 失败不会加入该名单。新 TCP 记录显示实际透传原因，例如主机被排除、HTTP/2-only、非 TLS 或协议识别超时。未被抓包选中的应用不会产生记录。不支持双向 TLS 和非 WebSocket 的 HTTPS 协议升级。

HTTP/1.1 WebSocket（WS、WSS）连接结束后，在详情中显示握手和前 64 帧的方向、操作码及负载大小；未压缩文本帧最多预览 256 字节。不会修改 WebSocket 帧。

### 可选：Android 系统信任

首次启用 HTTPS 解密时，模块将同一张公开 CA 安装到设备已初始化的各 Android 用户的个人证书存储，包括工作空间和应用分身所在用户。关闭解密保留证书；下次开启解密或启动设备时补装缺少证书的用户。要移除模块安装的副本，请先关闭解密，再在 HTTPS 调试页选择「移除个人调试 CA」，或运行 `api traffic ca-remove`。卸载模块也会清理这些副本，不会移除手动安装的证书。只信任系统 CA 或使用证书固定的应用不会因此信任个人 CA；信任状态已缓存的应用可能需要重新打开。

如果测试客户端无法单独配置 CA，可启用 `system_trust`，由模块在开机或热更新后按当前配置应用。模块将各存储原有的证书和公开调试 CA 放入临时内存文件系统，并在 init、zygote 和已有应用的挂载命名空间中绑定到 `/system/etc/security/cacerts` 以及设备上存在的 `/apex/com.android.conscrypt/cacerts`（Android 14+）。已完成挂载的命名空间不需要额外重启 zygote；如果应用缓存了信任状态，关闭后重新打开该应用。若设备限制挂载，请查看服务日志中的失败步骤和命名空间。系统信任不绕过应用证书钉扎。

关闭解密或停止抓包不影响系统信任。关闭 `system_trust` 或卸载模块时才会尝试卸载模块的系统 CA 目录；如未完成，重启设备。

## 编写规则

将以下文件保存到设备上，例如 `/data/local/tmp/netproxy-rewrite.json`。在 WebUI 输入 `api traffic set-rules /data/local/tmp/netproxy-rewrite.json` 应用，或使用 Root CLI 执行同一操作。不需要重启 Worker 或核心。用 `api traffic rules` 查看，应用 `{"rules":[]}` 删除全部规则。管理器中可开关每一条规则；用 WebUI 或 CLI 时，在该条规则中设 `"enabled":false` 或 `true` 后重新应用。省略时（含旧规则）默认启用，禁用规则不参与匹配或执行，也不改变规则顺序。

```json
{
  "rules": [
    {
      "phase": "request",
      "domains": ["example.org"],
      "pattern": "^http://example[.]org/",
      "script": "return {set_headers: {'X-Debug': 'NetProxy'}};"
    },
    {
      "phase": "response",
      "domains": ["example.org"],
      "pattern": "^http://example[.]org/",
      "script": "if (typeof input.body === 'string') return {body: input.body.replace('Example Domain', 'Debug Domain')};"
    }
  ]
}
```

- 每条规则先配置 `domains`（精确域名或 IP，无端口、无通配符）。只有目标主机在名单内才运行完整 URL（含 `ws://` 或 `wss://` 握手 URL）的 Go 正则匹配；各阶段仍按顺序执行第一条命中规则。旧规则从锚定的字面主机前缀推导域名；无法推导的保留供编辑，但添加域名之前不执行。脚本是函数体，不需要声明函数。
- `input.url`、`input.method` 两个阶段均可用；响应阶段额外提供 `input.status`。在正文可读取时还提供 `input.body`：最大 256 KiB 的 UTF-8 文本或 JSON；不可读取时不提供该字段。
- 两个阶段均可返回 `set_headers` 对象和 `remove_headers` 数组。连接级头、代理认证头和正文长度等由代理管理。
- 请求阶段还可返回 `{redirect: "https://example.org/new"}`（给客户端返回 302）或 `{reject: true}`（403）。两个阶段均可返回 `{body: "..."}` 替换可读取的正文。压缩、超限、分段或非 UTF-8 的正文不支持替换；否则继续原样转发。返回空值表示不改写。
- 规则只应使用你信任的本地脚本。JavaScript 不提供文件、网络或命令执行 API。脚本执行错误、超时或返回格式无效时，忽略该阶段的修改。保存时语法或格式无效会保留原来的规则。
