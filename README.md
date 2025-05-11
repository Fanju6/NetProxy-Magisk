# NetProxy-Magisk

基于 Xray 内核的 Magisk 代理模块，支持一键启动/停止透明代理，适用于 Android 设备。

## 功能

- 一键启动/停止 Xray 代理服务
- 自动配置 iptables 透明代理规则
- 支持全局 TCP 流量代理
- 日志记录，方便排查问题
- 适配 Magisk 模块环境

## 安装步骤

1. 下载本模块的 zip 文件
2. 在 Magisk Manager 中安装该模块
3. 重启设备

## 使用方法

- 安装后，模块会自动启动 Xray 服务并配置 iptables 规则
- 修改代理配置文件，在 Magisk 内点击模块执行按钮重启内核生效

## 配置说明

- 配置文件位于 `/data/adb/modules/netproxy/XrayCore/Config/config.json`
- 默认监听端口为 1080，可根据需要修改

## 注意事项

- 请确保设备已 root 并安装 Magisk
- 部分设备可能需要手动调整 iptables 规则

## 贡献

欢迎提交 Issue 或 Pull Request 来改进本模块！
