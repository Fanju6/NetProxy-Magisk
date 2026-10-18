import { describe, expect, it } from 'vitest'
import {
  readPolicyObject,
  replacePolicyObject,
  setPolicyProperty,
  outboundsDocumentSchema,
} from './policy-config'
import { policySnapshotSchema, serviceSchema } from './contracts'
import { createFixtures } from '@/mocks/fixtures'

describe('native policy objects', () => {
  const first = {
    type: 'selector',
    tag: 'A',
    providers: ['Pool / one'],
    unknown: { nested: [1, false] },
  }
  const second = {
    type: 'urltest',
    tag: 'B',
    outbounds: ['A'],
    tolerance: 50,
    url: 'https://example.invalid',
    future: true,
  }
  const content = JSON.stringify({ outbounds: [first, second], future_partition: true })
  it('preserves untagged outbounds, whitespace, unknown fields and large integer literals verbatim', () => {
    const target =
      '{ "type": "selector", "tag": "Native / 组", "outbounds": ["direct"], "future": {"integer":9007199254740993,"exponent":1e+30} }'
    const unrelated = '{ "type": "direct", "future": 18446744073709551615 }'
    const before =
      '{\n "outbounds" : [\n  ' +
      unrelated +
      ',\n  ' +
      target +
      '\n ],\n "unknown":9007199254740995\n}'
    expect(outboundsDocumentSchema.safeParse(JSON.parse(before)).success).toBe(true)
    expect(readPolicyObject(before, 'Native / 组')).toBe(target)
    const edited = setPolicyProperty(target, 'outbounds', ['direct', 'nested'])
    const result = replacePolicyObject(before, 'Native / 组', edited)
    expect(result).toBe(before.replace(target, target.replace('["direct"]', '["direct","nested"]')))
    expect(result).toContain('9007199254740993')
    expect(result).toContain(unrelated)
    expect(result).toContain('1e+30')
  })
  it.each(['first', 'middle', 'last', 'only'] as const)(
    'removes the %s object without reserializing siblings',
    (position) => {
      const target = '{"type":"selector","tag":"A","unknown":9007199254740993}'
      const raw = '{ "type" : "direct", "unknown" : 18446744073709551615 }'
      const values =
        position === 'first'
          ? [target, raw]
          : position === 'last'
            ? [raw, target]
            : position === 'middle'
              ? [raw, target, raw]
              : [target]
      const result = replacePolicyObject(
        '{ "outbounds" : [ ' + values.join(',\n') + ' ] }',
        'A',
        null,
      )
      expect(JSON.parse(result).outbounds).toHaveLength(values.length - 1)
      expect(result.split(raw)).toHaveLength(values.filter((value) => value === raw).length + 1)
    },
  )
  it('updates or removes only one property and preserves target unknown raw values', () => {
    const raw = '{ "tag":"A", "type":"selector", "unknown":9007199254740993 }'
    const added = setPolicyProperty(raw, 'providers', ['np/pool'])
    expect(added).toContain('"unknown":9007199254740993')
    expect(JSON.parse(added).providers).toEqual(['np/pool'])
    const removed = setPolicyProperty(added, 'providers', undefined)
    expect(removed).toContain('"unknown":9007199254740993')
    expect(JSON.parse(removed)).not.toHaveProperty('providers')
  })
  it('rejects invalid JSON, ambiguous targets and invalid replacement identities', () => {
    for (const source of [
      '{"outbounds":[}',
      '{"outbounds":[],"outbounds":[]}',
      '{"outbounds":[{"type":"selector","tag":"A"},{"type":"selector","tag":"A"}]}',
    ])
      expect(() => readPolicyObject(source, 'A')).toThrow()
    expect(() => replacePolicyObject(content, 'A', '{"type":"selector"}')).toThrow()
    expect(() =>
      replacePolicyObject(content, 'A', '{"tag":"A","tag":"B","type":"selector"}'),
    ).toThrow()
  })
  it('replaces only the target and retains unknown fields and ordering', () => {
    expect(JSON.parse(replacePolicyObject(content, 'A', { ...first, default: 'direct' }))).toEqual({
      outbounds: [{ ...first, default: 'direct' }, second],
      future_partition: true,
    })
  })
  it('does not rewrite references when deleting or renaming', () => {
    expect(JSON.parse(replacePolicyObject(content, 'A', null)).outbounds).toEqual([second])
    expect(
      JSON.parse(replacePolicyObject(content, 'A', { ...first, tag: 'Renamed' })).outbounds,
    ).toEqual([{ ...first, tag: 'Renamed' }, second])
  })
  it('adds native objects without a private group representation', () => {
    const added = {
      type: 'urltest',
      tag: 'New',
      use_all_providers: true,
      interval: '3m',
      tolerance: 100,
    }
    expect(JSON.parse(replacePolicyObject(content, null, added)).outbounds).toEqual([
      first,
      second,
      added,
    ])
  })
  it('rejects missing and duplicate targets', () => {
    expect(() => replacePolicyObject(content, 'missing', first)).toThrow()
    expect(() => replacePolicyObject(content, null, first)).toThrow()
  })
  it('requires the locked snapshot and new service fields, without Catalog selection', () => {
    const fixture = createFixtures()
    expect(policySnapshotSchema.safeParse(fixture.policy).success).toBe(true)
    expect(policySnapshotSchema.safeParse(fixture.snapshot).success).toBe(false)
    expect(
      policySnapshotSchema.safeParse({ ...fixture.policy, core_started_at_millis: 0 }).success,
    ).toBe(false)
    const { default_outbound: _default, ...oldService } = fixture.service
    void _default
    expect(serviceSchema.safeParse(oldService).success).toBe(false)
  })
})
