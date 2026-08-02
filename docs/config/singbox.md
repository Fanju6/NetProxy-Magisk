# sing-box 配置

NetProxy 的 sing-box 配置主目录：

```text
/data/adb/modules/netproxy/config/singbox/
```

## 目录结构

```text
config/singbox/
├── confdir/
├── outbounds/
│   ├── default/
│   └── sub_xxx/
├── runtime/
└── source/
```

### `confdir/`

放通用配置片段，例如：

- 日志
- experimental
- DNS
- 基础路由或 provider 配置

这些内容不直接代表“当前节点”，而是运行实例的公共部分。

### `outbounds/`

节点与订阅目录：

- `default/`：手动导入节点
- `sub_xxx/`：订阅目录

`module.conf` 中的 `CURRENT_CONFIG` 指向其中某个节点文件。

### `runtime/`

运行时生成目录。  
NetProxy 会在服务启动时，根据：

- 当前节点目录
- `SELECTOR_MODE`
- 当前出站模式

生成运行时需要的选择器、测速组与 eBPF 入站：

- `runtime/outbounds.json`
- `runtime/ebpf.json`

### `source/`

规则集与路由相关资源目录，`rule` 模式下会被引用。

## 运行时如何拼装

服务启动时，NetProxy 会组合：

1. `confdir/` 中的公共配置
2. 当前节点目录里已加载的节点文件
3. `runtime/` 中动态生成的出站配置
4. `runtime/ebpf.json` 中的透明代理入站

所以出现问题时，通常要分清楚是：

- 通用配置问题
- 节点文件问题
- 运行时选择器问题
- 路由模式问题

## Clash API 默认值

当前默认配置：

- Controller：`0.0.0.0:9999`
- Secret：`singbox`
- External UI：`/data/adb/modules/netproxy/bin/zashboard`

## 当前节点路径

默认 `CURRENT_CONFIG` 为空。导入并选择节点后，它会指向当前使用的节点文件，例如：

```text
/data/adb/modules/netproxy/config/singbox/outbounds/default/example.json
```

如果你切换到其他订阅目录中的节点，这个路径会随之变化。
