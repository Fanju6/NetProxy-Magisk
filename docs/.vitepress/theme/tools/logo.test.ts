import { describe, expect, it } from 'vitest'
import { defaultLogoOptions, generateSvg, generateVector, validateLogoOptions } from './logo'

describe('Logo 导出', () => {
  it('生成 SVG 与 Android Vector XML', () => {
    expect(generateSvg(defaultLogoOptions)).toContain('linearGradient id="diagGrad"')
    expect(generateSvg(defaultLogoOptions)).toContain('stroke-width="88"')
    expect(generateVector(defaultLogoOptions)).toContain('<vector xmlns:android=')
    expect(generateVector(defaultLogoOptions)).toContain('android:strokeAlpha="0.85"')
  })

  it('拒绝无效颜色、线宽、透明度和端点', () => {
    expect(() => validateLogoOptions({ ...defaultLogoOptions, leftColor: 'red' })).toThrow(/Hex/)
    expect(() => validateLogoOptions({ ...defaultLogoOptions, strokeWidth: 0 })).toThrow(/线宽/)
    expect(() => validateLogoOptions({ ...defaultLogoOptions, opacity: 2 })).toThrow(/透明度/)
    expect(() => validateLogoOptions({ ...defaultLogoOptions, strokeLinecap: 'bad' as never })).toThrow(/端点/)
  })
})
