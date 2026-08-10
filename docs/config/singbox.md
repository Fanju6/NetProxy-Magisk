# sing-box 配置

NetProxy 的 sing-box 静态配置位于：

```text
/data/adb/modules/netproxy/config/singbox/
```

## 目录结构

```text
config/singbox/
├── confdir/       # 静态 sing-box 配置片段
└── source/        # 规则集与路由资源

data/catalog/
└── <group-id>/    # 节点与订阅 Provider

runtime/           # 启动时生成的运行时配置
```

### `confdir/`

这里保存日志、DNS、路由、入站、HTTP Client 和 Service API 等静态配置。运行时节点、Provider 和 eBPF 入站由模块根据 Catalog 自动生成，不能直接写入静态片段。

### `source/`

这里保存规则集和其他静态资源。规则模式会按 `06_route.json` 的引用加载这些文件。

### `data/catalog/`

节点与订阅的事实源位于 `data/catalog/<group-id>/`，每个分组至少包含：

- `meta.json`：分组名称、订阅信息、更新状态和统计数据。
- `provider.json`：标准 sing-box Local Provider 内容。
- `history.jsonl`：订阅更新历史，仅订阅分组使用。

客户端停止服务时也可以通过 `netproxyctl` 读取 Catalog，不需要读取运行时文件。

### `runtime/`

运行时目录由服务启动和配置检查创建，包含：

- `providers.json`
- `outbounds.json`
- `ebpf.json`

运行时目录只包含启动或检查时生成的 sing-box 配置，不应由用户编辑。

## 配置组合

启动时 sing-box 按以下顺序组合配置：

1. 加载 `config/singbox/confdir/` 中的静态片段。
2. 加载 `runtime/providers.json`、`runtime/outbounds.json` 和 `runtime/ebpf.json`。
3. 由运行时 Provider、选择器和 eBPF 入站提供当前节点与透明代理能力。

节点选择由 `config/module.conf` 中的 `ACTIVE_GROUP_ID`、`SELECTOR_MODE` 和 `SELECTED_NODE_REF` 表示。自动模式使用 `Auto/<group>`，手动模式使用 `<group-id>/<tag>`，不会用文件路径保存选择状态。

## 控制接口

- Service API：`127.0.0.1:9090`，供 Android 管理器和模块控制使用。
- Clash API：`127.0.0.1:9999`，供本机 zashboard 和第三方 Clash 客户端使用。
- 默认密钥：`singbox`。

## 检查配置

配置检查由模块统一执行：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl config validate'
```

不要手动修改 `runtime/` 中的文件；需要修改 sing-box 行为时，请使用 Android 管理器的内核设置或通过 `netproxyctl config` 提交候选配置。
