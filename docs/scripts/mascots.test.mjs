import { readFileSync, existsSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const root = resolve(import.meta.dirname, '../public/mascots')
const manifest = JSON.parse(readFileSync(`${root}/stickers.json`, 'utf8'))

describe('奶屁角色素材契约', () => {
  it('只有两位角色，各 16 张独立透明 PNG，尺寸与大小符合发布规格', () => {
    expect(manifest).toHaveLength(32)
    expect(new Set(manifest.map(entry => entry.character))).toEqual(new Set(['dragon', 'niang']))
    expect(new Set(manifest.map(entry => entry.file)).size).toBe(32)
    for (const character of ['dragon', 'niang']) {
      expect(manifest.filter(entry => entry.character === character)).toHaveLength(16)
    }
    for (const entry of manifest) {
      const png = readFileSync(`${root}/${entry.file}`)
      expect(png.subarray(0, 8).toString('hex')).toBe('89504e470d0a1a0a')
      expect(png.readUInt32BE(16)).toBe(512)
      expect(png.readUInt32BE(20)).toBe(512)
      expect(png[25]).toBe(6)
      expect(png.length).toBeLessThanOrEqual(512 * 1024)
    }
  })

  it('奶屁龙母版为独立路径图层，不嵌入位图，所有表情均有矢量源', () => {
    const base = readFileSync(`${root}/dragon/base.svg`, 'utf8')
    expect(base).not.toMatch(/<image\b|data:image|<foreignObject\b/i)
    expect(base).toContain('shape-rendering="crispEdges"')
    for (const id of ['body', 'tail', 'spines', 'horns', 'forehead', 'belly', 'feet', 'paw-left', 'paw-right', 'eye-left', 'eye-right', 'mouth', 'face-underlay']) {
      expect(base).toContain(`id="${id}"`)
    }
    for (const entry of manifest.filter(entry => entry.character === 'dragon')) {
      const svg = readFileSync(`${root}/${entry.vector}`, 'utf8')
      expect(svg).not.toMatch(/<image\b|data:image/i)
      expect(svg).toContain(entry.caption)
      expect(svg).toContain('viewBox="0 0 512 512"')
    }
  })

  it('运行态样张保持完整静态图形，文档 HTML 素材链接真实存在', () => {
    for (const state of ['dig-raised', 'dig-strike', 'sleep-a', 'sleep-b', 'failed-a', 'failed-b', 'waiting-a', 'waiting-b']) {
      const svg = readFileSync(`${root}/dragon/${state}.svg`, 'utf8')
      expect(svg).not.toContain('@keyframes')
      expect(svg).not.toContain('animation:')
    }
    for (const page of ['index', 'dragon', 'niang', 'stickers', 'production']) {
      const markdown = readFileSync(resolve(import.meta.dirname, `../mascot/${page}.md`), 'utf8')
      for (const match of markdown.matchAll(/(?:src|href|data)="\/mascots\/([^"#?]+)"/g)) {
        expect(existsSync(`${root}/${match[1]}`), `${page}: ${match[1]}`).toBe(true)
      }
    }
    expect(readFileSync(resolve(import.meta.dirname, '../mascot/stickers.md'), 'utf8').match(/<figure>/g)).toHaveLength(32)
    expect(existsSync(`${root}/naipi-stickers.zip`)).toBe(true)
  })
})
