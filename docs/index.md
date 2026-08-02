---
layout: home

hero:
  name: NetProxy
  text: Android sing-box 透明代理模块
  tagline: 以 sing-box 为核心，支持 Android 管理器、CLI、Clash API 和 zashboard 的 Android 透明代理模块。
  image:
    src: /N.svg
    alt: NetProxy Logo
  actions:
    - theme: brand
      text: 快速开始
      link: /guide/quick-start
    - theme: alt
      text: 安装与升级
      link: /guide/installation
    - theme: alt
      text: GitHub
      link: https://github.com/Fanju6/NetProxy-Magisk

features:
  - title: sing-box 核心
    details: 当前版本以 sing-box 为核心，配置、运行时和控制接口都围绕 sing-box 组织。
  - title: Android 管理器
    details: 原生 Android 管理器负责仪表盘、节点、订阅、分应用代理、日志和常用配置编辑。
  - title: CLI + Clash API
    details: 命令行分组命令与 Clash API / zashboard 并存，既适合日常操作，也方便自动化和排障。
  - title: eBPF 透明代理
    details: 通过 cgroup 与 TC eBPF 接管本机及共享网络流量，无需 TUN 设备或防火墙重定向规则。
  - title: 节点与订阅
    details: 支持单链接、文件和订阅三种导入方式，统一转为 sing-box 节点配置。
  - title: 兼容与排障
    details: 文档覆盖安装、升级、控制入口、配置参考和常见问题，方便日常使用与排障。
---
