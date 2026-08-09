# 快速开始

这是一条最短闭环：安装模块、导入节点、启动服务、切换模式、验证面板。

## 1. 检查服务状态

```sh
su -c /data/adb/modules/netproxy/scripts/cli service status
```

## 2. 导入节点或订阅

### 单个链接

```sh
su -c '/data/adb/modules/netproxy/scripts/cli node add "vless://..."'
```

### 导入节点文件或 Clash YAML

```sh
su -c '/data/adb/modules/netproxy/scripts/cli node import /sdcard/clash.yaml'
```

### 添加订阅并更新

```sh
su -c '/data/adb/modules/netproxy/scripts/cli sub add 我的订阅 https://example.com/sub'
su -c '/data/adb/modules/netproxy/scripts/cli sub update-all'
```

## 3. 选择节点

```sh
su -c '/data/adb/modules/netproxy/scripts/cli node list'
su -c '/data/adb/modules/netproxy/scripts/cli node use 节点名称'
```

## 4. 启动服务

```sh
su -c /data/adb/modules/netproxy/scripts/cli service start
```

## 5. 确认出站模式

```sh
su -c '/data/adb/modules/netproxy/scripts/cli mode'
su -c '/data/adb/modules/netproxy/scripts/cli mode rule'
```

可选模式：

- `rule`：规则分流
- `global`：全局代理
- `direct`：全局直连

## 6. 打开控制面板

```sh
su -c /data/adb/modules/netproxy/scripts/cli api ui
```

默认入口：

- Controller：`http://<设备IP>:9999`
- UI：`http://<设备IP>:9999/ui`
- Secret：`singbox`

## 7. 遇到问题先看日志

```sh
su -c '/data/adb/modules/netproxy/scripts/cli service logs service 80'
su -c '/data/adb/modules/netproxy/scripts/cli service logs core 80'
```
