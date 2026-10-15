import { describe, expect, it } from 'vitest'
import { acceptTraffic } from './events'
import { createFixtures, demoTraffic } from '@/mocks/fixtures'

describe('core identity gate', () => {
  it('accepts only matching ready-core traffic after a full status snapshot', () => {
    const { service } = createFixtures()
    expect(acceptTraffic(undefined, demoTraffic(service))).toBe(false)
    expect(acceptTraffic(service, demoTraffic(service))).toBe(true)
    expect(acceptTraffic({ ...service, core_started_at_millis: 1 }, demoTraffic(service))).toBe(
      false,
    )
    expect(acceptTraffic({ ...service, state: 'stopped' }, demoTraffic(service))).toBe(false)
    expect(acceptTraffic(service, { available: false })).toBe(false)
  })
})
