import { describe, expect, it } from 'vitest'
import {
  changeListMode,
  makeSimpleRule,
  replaceRule,
  simpleRule,
  ssidSchema,
} from './management-data'

describe('policy and rule editing', () => {
  it('turns off a policy without clearing either list or changing its mode', () => {
    const before = {
      enabled: true,
      mode: 'whitelist' as const,
      proxy_apps: ['10:com.example.app'],
      bypass_apps: ['0:com.termux'],
    }
    const off = changeListMode(before, 'off')
    expect(off).toEqual({ ...before, enabled: false })
    expect(changeListMode(off, 'blacklist')).toEqual({
      ...before,
      enabled: true,
      mode: 'blacklist',
    })
  })
  it('keeps SSID spaces, commas, case and valid Unicode', () => {
    for (const value of [' Name, WiFi ', '无线网络', 'Guest 🐉'])
      expect(ssidSchema.parse(value)).toBe(value)
    for (const value of ['', '\ud800', 'x\n', 'a'.repeat(33)])
      expect(ssidSchema.safeParse(value).success).toBe(false)
  })
  it('does not split or flatten advanced rules', () => {
    for (const rule of [
      { domain: ['example.invalid'], port: [443] },
      { type: 'logical', mode: 'or', rules: [] },
      { domain: ['example.invalid'], invert: true },
    ])
      expect(simpleRule(rule)).toBeUndefined()
    const logical = { type: 'logical', mode: 'or', rules: [{ port: [443] }] }
    const original = {
      version: 3,
      rules: [logical, { domain: ['example.invalid'] }],
      extra: 'preserved',
    }
    const next = replaceRule(original, 1, makeSimpleRule('ip_cidr', '192.168.10.0/24'))
    expect(next.rules[0]).toBe(logical)
    expect(next.rules[1]).toEqual({ ip_cidr: ['192.168.10.0/24'] })
    expect(next.extra).toBe('preserved')
    expect(original.rules[1]).toEqual({ domain: ['example.invalid'] })
  })
  it('uses numeric ports and preserves Android package matching', () => {
    expect(makeSimpleRule('port', '443\n443\n853')).toEqual({ port: [443, 853] })
    expect(makeSimpleRule('port', '65536')).toBeUndefined()
    expect(makeSimpleRule('port_range', '443:1')).toBeUndefined()
    expect(makeSimpleRule('package_name', 'com.termux')).toEqual({ package_name: ['com.termux'] })
  })
})
