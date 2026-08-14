# sing-box 配置

NetProxy 8.0 的 sing-box 配置位于：

```text
/data/adb/modules/netproxy/config/singbox/
```

## 目录结构

```text
config/singbox/
├── confdir/       # 静态 sing-box 配置片段
└── rules/
    ├── local/     # 可编辑的本地规则集
    └── remote/    # 内置远程 SRS 规则资源

data/catalog/
└── <group-id>/    # 节点与订阅 Provider

runtime/           # 启动时生成的运行时配置
```

## 静态配置片段

`confdir/` 按职责保存 sing-box JSON 片段：

- `01_log.json`：日志设置。
- `02_experimental.json`：Clash API、缓存和外部 UI。
- `03_dns.json`：DNS 服务器与 DNS 路由。
- `04_inbounds.json`：用户自定义入站。
- `06_route.json`：路由规则、规则集和出站选择。
- `07_http_clients.json`：HTTP Client 设置。
- `08_services.json`：Service API 与 Dashboard。

运行时节点 Provider、Auto / Select 选择器和 eBPF 入站由 Native 组件生成，不应手动写入静态片段。

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

## API 与 Dashboard

- Service API：`127.0.0.1:9090`，Dashboard 为 `http://127.0.0.1:9090/dashboard/`。
- Clash API：`127.0.0.1:9999`，zashboard 为 `http://127.0.0.1:9999/ui/`。
- 默认密钥：`singbox`。

两个 API 均默认只监听 loopback。固定配置位于 `02_experimental.json` 和 `08_services.json`，不要改回运行时随机生成 bootstrap 的方式。

## 检查配置

检查当前静态配置和 Catalog：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl config check'
```

管理器配置编辑器会先写候选文件，再执行 sing-box 检查和原子替换。不要手动修改 `runtime/`；需要调整 sing-box 行为时，应使用管理器内核设置或 `netproxyctl config` 的事务入口。
