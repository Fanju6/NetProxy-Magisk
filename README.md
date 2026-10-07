<p align="center">
  <img src="docs/public/N.svg" alt="NetProxy Logo" width="120" />
</p>

<h1 align="center">NetProxy</h1>

<p align="center">
  <strong>Android 系统级 sing-box 透明代理模块</strong><br>
  eBPF / TUN · 节点与订阅 · 分应用代理 · 共享网络
</p>

<p align="center">
  <a href="https://github.com/Fanju6/NetProxy-Magisk/releases">
    <img src="https://img.shields.io/github/v/release/Fanju6/NetProxy-Magisk?style=flat-square&label=Release&color=blue" alt="Latest Release" />
  </a>
  <a href="https://github.com/Fanju6/NetProxy-Magisk/releases">
    <img src="https://img.shields.io/github/downloads/Fanju6/NetProxy-Magisk/total?style=flat-square&color=green" alt="Downloads" />
  </a>
  <img src="https://img.shields.io/badge/Core-sing--box-blueviolet?style=flat-square" alt="sing-box Core" />
</p>

<p align="center">
  <a href="https://github.com/Fanju6/NetProxy-Magisk/releases">下载模块与管理器</a> ·
  <a href="https://www.netproxy.store/">使用文档</a> ·
  <a href="src/android/">管理器源码</a> ·
  <a href="https://t.me/NetProxy_Magisk">Telegram</a>
</p>

<p align="center">
  中文 | <a href="README_EN.md">English</a>
</p>

---

## 项目简介

NetProxy 是面向已 Root Android 设备的系统级透明代理模块，支持 **Magisk、KernelSU 与 APatch**。使用 sing-box，通过 eBPF 或 TUN + auto_redirect 接管流量。

日常操作可使用随模块提供的 Android 管理器，也可通过终端式模块 WebUI、CLI 或 sing-box Dashboard 管理。节点与订阅在服务停止时仍可浏览和编辑。

## 界面预览

<div align="center">
  <img src="docs/public/Screenshot.jpg" width="60%" alt="NetProxy Android 管理器界面预览" />
</div>

## 主要功能

- **透明代理**：eBPF 或 TUN 接管 TCP、UDP 与 DNS，支持热点和 USB 共享网络。
- **节点与订阅**：导入节点链接、节点文本、Clash YAML 和 sing-box JSON；支持订阅筛选、请求头、流量信息与自动更新。
- **节点选择与测速**：手动选择或自动优选，服务停止时也可测速。
- **分应用代理**：黑名单 / 白名单，按 Android 用户分别配置，支持应用分身。
- **网络策略**：规则、全局、直连与允许广告模式，按 Wi-Fi 名称和实际出口自动切换。
- **配置与诊断**：编辑 sing-box 配置、本地规则，查看运行时配置与日志，导出脱敏诊断包。

## 安装与开始使用

运行环境为 `arm64-v8a` Android 设备与 Magisk、KernelSU 或 APatch，使用 Root 权限接管流量。

默认使用 eBPF，可在“设置 → 入站设置”中选择 TUN。设备能力要求与安装选项见[安装与升级](https://www.netproxy.store/guide/installation)。

1. 从 [Releases](https://github.com/Fanju6/NetProxy-Magisk/releases) 下载 `NetProxy_<版本>_<构建号>.zip`，内含模块和 Android 管理器。
2. 在 Root 管理器中刷入，按提示选择数据保留方式及是否安装管理器；等待安装完成。
3. 打开 Android 管理器并授予 Root 权限，在“节点”页导入节点，或在“订阅”页添加订阅。
4. 在“节点”页选择分组与自动 / 手动节点，返回“仪表盘”开启服务。

服务默认手动启动，开机自启可在“设置”中开启。首次配置参阅[快速开始](https://www.netproxy.store/guide/quick-start)。

## 使用文档

| 内容 | 文档 |
|---|---|
| 安装与首次配置 | [安装与升级](https://www.netproxy.store/guide/installation) · [快速开始](https://www.netproxy.store/guide/quick-start) |
| 节点、订阅与测速 | [节点与订阅](https://www.netproxy.store/guide/nodes-subscriptions) |
| 入站、分应用与共享网络 | [入站配置](https://www.netproxy.store/config/inbound) · [eBPF](https://www.netproxy.store/config/ebpf) · [TUN](https://www.netproxy.store/config/tun) |
| 网络自动切换 | [Wi-Fi 自动策略](https://www.netproxy.store/guide/wifi-policy) |
| 路由、DNS 与策略组 | [配置参考](https://www.netproxy.store/config/singbox) · [策略分组](https://www.netproxy.store/config/policy-groups) |
| 终端、面板与第三方客户端 | [CLI](https://www.netproxy.store/guide/cli) · [控制面板与 API](https://www.netproxy.store/guide/control-panel) |
| 故障排查与问题反馈 | [常见问题与诊断](https://www.netproxy.store/guide/faq) |

[更新日志](https://www.netproxy.store/changelog) · [隐私政策](https://www.netproxy.store/privacy)

## 奶屁伙伴

<p align="center">
  <a href="https://www.netproxy.store/mascot/dragon"><img src="docs/public/mascots/dragon/base.svg" width="144" alt="薄荷色像素奶屁龙" /></a>
  <a href="https://www.netproxy.store/mascot/niang"><img src="docs/public/mascots/niang/stickers/16-leave-it.png" width="144" alt="长发蓝蝴蝶结的奶屁娘：交给我" /></a>
</p>

**奶屁龙**，通路工坊的开路员。爱挖洞，也爱打盹，偶尔会把出口挖到饼干柜里。

**奶屁娘**，通路工坊的主人。讲究、有点傲娇，嘴上问着“日志呢？”，手里已经递来一杯热饮。

[工坊故事](https://www.netproxy.store/mascot/) · [表情包](https://www.netproxy.store/mascot/stickers) · [股东名册](https://www.netproxy.store/mascot/supporters)

## 源码结构

```text
src/module/          模块安装与运行文件
src/native/netproxy/ Go 原生组件
src/webui/           终端式模块 WebUI
src/android/         Android 管理器
docs/                用户文档站
tests/               契约与回归测试
```

开发流程与验证方法见[贡献指南](CONTRIBUTING.md)，架构和跨组件契约见 [AGENTS.md](AGENTS.md)，管理器构建见 [Android 说明](src/android/README.md)。

## 鸣谢

| 项目 | 用途 |
|------|------|
| [reF1nd/sing-box](https://github.com/reF1nd/sing-box) | 当前代理核心 |
| [SagerNet/sing-box](https://github.com/SagerNet/sing-box) | 上游 sing-box 项目 |
| [Proxylink](https://github.com/Fanju6/Proxylink) | NetProxy 内部节点转换能力的原始项目 |
| [AsteriskNG](https://github.com/Asterisk4Magisk/AsteriskNG) | Android eBPF 实现参考 |
| [v2rayNG](https://github.com/2dust/v2rayNG) | 节点解析实现参考 |

同时感谢曾为早期版本提供核心能力或实现参考的 [CHIZI-0618/sing-box](https://github.com/CHIZI-0618/sing-box)、[Xray-core](https://github.com/XTLS/Xray-core)、[AndroidTProxyShell](https://github.com/CHIZI-0618/AndroidTProxyShell)、[IPSET_LKM](https://github.com/TanakaLun/IPSET_LKM) 与 [KsuWebUIStandalone](https://github.com/KOWX712/KsuWebUIStandalone)。

## 交流与贡献

- [Telegram 群组](https://t.me/NetProxy_Magisk)
- [提交 Issue](https://github.com/Fanju6/NetProxy-Magisk/issues)
- [提交 Pull Request](https://github.com/Fanju6/NetProxy-Magisk/pulls)

## 许可证

[GPL-3.0 License](LICENSE)

## Star

[![Star History Chart](https://star-history.dera.page/svg?repos=Fanju6/NetProxy-Magisk&type=date&legend=top-left)](https://star-history.dera.page/#Fanju6/NetProxy-Magisk&type=date&legend=top-left)
