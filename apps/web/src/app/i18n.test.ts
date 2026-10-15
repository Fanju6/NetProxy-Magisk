import { expect, it } from 'vitest'
import { en, zh, translator } from './i18n'
import { bytes, displayNode } from '@/lib/format'

it('has complete Chinese and English translations', () => {
  expect(Object.keys(zh).sort()).toEqual(Object.keys(en).sort())
  expect(translator('en')('nodeCount', { count: 7 })).toBe('7 nodes')
  expect(translator('zh')('selectNode', { name: 'Tokyo 01' })).toBe('选择 Tokyo 01')
})
it('preserves slashes in node tags and formats zero safely', () => {
  expect(displayNode('group/node/with/slash')).toBe('node/with/slash')
  expect(bytes(0)).toBe('0 B')
  expect(bytes(1024)).toBe('1 KB')
})
