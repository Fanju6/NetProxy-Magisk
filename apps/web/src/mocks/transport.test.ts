import { afterEach, describe, expect, it, vi } from 'vitest'
import { z } from 'zod'
import { ApiClient } from '@/api/client'
import {
  configSchema,
  envelopeSchema,
  groupSchema,
  infoSchema,
  serviceSchema,
  snapshotSchema,
  policySnapshotSchema,
  trafficSchema,
} from '@/api/contracts'
import { MockTransport } from './transport'
import { createFixtures } from './fixtures'

afterEach(() => vi.useRealTimers())
describe('Go API shaped mock', () => {
  it('creates, renames and removes only local groups with stable provider identity', async () => {
    const client = new ApiClient(new MockTransport(0))
    const shape = z.object({ id: z.literal('fixture-local') })
    expect(
      await client.request(
        { method: 'POST', path: '/catalog', body: { id: 'fixture-local', name: '旅行' } },
        shape,
      ),
    ).toEqual({ id: 'fixture-local' })
    await client.request(
      { method: 'PUT', path: '/catalog/fixture-local', body: { name: 'Renamed' } },
      shape,
    )
    const groups = await client.request(
      { method: 'GET', path: '/subscriptions' },
      z.array(groupSchema),
    )
    expect(groups.find((group) => group.id === 'fixture-local')).toMatchObject({
      name: 'Renamed',
      provider_tag: 'np/fixture-local',
      type: 'local',
    })
    await expect(
      client.request({ method: 'DELETE', path: '/catalog/demo-global' }, z.unknown()),
    ).rejects.toMatchObject({ code: 'catalog.read_only' })
    await client.request({ method: 'DELETE', path: '/catalog/fixture-local' }, shape)
    expect(
      (await client.request({ method: 'GET', path: '/subscriptions' }, z.array(groupSchema))).some(
        (group) => group.id === 'fixture-local',
      ),
    ).toBe(false)
    await expect(
      client.request({ method: 'DELETE', path: '/catalog/default' }, z.unknown()),
    ).rejects.toBeDefined()
  })
  it('validates all first-batch read responses against the source-derived DTOs', async () => {
    const client = new ApiClient(new MockTransport(0))
    expect(await client.request({ method: 'GET', path: '/info' }, infoSchema)).toMatchObject({
      api_version: 1,
      version: 'v8.6.1',
    })
    expect(await client.request({ method: 'GET', path: '/service' }, serviceSchema)).toMatchObject({
      state: 'ready',
    })
    const snapshot = await client.request(
      { method: 'GET', path: '/nodes/snapshot' },
      snapshotSchema,
    )
    const subscriptions = await client.request(
      { method: 'GET', path: '/subscriptions' },
      z.array(groupSchema),
    )
    const catalog = await client.request({ method: 'GET', path: '/catalog' }, snapshotSchema)
    expect(catalog).toEqual(snapshot)
    expect(catalog.groups.map(({ group }) => group)).toEqual(subscriptions)
    expect(subscriptions.some((group) => group.id === 'default' && group.type === 'local')).toBe(
      true,
    )
    expect(snapshot.groups.find((item) => item.group.id === 'demo-empty')?.nodes).toEqual([])
  })
  it('keeps Catalog independent and readable while native groups are not live', async () => {
    const client = new ApiClient(new MockTransport(0))
    await client.request({ method: 'POST', path: '/service/stop' }, z.unknown())
    expect(
      (await client.request({ method: 'GET', path: '/subscriptions' }, z.array(groupSchema)))
        .length,
    ).toBe(4)
    const catalog = await client.request({ method: 'GET', path: '/nodes/snapshot' }, snapshotSchema)
    expect(catalog).not.toHaveProperty('selection')
    const policy = await client.request(
      { method: 'GET', path: '/policy-groups' },
      policySnapshotSchema,
    )
    expect(policy).toMatchObject({
      live: false,
      core_started_at_millis: 0,
      default_outbound: 'Internet',
    })
    await expect(
      client.request(
        {
          method: 'POST',
          path: '/policy-groups/select',
          body: { group_tag: 'Internet', member_tag: 'Fastest' },
        },
        policySnapshotSchema,
      ),
    ).rejects.toMatchObject({ code: 'policy.not_live' })
  })
  it('uses complete snapshots for native group selection and delay without changing Catalog', async () => {
    const client = new ApiClient(new MockTransport(0))
    const catalog = await client.request({ method: 'GET', path: '/catalog' }, snapshotSchema)
    const selected = await client.request(
      {
        method: 'POST',
        path: '/policy-groups/select',
        body: { group_tag: 'Internet', member_tag: 'Hong Kong 01' },
      },
      policySnapshotSchema,
    )
    expect(selected.groups[0]!.selected).toBe('Hong Kong 01')
    expect(await client.request({ method: 'GET', path: '/catalog' }, snapshotSchema)).toEqual(
      catalog,
    )
    const measured = await client.request(
      { method: 'POST', path: '/policy-groups/delay', body: { group_tag: 'Fastest' } },
      policySnapshotSchema,
    )
    expect(measured.groups[1]!.items[0]!.url_test_time).toBeGreaterThan(1_000_000_000)
    expect(measured).not.toHaveProperty('selection')
  })
  it('requires the read revision and does not overwrite on conflict', async () => {
    const client = new ApiClient(new MockTransport(0))
    const query = { target: 'module/auto_start' }
    const before = await client.request({ method: 'GET', path: '/config', query }, configSchema)
    await client.request(
      {
        method: 'PUT',
        path: '/config',
        query,
        body: { revision: before.revision, content: '{"auto_start":true}' },
      },
      z.unknown(),
    )
    await expect(
      client.request(
        {
          method: 'PUT',
          path: '/config',
          query,
          body: { revision: before.revision, content: '{"auto_start":false}' },
        },
        z.unknown(),
      ),
    ).rejects.toMatchObject({ code: 'config.conflict', status: 409 })
    expect(
      (await client.request({ method: 'GET', path: '/config', query }, configSchema)).content,
    ).toBe('{"auto_start":true}')
  })
  it('returns detached snapshots rather than exposing server state for mutation', async () => {
    const mock = new MockTransport(0)
    const first = await new ApiClient(mock).request(
      { method: 'GET', path: '/service' },
      serviceSchema,
    )
    first.state = 'failed'
    expect(
      (await new ApiClient(mock).request({ method: 'GET', path: '/service' }, serviceSchema)).state,
    ).toBe('ready')
  })
  it('cancels reads but completes an accepted write after client cancellation', async () => {
    const mock = new MockTransport(5)
    const read = new AbortController()
    const pendingRead = mock.request({ method: 'GET', path: '/service' }, read.signal)
    read.abort()
    await expect(pendingRead).rejects.toMatchObject({ name: 'AbortError' })
    const write = new AbortController()
    const pendingWrite = mock.request({ method: 'POST', path: '/service/stop' }, write.signal)
    write.abort()
    expect((await pendingWrite).status).toBe(200)
  })
  it('sends complete snapshots first and stops work when unsubscribed', async () => {
    vi.useFakeTimers()
    const mock = new MockTransport()
    const consume = vi.fn()
    const stop = mock.subscribe(['status', 'groups', 'subscriptions'], consume)
    await Promise.resolve()
    expect(consume.mock.calls.map((call) => call[0].name)).toEqual([
      'status',
      'groups',
      'subscriptions',
      'traffic',
    ])
    for (const [event] of consume.mock.calls) expect(envelopeSchema.parse(event.body).ok).toBe(true)
    expect(
      trafficSchema.safeParse(envelopeSchema.parse(consume.mock.calls[3]![0].body).data).success,
    ).toBe(true)
    stop()
    const count = consume.mock.calls.length
    await vi.advanceTimersByTimeAsync(3000)
    expect(consume).toHaveBeenCalledTimes(count)
    expect(vi.getTimerCount()).toBe(0)
  })
  it('keeps deterministic fixtures independent across sessions', () => {
    const first = createFixtures()
    first.snapshot.groups.splice(0)
    expect(createFixtures().snapshot.groups).toHaveLength(4)
  })
  it('filters pure Catalog snapshots without inventing a selection', async () => {
    const client = new ApiClient(new MockTransport(0))
    const filtered = await client.request(
      { method: 'GET', path: '/nodes/snapshot', query: { group_id: 'demo-empty' } },
      snapshotSchema,
    )
    expect(filtered.groups.map((item) => item.group.id)).toEqual(['demo-empty'])
    expect(filtered).not.toHaveProperty('selection')
    await expect(
      client.request(
        { method: 'GET', path: '/nodes/snapshot', query: { group_id: '../private' } },
        snapshotSchema,
      ),
    ).rejects.toMatchObject({ code: 'usage.invalid', status: 400 })
  })
  it('rejects invalid request fields and modes with the current Go error codes', async () => {
    const client = new ApiClient(new MockTransport(0))
    for (const request of [
      { method: 'GET', path: '/service', query: { unknown: 'value' } },
      { method: 'POST', path: '/policy-groups/select', body: { group_tag: 'Internet' } },
    ] as const) {
      await expect(client.request(request, z.unknown())).rejects.toMatchObject({
        code: 'usage.invalid',
        status: 400,
      })
    }
    await expect(
      client.request(
        { method: 'PUT', path: '/mode', body: { mode: 'invented-mode' } },
        z.unknown(),
      ),
    ).rejects.toMatchObject({ code: 'mode.invalid', status: 422 })
  })
})
