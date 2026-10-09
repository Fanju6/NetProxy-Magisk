import { existsSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import config from '../.vitepress/config.mts'

function pageHead(relativePath, title, description = '') {
  const pageData = { relativePath, title, description, frontmatter: {} }
  config.transformPageData(pageData, { siteConfig: { site: config } })
  return pageData.frontmatter.head
}

describe('页面元数据', () => {
  it.each([
    ['index.md', 'https://www.netproxy.store/'],
    ['statistics.md', 'https://www.netproxy.store/statistics'],
    ['config/module.md', 'https://www.netproxy.store/config/module'],
    ['config/tun.md', 'https://www.netproxy.store/config/tun'],
    ['mascot/index.md', 'https://www.netproxy.store/mascot/']
  ])('%s 使用独立规范地址', (path, url) => {
    const head = pageHead(path, '页面标题')
    expect(head).toContainEqual(['link', { rel: 'canonical', href: url }])
    expect(head).toContainEqual(['meta', { property: 'og:url', content: url }])
  })

  it('分享信息使用页面标题与描述', () => {
    const head = pageHead('statistics.md', '设备统计', '设备活跃与版本分布')
    expect(head).toContainEqual(['meta', { property: 'og:title', content: '设备统计' }])
    expect(head).toContainEqual(['meta', { property: 'og:description', content: '设备活跃与版本分布' }])
  })

  it('首页缺省分享标题使用站点名', () => {
    expect(pageHead('index.md', '')).toContainEqual(['meta', { property: 'og:title', content: 'NetProxy' }])
  })

  it('缺省描述使用站点介绍，并保留页面其他 head 项', () => {
    const existing = ['meta', { name: 'robots', content: 'noindex' }]
    const pageData = { relativePath: 'guide/cli.md', title: 'CLI', description: '', frontmatter: { head: [existing] } }
    config.transformPageData(pageData, { siteConfig: { site: config } })
    expect(pageData.frontmatter.head).toContainEqual(existing)
    expect(pageData.frontmatter.head).toContainEqual(['meta', { property: 'og:description', content: config.description }])
  })
})

describe('文档导航', () => {
  it('顶部保留四个文字入口与一个 GitHub 图标', () => {
    expect(config.themeConfig.nav.map(item => item.text)).toEqual(['开始使用', '配置参考', '更新日志', '更多'])
    expect(config.themeConfig.socialLinks).toEqual([{ icon: 'github', link: 'https://github.com/Fanju6/NetProxy-Magisk' }])
  })

  it('更多菜单按三组组织，所有导航指向已有页面', () => {
    expect(config.themeConfig.nav.at(-1).items.map(item => item.text)).toEqual(['项目', '奶屁伙伴', '互动工具'])
    function check(items) {
      for (const item of items) {
        if (item.items) check(item.items)
        if (!item.link) continue
        const path = resolve(import.meta.dirname, '..', item.link.slice(1))
        expect(existsSync(`${path}.md`) || existsSync(resolve(path, 'index.md')), item.link).toBe(true)
      }
    }
    check(config.themeConfig.nav)
  })
})
