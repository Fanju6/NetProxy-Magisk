# 节点与订阅

节点页和订阅页使用同一份持久数据。即使服务停止，也可以浏览、添加、编辑、导出、删除或更新内容。

## 本地配置

单个链接和本地文件都追加到固定的“本地配置”分组：

- 支持常见节点链接与节点文本。
- 支持 Clash YAML 和 sing-box JSON。
- 导入不会覆盖已有节点；重复 tag 会自动生成稳定后缀。
- 本地节点可测速、编辑、导出和删除。

文件导入不会按文件名创建额外分组。

### 手写 sing-box 节点文件

将节点放入顶层 `outbounds` 数组，不要直接把单个 outbound 对象作为文件根节点。例如，创建 UTF-8 编码的 `nodes.json`：

```json
{
  "outbounds": [
    {
      "type": "socks",
      "tag": "my-socks",
      "server": "proxy.example.com",
      "server_port": 1080,
      "version": "5",
      "username": "user",
      "password": "password"
    }
  ]
}
```

把示例服务器、端口和账号替换为自己的 SOCKS5 节点。`type` 是协议类型，`tag` 是节点名称；多个节点在同一数组中依次添加，不需要附带 DNS、路由或入站配置。

在管理器“节点 → 添加 → 本地文件”中选择该文件，导入后在“本地配置”中选择节点。也可将文件放到手机 Download 目录，通过 CLI 导入和选择：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl node import /sdcard/Download/nodes.json'
su -c '/data/adb/modules/netproxy/netproxyctl node list'
su -c '/data/adb/modules/netproxy/netproxyctl policy groups'
```

为节点使用清晰、独特的 `tag`，避免与 `direct`、`block`、`Proxy` 等运行时标签重名。导入时重名节点会生成后缀，请按实际名称选择。不要直接修改 Catalog 中的 `provider.json`，应通过管理器或 CLI 编辑节点。

其他协议同样使用对应的 sing-box 出站字段，参考 [Outbound 文档](https://sing-box.sagernet.org/configuration/outbound/)。

## 订阅

订阅页面可以配置：

- 名称、URL 和 User-Agent
- 自定义请求头与 HWID
- 包含、排除筛选规则
- 下载超时、TLS 校验与代理下载策略
- 自动更新周期

名称留空时，依次尝试响应中的 `Profile-Title`、下载文件名、URL 主机名，最后使用“订阅”。支持的响应信息还包括流量用量和到期时间。

订阅更新不依赖正式服务运行。下载、转换或校验失败时，上一版有效节点会继续保留；服务运行时会继续确认新 Provider 是否已在核心中生效，并在失败时记录待同步状态。手动编辑订阅节点只影响当前版本，下次成功更新订阅时会被远端内容覆盖。

## 自动与手动选择

默认 `Proxy` 是原生 Selector。空资源时核心仍可启动，但需要代理的流量会被阻断，直到在「代理」页选择节点；需要直连请显式切到直连模式。自动测速组可在同一份原生配置中按需定义。

在原生配置的 `providers` 引用列表中使用资源名称，例如 `本地配置`。成员的完整标签是 `资源名称/节点标签`。重名时保存为「名称 (2)」「名称 (3)」，资源页和 Provider 都使用最终名称。名称留空的订阅以下载地址的域名命名，后续更新不会改名。资源名称不支持斜线和控制字符。

订阅改名也会改变 Provider 和节点标签；有显式引用时，需先从原生配置移除旧引用再改名。改名后可重新添加引用并重新选中节点。旧版 `np/<分组 ID>` 引用不再兼容：保留配置升级时，请将自定义引用改为资源名称，已有重名资源需先改成不同名称。

从依赖旧活动分组的版本升级时，建议先备份，安装时选择「仅保留节点与订阅」。旧 `module.json` 中的 `selection` 已不再接受；原先由 Go 生成的出站也不会被补齐。需要恢复自定义规则时，请先把原生出站与策略组补到新 `config.json` 中，再用配置检查确认。

需要在全部订阅节点之上再建立地区或业务选择器时，请参阅[策略分组配置教程](/config/policy-groups)。自定义策略组属于 sing-box 出站配置，不会改变资源页中的 Catalog 分组。

## 测速

代理页和 `policy delay` 只在服务运行时请求核心测速；停止时只能查看配置成员。

## CLI 示例

```sh
su -c '/data/adb/modules/netproxy/netproxyctl node add "vless://..."'
su -c '/data/adb/modules/netproxy/netproxyctl node import /sdcard/Download/nodes.json'
su -c '/data/adb/modules/netproxy/netproxyctl sub add https://example.com/sub'
su -c '/data/adb/modules/netproxy/netproxyctl sub update <分组 ID>'
su -c '/data/adb/modules/netproxy/netproxyctl policy groups'
su -c '/data/adb/modules/netproxy/netproxyctl policy select "<策略组标签>" "<成员标签>"'
su -c '/data/adb/modules/netproxy/netproxyctl policy delay "<策略组标签>"'
```

订阅 URL、Header 和节点凭据属于敏感信息。脚本化配置自定义请求头时应使用 `--headers-file`，不要直接放进命令行。
