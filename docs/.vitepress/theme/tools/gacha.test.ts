import { describe, expect, it } from 'vitest'
import { pull, rollOne } from './gacha'

describe('祈愿机制', () => {
  it('第 80 抽触发 5 星保底并重置两个计数', () => {
    const result = rollOne({ pity5: 79, pity4: 4 }, () => 0)
    expect(result.card.rarity).toBe(5)
    expect(result.pity).toEqual({ pity5: 0, pity4: 0 })
  })

  it('第 10 抽触发 4 星保底', () => {
    const result = rollOne({ pity5: 2, pity4: 9 }, () => 0.5)
    expect(result.card.rarity).toBe(4)
    expect(result.pity).toEqual({ pity5: 3, pity4: 0 })
  })

  it('保持 1.6% 和 8.4% 的概率边界', () => {
    const five = [0.015, 0].values()
    expect(rollOne({ pity5: 0, pity4: 0 }, () => five.next().value ?? 0).card.rarity).toBe(5)
    const four = [0.05, 0].values()
    expect(rollOne({ pity5: 0, pity4: 0 }, () => four.next().value ?? 0).card.rarity).toBe(4)
    const three = [0.5, 0, 0, 0].values()
    expect(rollOne({ pity5: 0, pity4: 0 }, () => three.next().value ?? 0).card.rarity).toBe(3)
  })

  it('十连完整推进保底状态', () => {
    const result = pull(10, { pity5: 0, pity4: 0 }, () => 0.99)
    expect(result.cards).toHaveLength(10)
    expect(result.cards[9].rarity).toBe(4)
    expect(result.pity).toEqual({ pity5: 10, pity4: 0 })
  })
})
