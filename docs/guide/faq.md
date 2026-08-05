# 常见问题

## 服务启动失败怎么办

先看三类日志：

- `/data/adb/modules/netproxy/logs/service.log`
- `/data/adb/modules/netproxy/logs/sing-box.log`
- `/data/adb/modules/netproxy/logs/subscription.log`

再确认这几项：

- `CURRENT_CONFIG` 指向的节点文件存在
- `config/singbox/outbounds/` 下至少有一个可用节点
- 当前内核支持 BPF、cgroup v2 与 cgroup socket attach
- `config/ebpf/ebpf.conf` 中的网络、DNS、接口和 Map 容量合法
- 当前节点或订阅生成的 sing-box 配置没有语法错误

若 `sing-box.log` 出现 eBPF 加载或挂载错误，说明内核能力、权限或配置不满足要求。本版本不提供 TPROXY / REDIRECT 回退。

## 打不开 zashboard

请依次检查：

1. 服务是否在运行
2. 设备与访问端是否处于同一局域网
3. `9999` 端口是否可达
4. 是否带上正确的 Secret：`singbox`

默认入口：

- `http://<设备IP>:9999`
- `http://<设备IP>:9999/ui`

## 升级后找不到旧 WebUI

这是正常变化。
当前版本已移除内置模块 WebUI，图形化入口改为 **Android 管理器**，控制面板改为 **Clash API + zashboard**。

## 节点导入失败

优先检查：

- 链接本身是否完整
- 订阅地址是否返回有效内容
- 导入文件是否为标准 Clash YAML 或节点列表
- NetProxy 原生组件生成的 Provider 是否包含 sing-box 所需字段

## 切换节点后没有立刻生效

通常先区分两种情况：

- **同目录已加载节点**：优先通过 Clash API 即时切换
- **目标节点不在当前运行目录**：服务可能会重启后生效

如果你启用了 `SELECTOR_MODE=urltest`，运行时默认优先使用动态测速组，而不是固定节点。

## 访问 Google / Chrome 出现个别异常

部分 ROM、浏览器和安全 DNS 组合会影响表现。常见排查方向：

- 检查是否启用了 `GMS_FIX`
- 临时切换 `rule / global / direct` 对比行为
- 在 Android 管理器或控制面板里检查当前实际节点和代理组

## 想看当前真实运行模式和代理组

可以使用：

- Android 管理器的仪表盘与节点页
- CLI：`cli mode`、`cli api groups`、`cli api conns`
- zashboard：查看代理组、规则和连接
