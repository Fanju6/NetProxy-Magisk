import { defineConfig } from 'vitepress'

const siteUrl = 'https://www.netproxy.store'

export default defineConfig({
  title: 'NetProxy',
  description: 'Android sing-box 透明代理模块，支持 eBPF、节点与订阅、分应用代理和共享网络。',
  lang: 'zh-CN',
  base: '/',
  cleanUrls: true,
  lastUpdated: true,
  sitemap: {
    hostname: siteUrl
  },

  head: [
    ['link', { rel: 'icon', href: '/N.svg' }],
    ['link', { rel: 'canonical', href: siteUrl }],
    ['meta', { name: 'theme-color', content: '#008a73' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:site_name', content: 'NetProxy' }],
    ['meta', { property: 'og:title', content: 'NetProxy' }],
    ['meta', { property: 'og:description', content: 'Android sing-box 透明代理模块' }],
    ['meta', { property: 'og:url', content: siteUrl }],
    ['meta', { property: 'og:image', content: `${siteUrl}/Screenshot.jpg` }],
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
    [
      'script',
      {
        async: '',
        src: 'https://pagead2.googlesyndication.com/pagead/js/adsbygoogle.js?client=ca-pub-4560980501971534',
        crossorigin: 'anonymous'
      }
    ]
  ],

  themeConfig: {
    logo: '/N.svg',
    siteTitle: 'NetProxy',

    nav: [
      { text: '开始使用', link: '/guide/introduction' },
      { text: '配置参考', link: '/config/module' },
      {
        text: '互动工具',
        items: [
          { text: '莫奈调色器', link: '/tools/monet' },
          { text: '祈愿模拟器', link: '/tools/gacha' }
        ]
      },
      { text: '更新日志', link: '/changelog' },
      { text: 'GitHub', link: 'https://github.com/Fanju6/NetProxy-Magisk' }
    ],

    sidebar: {
      '/guide/': [
        {
          text: '开始使用',
          items: [
            { text: '项目介绍', link: '/guide/introduction' },
            { text: '安装与升级', link: '/guide/installation' },
            { text: '快速开始', link: '/guide/quick-start' }
          ]
        },
        {
          text: '日常使用',
          items: [
            { text: 'Android 管理器', link: '/guide/android-manager' },
            { text: '节点与订阅', link: '/guide/nodes-subscriptions' },
            { text: 'eBPF 与分应用代理', link: '/guide/transparent-proxy' },
            { text: 'Wi-Fi 自动策略', link: '/guide/wifi-policy' },
            { text: '控制面板与 API', link: '/guide/control-panel' }
          ]
        },
        {
          text: '进阶与支持',
          items: [
            { text: 'CLI', link: '/guide/cli' },
            { text: '常见问题与诊断', link: '/guide/faq' },
            { text: '设计理念', link: '/guide/philosophy' }
          ]
        },
        {
          text: '历史',
          items: [
            { text: '7.2 → 8.0 架构迁移', link: '/guide/compare-v7-v8' },
            { text: '6.x → 7.x 升级指南', link: '/guide/upgrade-v7' }
          ]
        }
      ],
      '/config/': [
        {
          text: '配置参考',
          items: [
            { text: 'module.conf', link: '/config/module' },
            { text: 'ebpf.conf', link: '/config/ebpf' },
            { text: 'sing-box 配置与运行时', link: '/config/singbox' },
            { text: '路由与 DNS', link: '/config/routing' },
            { text: '策略分组配置', link: '/config/policy-groups' },
            { text: 'CLI', link: '/guide/cli' }
          ]
        }
      ],
      '/tools/': [
        {
          text: '互动工具',
          items: [
            { text: '莫奈调色器', link: '/tools/monet' },
            { text: '祈愿模拟器', link: '/tools/gacha' }
          ]
        }
      ]
    },

    socialLinks: [{ icon: 'github', link: 'https://github.com/Fanju6/NetProxy-Magisk' }],
    editLink: {
      pattern: 'https://github.com/Fanju6/NetProxy-Magisk/edit/main/docs/:path',
      text: '在 GitHub 上编辑此页'
    },
    footer: {
      message: '基于 GPL-3.0 许可证发布 · <a href="/privacy">隐私政策</a>',
      copyright: 'Copyright © 2024-present Fanju'
    },
    search: { provider: 'local' },
    outline: { label: '本页内容', level: [2, 3] },
    docFooter: { prev: '上一页', next: '下一页' },
    lastUpdated: { text: '最后更新' },
    returnToTopLabel: '返回顶部',
    sidebarMenuLabel: '菜单',
    darkModeSwitchLabel: '主题',
    lightModeSwitchTitle: '切换到浅色模式',
    darkModeSwitchTitle: '切换到深色模式'
  }
})
