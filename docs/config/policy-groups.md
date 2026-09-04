# 策略分组配置教程

NetProxy 默认按节点来源生成 `Auto/<分组>` 和 `Select/<分组>`。如果希望进一步按地区整理节点，并为 AI、流媒体或社交应用单独选择出口，可以在 sing-box 配置中增加策略分组。

本教程从零建立一套最小配置，再说明如何继续扩展。完成后可以得到这样的流量路径：

```text
本地节点与订阅
      ↓
Catalog Provider
      ↓
地区分组
      ↓
业务分组
      ↓
路由规则
```

策略分组只引用 NetProxy 已有的 Provider，不会复制节点，也不会创建新的 Catalog 分组。

## 先理解三个概念

### 地区分组

地区分组从全部 Provider 中读取节点，再按节点名称筛选。例如：

```json
{
  "type": "selector",
  "tag": "Hong Kong",
  "use_all_providers": true,
  "include": "(?i)(🇭🇰|香港|港区|港區|hong[ _-]?kong|(^|[^a-z])hk([^a-z]|$))"
}
```

- `use_all_providers`：读取 NetProxy 生成的全部 Provider。
- `include`：只收入名称匹配该正则的节点。
- `tag`：分组名称，也是其他配置引用它时使用的唯一标识。

### 业务分组

业务分组不直接匹配节点，而是让用户从 `Proxy`、地区分组、全部节点或 `direct` 中选择。例如：

```json
{
  "type": "selector",
  "tag": "Netflix",
  "outbounds": [
    "Proxy",
    "Hong Kong",
    "Japan",
    "United States",
    "All Proxies",
    "direct"
  ],
  "default": "Proxy",
  "interrupt_exist_connections": true
}
```

### 路由规则

路由规则负责判断流量属于哪个业务，再把连接交给对应的业务分组：

```json
{
  "rule_set": [
    "netflix",
    "netflix-ip"
  ],
  "action": "route",
  "outbound": "Netflix"
}
```

`outbound` 必须与业务分组的 `tag` 完全一致。

## 第一步：确定需要的分组

初次使用不建议一次建立几十个分组。先选择两个或三个常用地区，再增加一两个确实需要单独控制的业务。

本教程使用以下结构：

| 类型 | 分组 |
|---|---|
| 地区 | Hong Kong、Japan、United States |
| 节点集合 | All Proxies |
| 业务 | AI、Netflix |
| 兜底 | Final |

其中 `Proxy` 是 NetProxy 已有的顶层选择器，不需要重复定义。

## 第二步：准备自定义出站

在手机或电脑的文本编辑器中创建 `policy-groups.json`，填入以下内容，作为主配置 `outbounds` 分区的候选值：

```json
{
  "outbounds": [
    {
      "type": "selector",
      "tag": "Hong Kong",
      "use_all_providers": true,
      "include": "(?i)(🇭🇰|香港|港区|港區|hong[ _-]?kong|(^|[^a-z])hk([^a-z]|$))",
      "interrupt_exist_connections": true
    },
    {
      "type": "selector",
      "tag": "Japan",
      "use_all_providers": true,
      "include": "(?i)(🇯🇵|日本|东京|東京|大阪|japan|(^|[^a-z])jp([^a-z]|$))",
      "interrupt_exist_connections": true
    },
    {
      "type": "selector",
      "tag": "United States",
      "use_all_providers": true,
      "include": "(?i)(🇺🇸|美国|美國|洛杉矶|洛杉磯|西雅图|西雅圖|达拉斯|達拉斯|硅谷|矽谷|united[ _-]?states|america|(^|[^a-z])us([^a-z]|$))",
      "interrupt_exist_connections": true
    },
    {
      "type": "selector",
      "tag": "All Proxies",
      "use_all_providers": true,
      "interrupt_exist_connections": true
    },
    {
      "type": "selector",
      "tag": "AI",
      "outbounds": [
        "Proxy",
        "United States",
        "Japan",
        "Hong Kong",
        "All Proxies",
        "direct"
      ],
      "default": "Proxy",
      "interrupt_exist_connections": true
    },
    {
      "type": "selector",
      "tag": "Netflix",
      "outbounds": [
        "Proxy",
        "United States",
        "Japan",
        "Hong Kong",
        "All Proxies",
        "direct"
      ],
      "default": "Proxy",
      "interrupt_exist_connections": true
    },
    {
      "type": "selector",
      "tag": "Final",
      "outbounds": [
        "Proxy",
        "United States",
        "Japan",
        "Hong Kong",
        "All Proxies",
        "direct"
      ],
      "default": "Proxy",
      "interrupt_exist_connections": true
    }
  ]
}
```

这里没有建立跨全部 Provider 的 `urltest`。`All Proxies` 只是手动选择器，不会额外对全部节点执行周期测速；NetProxy 原有的 `Auto/<分组>` 仍照常工作。

将文件保存到手机：

```text
/sdcard/Download/policy-groups.json
```

## 第三步：备份当前主配置

修改前先备份，备份可能包含敏感配置，不要公开分享：

```sh
su -c 'mkdir -p /sdcard/Download/NetProxy-backup && cp /data/adb/modules/netproxy/config/singbox/config.json /sdcard/Download/NetProxy-backup/config.json'
```

如果主配置已经有自定义 `outbounds`，先在“内核设置 → 自定义出站”中取出原内容，把上述分组追加到已有数组后再保存，不要覆盖原有出站。后续路由也应在现有规则上扩展。

## 第四步：加入策略分组

主配置还没有自定义出站时，可以通过配置事务应用候选分区：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl config apply singbox/outbounds /sdcard/Download/policy-groups.json'
```

`config apply` 会把候选 `outbounds` 放入主配置，与 Catalog Provider 和运行时出站一起检查。通过后原子保存主配置；核心正在运行时会自动重新加载。其他顶层字段不受影响。

后续在 Android 管理器“内核设置 → 自定义出站”中修改即可。候选文件只是导入输入，不会作为独立配置文件部署到模块。

## 第五步：添加业务规则集

在 Android 管理器中打开：

```text
设置 → 内核设置 → 路由
```

找到 `route.rule_set` 中使用 `https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/{tag}.srs` 的远程规则项，在它的 `tag` 数组末尾追加 `geosite/netflix` 与 `geoip/netflix`。保留其他字段和原有标签，完整数组如下：

```json
[
  "geosite/category-ads-all",
  "geosite/apple-cn",
  "geosite/category-ai-!cn",
  "geosite/google",
  "geosite/geolocation-!cn",
  "geosite/cn",
  "geoip/cn",
  "geoip/telegram",
  "geosite/netflix",
  "geoip/netflix"
]
```

`geosite/category-ai-!cn` 已由默认配置提供，不必重复添加。如果 Netflix 标签已经存在，也不要再追加。

## 第六步：把流量送入业务分组

继续在 `route.rules` 数组中增加：

```json
{
  "rule_set": "geosite/category-ai-!cn",
  "action": "route",
  "outbound": "AI"
},
{
  "rule_set": [
    "geosite/netflix",
    "geoip/netflix"
  ],
  "action": "route",
  "outbound": "Netflix"
}
```

把业务规则插在广告拒绝规则之后、默认的 `geosite/category-ai-!cn` / `geosite/google` 代理规则之前。不能只放在 `geosite/geolocation-!cn` 之前，否则 AI 流量已经被前面的默认规则送往 `Proxy`，不会进入新建的 `AI` 分组。

最后把 `route.final` 修改为：

```json
"final": "Final"
```

保存时，Android 管理器会先校验整个 sing-box 配置。校验失败时不要强行覆盖，根据错误路径检查 JSON 逗号、重复 tag 和不存在的出站名称。

## 第七步：检查并使用

执行一次完整检查：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl config check'
```

如果核心没有自动重新加载，可以手动重启：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl service restart'
```

策略组可在以下面板中切换：

- zashboard：`http://127.0.0.1:9999/ui/`
- Service API Dashboard：`http://127.0.0.1:9090/dashboard/`
- 默认密钥：`singbox`

例如将 Netflix 切换为 United States，只会改变命中 Netflix 规则的连接。`interrupt_exist_connections` 会在切换后关闭该组已有连接，使新选择尽快生效。

::: info Android 管理器中的节点页
节点页展示的是 Catalog 分组和节点，不是任意 sing-box selector。自定义地区组与业务组应在 Dashboard 中管理，这是两类不同的数据。
:::

## 继续增加地区

复制一个现有地区 selector，修改 `tag` 和 `include`。下面是常用匹配表达式：

| 地区 | `tag` | `include` 关键词 |
|---|---|---|
| 台湾 | `Taiwan` | `🇹🇼`、台湾、臺灣、台北、Taiwan、TW |
| 新加坡 | `Singapore` | `🇸🇬`、新加坡、狮城、Singapore、SG |
| 香港 | `Hong Kong` | `🇭🇰`、香港、Hong Kong、HK |
| 日本 | `Japan` | `🇯🇵`、日本、东京、大阪、Japan、JP |
| 美国 | `United States` | `🇺🇸`、美国、洛杉矶、西雅图、United States、US |

新增地区后，还要把它加入需要使用该地区的业务分组 `outbounds` 数组。

正则应尽量匹配完整地区名、旗帜或有边界的缩写。不要只匹配“美”“新”等单个汉字，否则容易收入无关节点。

## 继续增加业务

增加一个业务通常需要三步：

1. 在“自定义出站”的 `outbounds` 数组中增加业务 selector。
2. 在 `route.rule_set` 中声明对应规则资源。
3. 在 `route.rules` 中把规则送到该 selector。

常见规则 tag：

| 业务 | Geosite tag | GeoIP tag | GeoIP 远程文件 |
|---|---|---|---|
| Google | `geosite/google` | `geoip/google` | `google.srs` |
| Telegram | `geosite/telegram` | `geoip/telegram` | `telegram.srs` |
| Twitter | `geosite/twitter` | `geoip/twitter` | `twitter.srs` |
| Netflix | `geosite/netflix` | `geoip/netflix` | `netflix.srs` |
| YouTube | `geosite/youtube` | 不需要 | - |
| GitHub | `geosite/github` | 不需要 | - |
| Spotify | `geosite/spotify` | 不需要 | - |
| Bilibili | `geosite/bilibili` | 不需要 | - |

Geosite 与 GeoIP 共用下载模板，`{tag}` 包含 `geosite/` 或 `geoip/` 前缀：

```text
https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/{tag}.srs
```

例如增加 `geoip/google`，可以直接追加到现有远程规则的标签数组；需要单独声明时使用：

```json
{
  "type": "remote",
  "tag": "geoip/google",
  "format": "binary",
  "url": "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/google.srs",
  "update_interval": "24h",
  "path": "./rules/remote/geoip/google.srs"
}
```

本地缓存文件名可以自定义，但 `route.rule_set` 中声明的 tag 和 `route.rules` 中引用的 tag 必须一致。

## 恢复默认配置

停止服务，通过一次事务恢复完整备份，避免先删出站造成路由引用失效：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl service stop'
su -c '/data/adb/modules/netproxy/netproxyctl config apply singbox/config.json /sdcard/Download/NetProxy-backup/config.json'
su -c '/data/adb/modules/netproxy/netproxyctl service start'
```

没有备份时，可以从同版本模块安装包中取出默认 `config/singbox/config.json`，再通过 `config apply singbox/config.json` 恢复，但这会重置所有静态配置修改。Catalog 节点和本地规则不会因此被删除。

## 常见问题

### 地区组为空

节点 tag 没有命中 `include`。先在节点页查看实际名称，再调整正则。

### 配置检查提示出站不存在

检查 `route.rules[].outbound` 是否与主配置 `outbounds` 中的 `tag` 完全一致，并确认自定义出站已经成功保存。

### 配置检查提示规则集重复

默认路由可能已经声明了同名 tag。保留原定义，不要再添加第二份。

### 首次启动提示规则集下载失败

新增远程规则需要先下载 SRS 文件。检查网络、核心日志和 `s-download` HTTP Client。

### 路由结果与预期不同

规则按顺序匹配。Global、Direct、本地 `proxy/direct/block` 和广告规则会优先生效；更具体的业务规则应放在宽泛的国外或国内规则之前。

## 相关资料

- [Selector 出站](https://sing-box.sagernet.org/configuration/outbound/selector/)
- [路由规则](https://sing-box.sagernet.org/zh/configuration/route/rule/)
- [规则动作](https://sing-box.sagernet.org/zh/configuration/route/rule_action/)
- [规则集](https://sing-box.sagernet.org/configuration/rule-set/)
