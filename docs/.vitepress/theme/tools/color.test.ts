import { describe, expect, it } from 'vitest'
import { createMonetScheme, hexToHsl, hslToHex, normalizeHex } from './color'

describe('颜色工具', () => {
  it('规范化三位和六位 Hex', () => {
    expect(normalizeHex('#0af')).toBe('#00AAFF')
    expect(normalizeHex('12abEF')).toBe('#12ABEF')
    expect(normalizeHex('not-a-color')).toBeNull()
  })

  it('在 Hex 和 HSL 之间稳定转换', () => {
    expect(hexToHsl('#FF0000')).toEqual({ h: 0, s: 100, l: 50 })
    expect(hslToHex(120, 100, 50)).toBe('#00FF00')
    expect(hslToHex(-240, 100, 50)).toBe('#00FF00')
  })

  it('从种子色生成完整且有效的方案', () => {
    const scheme = createMonetScheme('#003BCC')
    expect(Object.values(scheme)).toHaveLength(6)
    expect(Object.values(scheme).every((value) => /^#[0-9A-F]{6}$/.test(value))).toBe(true)
  })
})
