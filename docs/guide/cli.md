# CLI 使用

NetProxy 的 CLI 以 sing-box 架构为中心，脚本路径固定为：

```text
/data/adb/modules/netproxy/scripts/cli
```

通常通过 Root 调用：

```sh
su -c /data/adb/modules/netproxy/scripts/cli help
```

## 命令分组

```text
cli service {status|start|stop|restart|logs}
cli node {list|current|use|add|import|export|show|remove|delay}
cli mode [rule|global|direct]
cli sub {list|add|update|update-all|remove}
cli api {groups|conns|close|close-all|ui}
cli app {list|mode|add|remove|enable|disable}
cli ebpf {status|reload|dns|ipv6|shared|interface}
cli wifi {status|on|off|mode|add|del|list|clear|cellular}
```

## service

### 查看状态

```sh
su -c /data/adb/modules/netproxy/scripts/cli service status
```

### 启动 / 停止 / 重启

```sh
su -c /data/adb/modules/netproxy/scripts/cli service start
su -c /data/adb/modules/netproxy/scripts/cli service stop
su -c /data/adb/modules/netproxy/scripts/cli service restart
```

### 查看日志

```sh
su -c '/data/adb/modules/netproxy/scripts/cli service logs service 80'
su -c '/data/adb/modules/netproxy/scripts/cli service logs core 80'
```

## node

### 列表、当前节点、切换

```sh
su -c '/data/adb/modules/netproxy/scripts/cli node list'
su -c '/data/adb/modules/netproxy/scripts/cli node current'
su -c '/data/adb/modules/netproxy/scripts/cli node use 节点名称'
```

### 导入节点

```sh
su -c '/data/adb/modules/netproxy/scripts/cli node add "vless://..."'
su -c '/data/adb/modules/netproxy/scripts/cli node import /sdcard/clash.yaml'
```

### 延迟测试和导出

```sh
su -c '/data/adb/modules/netproxy/scripts/cli node delay all'
su -c '/data/adb/modules/netproxy/scripts/cli node export 节点名称'
```

## mode

```sh
su -c '/data/adb/modules/netproxy/scripts/cli mode'
su -c '/data/adb/modules/netproxy/scripts/cli mode rule'
su -c '/data/adb/modules/netproxy/scripts/cli mode global'
su -c '/data/adb/modules/netproxy/scripts/cli mode direct'
```

## sub

```sh
su -c '/data/adb/modules/netproxy/scripts/cli sub list'
su -c '/data/adb/modules/netproxy/scripts/cli sub add 我的订阅 https://example.com/sub'
su -c '/data/adb/modules/netproxy/scripts/cli sub update 我的订阅'
su -c '/data/adb/modules/netproxy/scripts/cli sub update-all'
```

## api

```sh
su -c '/data/adb/modules/netproxy/scripts/cli api groups'
su -c '/data/adb/modules/netproxy/scripts/cli api conns'
su -c '/data/adb/modules/netproxy/scripts/cli api ui'
```

## app

```sh
su -c '/data/adb/modules/netproxy/scripts/cli app list'
su -c '/data/adb/modules/netproxy/scripts/cli app mode whitelist'
su -c '/data/adb/modules/netproxy/scripts/cli app add com.example.app'
su -c '/data/adb/modules/netproxy/scripts/cli app enable'
```

## eBPF

```sh
su -c '/data/adb/modules/netproxy/scripts/cli ebpf status'
su -c '/data/adb/modules/netproxy/scripts/cli ebpf reload'
su -c '/data/adb/modules/netproxy/scripts/cli ebpf dns hijack'
su -c '/data/adb/modules/netproxy/scripts/cli ebpf ipv6 on'
su -c '/data/adb/modules/netproxy/scripts/cli ebpf shared on'
su -c '/data/adb/modules/netproxy/scripts/cli ebpf interface add wlan2'
```
