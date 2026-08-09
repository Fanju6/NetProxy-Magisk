# 节点与订阅

NetProxy 使用 Catalog 保存节点和订阅，服务停止时也可以浏览和修改。

## 数据位置

```text
/data/adb/modules/netproxy/data/catalog/
├── default/       # 本地配置
├── <group-id>/    # 订阅或文件导入分组
└── staging/       # 更新事务临时目录
```

每个分组使用 `meta.json` 和 `provider.json`。客户端优先通过 `netproxyctl` 访问 Catalog，不直接解析文件。

## 导入节点

```sh
su -c '/data/adb/modules/netproxy/netproxyctl node add "vless://..."'
su -c '/data/adb/modules/netproxy/netproxyctl node import /sdcard/clash.yaml'
```

单节点和本地文件默认进入 `default` 本地配置组。

## 添加订阅

```sh
su -c '/data/adb/modules/netproxy/netproxyctl sub add 我的订阅 https://example.com/sub'
su -c '/data/adb/modules/netproxy/netproxyctl sub update 我的订阅'
```

订阅更新独立于 sing-box 服务运行。更新失败时保留上一版有效 Provider；更新成功后运行中的 Local Provider 会按需热加载。

## 选择节点

自动测速：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl node use auto default'
```

手动选择：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl node use default/example-tag'
```

自动模式保存空的 `SELECTED_NODE_REF`，手动模式保存 `<group-id>/<tag>`，不保存节点文件路径。

## 常用操作

```sh
su -c '/data/adb/modules/netproxy/netproxyctl catalog list'
su -c '/data/adb/modules/netproxy/netproxyctl node list'
su -c '/data/adb/modules/netproxy/netproxyctl node delay auto default'
su -c '/data/adb/modules/netproxy/netproxyctl node export default/example-tag'
```

Android 管理器提供相同能力的图形化入口。
