---
layout: home

hero:
  name: NetProxy
  text: Android sing-box 透明代理模块
  tagline: 使用 eBPF 接管本机与共享网络流量，在一个模块中管理节点、订阅、分应用策略和运行状态。
  image:
    src: /N.svg
    alt: NetProxy 标志
  actions:
    - theme: brand
      text: 安装
      link: /guide/installation
    - theme: alt
      text: 快速开始
      link: /guide/quick-start
    - theme: alt
      text: 排查问题
      link: /guide/faq

features:
  - title: eBPF 透明代理
    details: 接管本机应用与热点、LAN 等共享网络流量，支持 TCP、UDP、IPv4 和 IPv6。
  - title: 节点与订阅
    details: 管理本地节点和订阅，支持导入、筛选、自动更新、测速、编辑、导出与更新历史。
  - title: 分应用代理
    details: 按 Android 用户分别配置应用黑白名单，兼容多用户、应用分身和工作资料。
  - title: Wi-Fi 自动策略
    details: 结合 SSID 与真实出口判断，在 Wi-Fi、移动数据和双连接场景下自动评估出站模式。
  - title: 多种管理入口
    details: 使用 Android 管理器完成日常操作，也可通过终端 WebUI、CLI 和 sing-box 面板管理。
  - title: 可审查的配置
    details: 常用选项可视化，高级配置保留完整文件入口；检查失败不会覆盖上一份有效配置。
---

<img class="home-screenshot" src="/Screenshot.jpg" alt="NetProxy Android 管理器界面" />
