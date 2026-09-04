# sing-box 配置

NetProxy 的 sing-box 配置位于：

```text
/data/adb/modules/netproxy/config/singbox/
```

## 目录结构

```text
config/singbox/
├── config.json    # 唯一静态主配置
└── rules/
    ├── local/     # 可编辑的本地规则集
    └── remote/    # 内置远程 SRS 规则资源

data/catalog/
└── <group-id>/    # 节点与订阅 Provider

runtime/           # 启动时生成的运行时配置
```

## 静态主配置

`config.json` 按 sing-box 顶层字段组织配置：

- `log`：日志设置。
- `experimental`：缓存、observability、Clash API 和外部 UI。
- `dns`：DNS 服务器与 DNS 路由。
- `inbounds`：用户自定义入站。
- `route`：路由规则、规则集和出站选择。
- `http_clients`：HTTP Client 设置。
- `services`：Service API 与 Dashboard。

运行时节点 Provider、Auto / Select / Proxy 选择器和 eBPF 入站由 Native 组件生成，不应在主配置中重复定义。主配置可以增加独立命名的自定义出站和[策略分组](./policy-groups)。

### 在管理器中编辑

进入“设置 → 内核设置”，可以分别打开 DNS、入站、路由等分区，也可以打开“完整配置”。分区是主配置的编辑视图，不是另一个磁盘文件。

DNS 编辑器只显示 `{"dns": {...}}`。保存时 Go 只替换 `dns`，其他分区保持不变；不能在 DNS 编辑器中写入 `route` 等其他顶层字段。将内容改为 `{}` 会删除该分区，`null` 与删除不是一回事。

保存会检查读取时的版本。同一分区已被其他客户端修改时，会提示重新加载，防止覆盖他人的修改；其他分区更新不会阻止当前分区保存。

### 更换整份配置

先备份 `config.json`，再通过“完整配置”或 `config apply singbox/config.json` 提交候选文件。校验时会一起加载 Catalog 与 eBPF 运行时，失败不替换当前配置。

上游通用配置不能保证直接可用：需要保留 NetProxy 的控制 API，避免与自动生成的出站和入站重复，并确认规则路径相对于 `config/singbox/` 有效。`rules/` 不会内嵌到主配置中。

安装选择“保留现有数据”时保留用户主配置，不用包内默认值覆盖它。安装器不转换旧配置格式；从片段目录布局升级时，先导出节点、记录订阅与个人设置，再选择“全新安装”，之后按当前格式重新配置。

## 规则集

- `rules/local/`：用户可编辑的 `block.json`、`direct.json` 和 `proxy.json` 等规则集。
- `rules/remote/`：模块内置的 `.srs` 规则资源，由远程 Provider 更新，不通过配置编辑器修改。

本地规则和内置远程规则是两类不同资源。升级时内置资源按工作流更新，本地规则属于用户数据并由安装流程保留。

## Catalog 与运行时

节点与订阅事实源位于：

```text
/data/adb/modules/netproxy/data/catalog/<group-id>/
```

每个分组通常包含 `meta.json`、`provider.json` 和订阅组的 `history.jsonl`。服务停止时仍可通过 `netproxyctl` 读取 Catalog，不需要读取运行时文件。

启动或配置检查时，NetProxy 生成：

- `runtime/providers.json`
- `runtime/outbounds.json`
- `runtime/ebpf.json`

这些文件可以帮助排障，但会随 Catalog、选择状态和 eBPF 设置重新生成，不应直接编辑。

## 临时运行状态

短生命周期状态位于内存文件系统：

```text
/dev/netproxy/
├── service.json       # 当前启动周期的服务状态
├── worker.pid         # 后台 Worker PID
├── subscriptions/     # 正在执行的订阅进度与取消标记
├── delay/             # 离线测速临时会话
└── wifi_state         # 最近一次 Wi-Fi 策略结果
```

这些文件在重启后可重新建立，不属于 sing-box 配置，也不会显示在内核配置编辑器中。`service.json` 只表示当前启动周期；连接、流量和实际节点仍以运行中的 API 为准。

## API 与 Dashboard

- Service API：`127.0.0.1:9090`，Dashboard 为 `http://127.0.0.1:9090/dashboard/`。
- Clash API：`127.0.0.1:9999`，zashboard 为 `http://127.0.0.1:9999/ui/`。
- 默认密钥：`singbox`。

两个 API 均默认只监听 loopback。固定配置位于主配置的 `experimental.clash_api` 和 `services`，替换主配置时不要移除或随意更改它们，否则管理器和面板可能无法连接核心。

## 检查配置

检查当前静态配置和 Catalog：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl config check'
```

管理器配置编辑器会先写候选文件，再执行 sing-box 检查和原子替换。不要手动修改 `runtime/`；需要调整 sing-box 行为时，应使用管理器内核设置或 `netproxyctl config` 的事务入口。
