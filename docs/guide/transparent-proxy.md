# eBPF 与分应用代理

NetProxy 使用 sing-box 的 eBPF 入站接管透明代理流量。eBPF 是入站实现，不是独立代理核心或单独服务。

## 数据路径

| 数据路径 | 默认实现 | 作用 |
|---|---|---|
| 本机 | `cgroup` | 在 socket 阶段接管本机应用，不依赖默认出口接口 |
| 共享网络 | `packet_rewrite` | 在热点或 LAN 下游接口改写数据包 |

两条路径由独立开关控制，可以只开启其中一条，也可以同时开启。本机还可以改用跟随默认出口接口的 `tc`；raw-IP、PPP 或隧道类共享接口可以改用 `socket_assign`。两条路径可以分别控制 DNS、IPv6、私网绕过、目标端口和筛选条件。

## 分应用代理

管理器按 Android 用户分别显示应用。保存项必须是：

```text
<用户ID>:<包名>
```

例如主用户与用户 10 的同一个应用是两个条目：

```text
0:com.example.app,10:com.example.app
```

- **黑名单**：名单内应用绕过代理。
- **白名单**：只有名单内应用进入代理，并自动包含 UID 0，避免核心与必要系统流量被排除。

模块不会持久化 UID。每次生成运行时配置时，Go 组件通过 Android package service 按用户查询当前 UID，因此应用重装、分身变化或 UID 变化后，重启核心即可重新解析。

系统 DNS、DownloadManager、isolated process 或 SDK sandbox 代发的流量可能属于其他 UID，不能只凭前台应用包名推断。

## 共享网络

启用共享网络后，至少填写一个实际下游接口，例如设备热点接口。可继续按来源 CIDR 或 MAC 地址限制哪些下游设备进入代理。

接口名由设备和厂商决定，不能把示例值当作所有设备的固定名称。先查看当前热点或 LAN 接口，再填写配置。

## 能力探测

模块直接调用 sing-box 内置探测，不再携带 `bpftool`：

```sh
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status configured'
su -c '/data/adb/modules/netproxy/netproxyctl ebpf status all --raw'
```

默认输出面向用户的诊断；`--raw` 显示核心原始结果。探测不会挂载程序或修改实际流量。

详细字段见 [`ebpf.conf`](/config/ebpf)。
