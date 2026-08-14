---
layout: home

hero:
  name: NetProxy
  text: Android 8.0 sing-box 透明代理模块
  tagline: 以 reF1nd sing-box 为核心，使用 eBPF 接管本机与共享网络流量，统一管理节点、订阅、配置和运行状态。
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
    details: 8.0 以 reF1nd sing-box 为核心，配置、运行时和控制接口都围绕 sing-box 组织。
  - title: Android 管理器
    details: 原生 Android 管理器负责仪表盘、节点、订阅、分应用代理、日志和常用配置编辑。
  - title: CLI + 双 API
    details: netproxyctl、Service API 与 Clash API 分工协作，分别覆盖模块管理、核心状态和第三方面板。
  - title: eBPF 透明代理
    details: 通过 cgroup 与 TC eBPF 接管本机及共享网络流量，不依赖防火墙重定向规则。
  - title: 节点与订阅
    details: 支持单链接、文件和订阅三种导入方式，统一转为 sing-box 节点配置。
  - title: 测试版文档
    details: 文档覆盖 8.0 安装、Catalog、Provider、eBPF、配置参考和常见问题；V7 资料单独保留为历史迁移指南。
---
