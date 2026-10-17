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
  trafficSchema,
} from '@/api/contracts'
import { MockTransport } from './transport'
import { createFixtures } from './fixtures'

afterEach(() => vi.useRealTimers())
describe('Go API shaped mock', () => {
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
    expect(subscriptions.some((group) => group.id === 'default' && group.type === 'local')).toBe(
      true,
    )
    expect(snapshot.groups.find((item) => item.group.id === 'demo-empty')?.nodes).toEqual([])
  })
  it('keeps Catalog readable after stopping and changes selection while stopped', async () => {
    const client = new ApiClient(new MockTransport(0))
    const result = await client.request(
      { method: 'POST', path: '/service/stop' },
      z.object({ action: z.literal('stop'), status: serviceSchema }),
    )
    expect(result.status).toMatchObject({
      state: 'stopped',
      active_backend: null,
      pid: null,
      core_started_at_millis: 0,
    })
    expect(
      (await client.request({ method: 'GET', path: '/subscriptions' }, z.array(groupSchema)))
        .length,
    ).toBe(4)
    await client.request(
      { method: 'PUT', path: '/selection', body: { group_id: 'default', node_tag: 'Local relay' } },
      z.unknown(),
    )
    const snapshot = await client.request(
      { method: 'GET', path: '/nodes/snapshot' },
      snapshotSchema,
    )
    expect(snapshot.selection).toMatchObject({
      selected_node_ref: 'default/Local relay',
      runtime_selected: '',
      selector_mode: 'manual',
    })
    await client.request(
      { method: 'PUT', path: '/selection', body: { group_id: 'default', node_tag: '' } },
      z.unknown(),
    )
    expect(
      (await client.request({ method: 'GET', path: '/nodes/snapshot' }, snapshotSchema)).selection,
    ).toMatchObject({ selected: 'Auto/本地配置', selected_node_ref: '', selector_mode: 'urltest' })
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
  it('filters group snapshots without changing the global selection', async () => {
    const client = new ApiClient(new MockTransport(0))
    const filtered = await client.request(
      { method: 'GET', path: '/nodes/snapshot', query: { group_id: 'demo-empty' } },
      snapshotSchema,
    )
    expect(filtered.groups.map((item) => item.group.id)).toEqual(['demo-empty'])
    expect(filtered.selection.active_group_id).not.toBe('demo-empty')
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
      { method: 'PUT', path: '/selection', body: { group_id: 'default' } },
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
