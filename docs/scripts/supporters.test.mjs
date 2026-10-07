import { readFileSync, existsSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const data = JSON.parse(readFileSync(new URL('../.vitepress/theme/data/supporters.json', import.meta.url), 'utf8'))

describe('股东名册', () => {
  it('公开数据只包含展示字段，不包含账户、订单、金额或留言', () => {
    expect(Object.keys(data).sort()).toEqual(['period', 'supporters', 'updatedAt'])
    expect(Object.keys(data.period).sort()).toEqual(['from', 'to'])
    for (const person of data.supporters) {
      expect(Object.keys(person).sort()).toEqual(['count', 'id', 'name', 'since'])
      expect(person.name.trim()).not.toBe('')
      expect(Number.isInteger(person.count) && person.count > 0).toBe(true)
      expect(person.since).toMatch(/^\d{4}-(0[1-9]|1[0-2])$/)
    }
  })

  it('独立展示同名账号，首次支持顺序和总数保持一致', () => {
    expect(new Set(data.supporters.map(person => person.id)).size).toBe(data.supporters.length)
    const months = data.supporters.map(person => person.since)
    expect(months).toEqual([...months].sort())
    expect(data.supporters.reduce((sum, person) => sum + person.count, 0)).toBeGreaterThanOrEqual(data.supporters.length)
    expect(data.period.from <= data.period.to && data.period.to <= data.updatedAt).toBe(true)
  })

  it('感谢页使用的角色素材存在', () => {
    for (const path of ['dragon/base.svg', 'niang/stickers/14-thanks.png']) {
      expect(existsSync(new URL(`../public/mascots/${path}`, import.meta.url))).toBe(true)
    }
  })
})
