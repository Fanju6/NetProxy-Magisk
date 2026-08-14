# 节点与订阅

NetProxy 8.0 使用 Catalog 保存节点和订阅。Catalog 是持久事实源，服务停止时仍可浏览、编辑、导入和更新。

## 数据位置

```text
/data/adb/modules/netproxy/data/catalog/
├── default/       # 本地配置组
├── <group-id>/    # URL 订阅组
└── staging/       # 更新事务临时目录，不是节点组
```

持久分组至少包含：

- `meta.json`：分组名称、来源、更新周期、流量、节点数和运行时同步状态。
- `provider.json`：标准 sing-box Local Provider 节点内容。
- `history.jsonl`：订阅更新历史，只有订阅组使用，内容会脱敏。

客户端通过 `netproxyctl` 访问 Catalog，不应直接扫描或修改这些文件。`staging/` 只在下载、转换、校验和原子提交期间使用。

## 本地节点

单个链接和本地文件都追加到固定的 `default` 本地配置组，不会按文件名创建不可编辑的临时分组：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl node add "vless://..."'
su -c '/data/adb/modules/netproxy/netproxyctl node import /sdcard/clash.yaml'
```

本地节点可以直接测速、编辑、导出和删除。重复 tag 会使用稳定后缀，不覆盖已有节点。

## URL 订阅

添加订阅时名称可以省略：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl sub add 我的订阅 https://example.com/sub'
su -c '/data/adb/modules/netproxy/netproxyctl sub add https://example.com/sub'
```

名称省略时按以下顺序生成：

1. `Profile-Title`
2. `Content-Disposition` 文件名
3. URL 主机名
4. 默认名称“订阅”

订阅设置包括更新周期、User-Agent、HWID、请求头、包含/排除过滤、TLS 校验、超时和是否通过代理更新。自定义请求头通过文件传递，不会出现在进程命令行中。

## 更新事务

更新过程为：下载、解析响应头、转换节点、校验 Provider、原子提交、同步运行时 Provider、写入脱敏历史。

- 服务停止时仍可更新订阅，结果保存为 `not_running`。
- 服务运行时会尝试热加载 Local Provider 并验证节点；失败时保留新 Provider，记录 `runtime_sync_pending`，后续可重试。
- 下载、解析或校验失败时保留上一版有效 Provider。
- HTTP 304 不重写 Provider；如果之前存在待同步状态，仍会尝试恢复运行时同步。
- 全部更新采用顺序处理，一项失败不会阻止其他订阅。

常用命令：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl sub list'
su -c '/data/adb/modules/netproxy/netproxyctl sub show <分组 ID>'
su -c '/data/adb/modules/netproxy/netproxyctl sub update <分组 ID>'
su -c '/data/adb/modules/netproxy/netproxyctl sub update-all'
su -c '/data/adb/modules/netproxy/netproxyctl sub history <分组 ID>'
su -c '/data/adb/modules/netproxy/netproxyctl sub edit <分组 ID> --interval 12h'
```

## 选择节点

自动模式：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl node use auto <分组 ID>'
```

手动模式：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl node use <分组 ID>/<节点标签>'
```

自动模式只保存活动分组和 `urltest` 选择状态，实际测速节点由 Service API 返回。手动节点在订阅更新后消失时回退到同组 Auto，不会回退到 `direct`。
