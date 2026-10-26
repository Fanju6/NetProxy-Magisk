import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiContext } from '@/app/context'
import { createFixtures, demoHistory } from '@/mocks/fixtures'
import { MockTransport } from '@/mocks/transport'
import { ApiClient } from './client'
import {
  groupSchema,
  historySchema,
  subscriptionProgressSchema,
  type ApiTransport,
} from './contracts'
import {
  acceptResourceEvent,
  catalogNodesKey,
  mergeResourceSummaries,
  resetResourceSession,
  resourceQueries,
  useResourceDetails,
} from './resources'

const clients: QueryClient[] = []
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((cache) => cache.clear())
  vi.useRealTimers()
})
const response = (data: unknown) => ({
  status: 200,
  body: { schema: 1, ok: true, code: 'fixture.read', message: '', data },
})
function setup() {
  const backend = new MockTransport(0)
  const request = vi.fn<ApiTransport['request']>((input, signal) => backend.request(input, signal))
  const transport: ApiTransport = {
    kind: 'native',
    capabilities: backend.capabilities,
    request,
    subscribe: () => () => {},
  }
  const api = new ApiClient(transport)
  const cache = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity, staleTime: 0 } },
  })
  clients.push(cache)
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <ApiContext value={api}>
        <QueryClientProvider client={cache}>{children}</QueryClientProvider>
      </ApiContext>
    )
  }
  return { api, cache, request, Wrapper, queries: resourceQueries(api) }
}
const summaries = () => createFixtures().snapshot.groups.map(({ group }) => group)
const catalogResponse = (groups: ReturnType<typeof summaries>) =>
  response({ groups: groups.map((group) => ({ group, nodes: [] })) })

describe('shared public Catalog nodes', () => {
  it('shares concurrent observers and remounts for 30 seconds without private fields', async () => {
    const h = setup()
    const snapshot = createFixtures().snapshot
    h.request.mockResolvedValue(
      response({
        ...snapshot,
        groups: snapshot.groups.map((entry) => ({ ...entry, private_key: 'FAKE_PRIVATE' })),
      }),
    )
    const first = renderHook(() => useQuery(h.queries.nodes), { wrapper: h.Wrapper })
    const second = renderHook(() => useQuery(h.queries.nodes), { wrapper: h.Wrapper })
    await waitFor(() => expect(first.result.current.isSuccess).toBe(true))
    expect(second.result.current.data).toEqual(first.result.current.data)
    expect(h.request).toHaveBeenCalledTimes(1)
    first.unmount()
    second.unmount()
    const again = renderHook(() => useQuery(h.queries.nodes), { wrapper: h.Wrapper })
    await waitFor(() => expect(again.result.current.isSuccess).toBe(true))
    expect(h.request).toHaveBeenCalledTimes(1)
    expect(h.cache.getQueryCache().findAll({ queryKey: catalogNodesKey })).toHaveLength(1)
    expect(JSON.stringify(again.result.current.data)).not.toContain('FAKE_PRIVATE')
    h.cache.setQueryData(catalogNodesKey, again.result.current.data, {
      updatedAt: Date.now() - 30_001,
    })
    await h.cache.fetchQuery(h.queries.nodes)
    expect(h.request).toHaveBeenCalledTimes(2)
  })

  it.each(['initial', 'rename', 'revision', 'count', 'add', 'remove'])(
    'invalidates the shared snapshot on %s events',
    async (change) => {
      const h = setup()
      const previous = summaries()
      h.cache.setQueryData(['subscriptions'], previous)
      h.cache.setQueryData(catalogNodesKey, { groups: [] })
      const incoming = previous.map((group) => ({ ...group }))
      if (change === 'rename') incoming[0]!.name += ' renamed'
      if (change === 'revision') incoming[0]!.revision++
      if (change === 'count') incoming[0]!.node_count++
      if (change === 'remove') incoming.pop()
      if (change === 'add') incoming.push({ ...previous[0]!, id: 'new-resource' })
      acceptResourceEvent(h.cache, incoming, change === 'initial')
      await act(async () => {})
      expect(h.cache.getQueryState(catalogNodesKey)?.isInvalidated).toBe(true)
    },
  )

  it('cancels old session reads and invalidates otherwise fresh cached nodes', async () => {
    const h = setup()
    let finish!: (value: ReturnType<typeof response>) => void
    h.request.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    const read = h.cache.fetchQuery(h.queries.nodes).catch(() => undefined)
    resetResourceSession(h.cache)
    expect(h.request.mock.calls[0]?.[1]?.aborted).toBe(true)
    finish(response(createFixtures().snapshot))
    await read
    expect(h.cache.getQueryData(catalogNodesKey)).toBeUndefined()
    h.cache.setQueryData(catalogNodesKey, { groups: [] })
    resetResourceSession(h.cache)
    expect(h.cache.getQueryState(catalogNodesKey)?.isInvalidated).toBe(true)
  })
})

describe('public Catalog contracts', () => {
  it('reads the Go Catalog snapshot as resource summaries, not a subscriptions array', async () => {
    const h = setup()
    h.request.mockResolvedValue(response(createFixtures().snapshot))
    await expect(h.cache.fetchQuery(h.queries.catalog)).resolves.toEqual(summaries())
    expect(h.request).toHaveBeenCalledWith(
      { method: 'GET', path: '/catalog' },
      expect.any(AbortSignal),
    )
    expect(h.cache.getQueryData(['subscriptions'])).toEqual(summaries())
  })

  it('uses actual Go types, optional traffic counters, sparse history and active-only progress', () => {
    const list = summaries()
    expect(list.filter((group) => group.type === 'local')).toHaveLength(2)
    expect(list.filter((group) => group.type === 'subscription')).toHaveLength(2)
    list.forEach((group) => expect(groupSchema.safeParse(group).success).toBe(true))
    expect(groupSchema.safeParse({ ...list[0], type: 'url' }).success).toBe(false)
    expect(groupSchema.safeParse({ ...list[0], usage: { expire: '100' } }).success).toBe(false)
    expect(historySchema.parse(demoHistory('demo-global'))).toHaveLength(3)
    expect(
      historySchema.safeParse([{ at: 'bad', ok: true, code: 'subscription.updated' }]).success,
    ).toBe(false)
    expect(
      subscriptionProgressSchema.safeParse({
        id: 'default',
        progress: {
          schema: 1,
          group_id: 'default',
          stage: 'success',
          message: '',
          updated_at: '2026-10-17T00:00:00Z',
        },
      }).success,
    ).toBe(false)
    const stripped = groupSchema.parse({
      ...list[0],
      url: 'private',
      hwid: 'private',
      custom_headers: { Authorization: 'private' },
    })
    expect(JSON.stringify(stripped)).not.toContain('private')
  })
  it('reads every public resource with fixed GETs and keeps local and subscription summaries', async () => {
    const h = setup()
    expect(await h.cache.fetchQuery(h.queries.catalog)).toHaveLength(4)
    expect(await h.cache.fetchQuery({ ...h.queries.subscriptions, staleTime: 0 })).toHaveLength(4)
    expect((await h.cache.fetchQuery(h.queries.detail('default'))).group.type).toBe('local')
    expect((await h.cache.fetchQuery(h.queries.detail('demo-global'))).group.usage?.total).toBe(
      107374182400,
    )
    expect(await h.cache.fetchQuery(h.queries.history('demo-global'))).toHaveLength(3)
    expect((await h.cache.fetchQuery(h.queries.progress('demo-empty'))).progress?.stage).toBe(
      'download',
    )
    expect(
      h.request.mock.calls.every(
        ([input]) =>
          input.method === 'GET' && input.query === undefined && input.body === undefined,
      ),
    ).toBe(true)
  })
  it('rejects invalid history, wrong resource identities and private queries', async () => {
    const h = setup()
    h.request.mockResolvedValue(response({ id: 'other', progress: null }))
    await expect(h.cache.fetchQuery(h.queries.progress('default'))).rejects.toMatchObject({
      code: 'client.invalid_response',
    })
    h.request.mockResolvedValue(response({ group: summaries()[0], nodes: [] }))
    await expect(h.cache.fetchQuery(h.queries.detail('default'))).rejects.toMatchObject({
      code: 'client.invalid_response',
    })
    h.request.mockResolvedValue(
      response([{ at: 'not-a-date', ok: true, code: 'subscription.updated' }]),
    )
    await expect(h.cache.fetchQuery(h.queries.history('default'))).rejects.toMatchObject({
      code: 'client.invalid_response',
    })
    const count = h.request.mock.calls.length
    await expect(
      h.api.request(
        { method: 'GET', path: '/subscriptions/default', query: { private: 'true' } },
        groupSchema,
      ),
    ).rejects.toMatchObject({ status: 403 })
    expect(h.request).toHaveBeenCalledTimes(count)
  })
})

describe('resource snapshot ordering', () => {
  it('restarts an initial list read without cached data when an SSE membership change arrives', async () => {
    const h = setup()
    let finish!: (value: ReturnType<typeof response>) => void
    const next = summaries().slice(0, 2)
    h.request.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    h.request.mockResolvedValue(catalogResponse(next))
    const hook = renderHook(() => useQuery(h.queries.catalog), { wrapper: h.Wrapper })
    await waitFor(() => expect(h.request).toHaveBeenCalledTimes(1))
    act(() => acceptResourceEvent(h.cache, next, false))
    await waitFor(() => expect(hook.result.current.data).toHaveLength(2))
    expect(h.request).toHaveBeenCalledTimes(2)
    expect(h.request.mock.calls[0]?.[1]?.aborted).toBe(true)
    await act(async () => {
      finish(catalogResponse(summaries()))
    })
    expect(hook.result.current.data).toHaveLength(2)
  })
  it('does not let a new SSE session replay overwrite a newer resource GET', () => {
    const h = setup()
    const fresh = summaries().map((group) => ({ ...group, revision: 9, name: `${group.name} new` }))
    h.cache.setQueryData(['subscriptions'], fresh)
    acceptResourceEvent(h.cache, summaries(), true)
    expect(h.cache.getQueryData(['subscriptions'])).toEqual(fresh)
    const next = fresh.map((group) => ({ ...group, revision: 10, name: `${group.name} updated` }))
    acceptResourceEvent(h.cache, next, false)
    expect(h.cache.getQueryData(['subscriptions'])).toEqual(next)
    acceptResourceEvent(h.cache, summaries(), false)
    expect(h.cache.getQueryData(['subscriptions'])).toEqual(next)
  })
  it('reconciles membership changes by GET rather than accepting unversioned deletion', async () => {
    const h = setup()
    h.cache.setQueryData(['subscriptions'], summaries())
    acceptResourceEvent(h.cache, [], false)
    expect(h.cache.getQueryData(['subscriptions'])).toHaveLength(4)
    await act(async () => {})
    expect(h.cache.getQueryState(['subscriptions'])?.isInvalidated).toBe(true)
    h.request.mockResolvedValue(catalogResponse([]))
    expect(await h.cache.fetchQuery(h.queries.catalog)).toEqual([])
  })
  it('discards a delayed GET after a newer SSE publication or session shutdown', async () => {
    const h = setup()
    const old = summaries()
    h.cache.setQueryData(['subscriptions'], old)
    let finish!: (value: ReturnType<typeof response>) => void
    h.request.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    const read = h.cache.fetchQuery(h.queries.catalog).catch(() => undefined)
    const newer = old.map((group) => ({ ...group, revision: 10 }))
    acceptResourceEvent(h.cache, newer, false)
    finish(catalogResponse(old))
    await read
    expect(h.cache.getQueryData(['subscriptions'])).toEqual(newer)
    const detail = h.cache.fetchQuery(h.queries.detail('default')).catch(() => undefined)
    resetResourceSession(h.cache)
    expect(h.request.mock.calls.at(-1)?.[1]?.aborted).toBe(true)
    finish(response(createFixtures().snapshot.groups[1]))
    await detail
    expect(h.cache.getQueryData(['resource', 'default', 'detail'])).toBeUndefined()
  })
  it('keeps newer metadata by revision then updated_at instead of client arrival time', () => {
    const base = summaries()
    const newer = base.map((group) => ({ ...group, updated_at: '2026-10-18T00:00:00Z' }))
    expect(mergeResourceSummaries(newer, base)).toEqual(newer)
  })
})

describe('detail lifetime and bounded progress', () => {
  it('rechecks durable results when progress disappears without inventing success', async () => {
    vi.useFakeTimers()
    const h = setup()
    const normal = h.request.getMockImplementation()!
    let checks = 0
    h.request.mockImplementation((input, signal) =>
      input.path === '/subscriptions/demo-empty/progress' && ++checks > 1
        ? Promise.resolve(response({ id: 'demo-empty', progress: null }))
        : normal(input, signal),
    )
    const hook = renderHook(() => useResourceDetails('demo-empty', true, true), {
      wrapper: h.Wrapper,
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10)
    })
    expect(hook.result.current.progress.data?.progress?.stage).toBe('download')
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5010)
    })
    expect(hook.result.current.progress.data?.progress).toBeNull()
    expect(hook.result.current.observing).toBe(false)
    expect(hook.result.current.history.data?.at(-1)?.ok).toBe(false)
    expect(
      h.request.mock.calls.filter(([input]) => input.path.endsWith('/history')).length,
    ).toBeGreaterThan(1)
    const stopped = h.request.mock.calls.length
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000)
    })
    expect(h.request).toHaveBeenCalledTimes(stopped)
  })
  it('reads local details without URL-only requests and cancels reads on unmount', async () => {
    const h = setup()
    const hook = renderHook(() => useResourceDetails('default', false, true), {
      wrapper: h.Wrapper,
    })
    await waitFor(() => expect(hook.result.current.detail.data?.group.id).toBe('default'))
    expect(h.request.mock.calls.map(([input]) => input.path)).toEqual(['/subscriptions/default'])
    hook.unmount()
    let release!: (value: ReturnType<typeof response>) => void
    h.request.mockImplementation(
      () =>
        new Promise((resolve) => {
          release = resolve
        }),
    )
    const pending = renderHook(() => useResourceDetails('demo-global', false, true), {
      wrapper: h.Wrapper,
    })
    await waitFor(() =>
      expect(h.request.mock.calls.at(-1)?.[0].path).toBe('/subscriptions/demo-global'),
    )
    pending.unmount()
    expect(h.request.mock.calls.at(-1)?.[1]?.aborted).toBe(true)
    release(response(createFixtures().snapshot.groups[0]))
    await act(async () => {})
    expect(h.cache.getQueryData(['resource', 'demo-global', 'detail'])).toBeUndefined()
  })
  it.each(['demo-global', 'demo-empty'])(
    'only refreshes active progress, stops after one minute and pauses in background: %s',
    async (id) => {
      vi.useFakeTimers()
      const h = setup()
      const hook = renderHook(({ live }) => useResourceDetails(id, true, live), {
        initialProps: { live: true },
        wrapper: h.Wrapper,
      })
      await act(async () => {
        await vi.advanceTimersByTimeAsync(10)
      })
      const initial = h.request.mock.calls.length
      expect(initial).toBe(3)
      await act(async () => {
        await vi.advanceTimersByTimeAsync(60_100)
      })
      const afterWindow = h.request.mock.calls.length
      if (id === 'demo-global') expect(afterWindow).toBe(initial)
      else {
        expect(afterWindow).toBeGreaterThan(initial)
        expect(afterWindow).toBeLessThanOrEqual(15)
        expect(hook.result.current.refreshLimitReached).toBe(true)
      }
      await act(async () => {
        await vi.advanceTimersByTimeAsync(60_000)
      })
      expect(h.request).toHaveBeenCalledTimes(afterWindow)
      hook.rerender({ live: false })
      await act(async () => {
        await vi.advanceTimersByTimeAsync(30_000)
      })
      expect(h.request).toHaveBeenCalledTimes(afterWindow)
      resetResourceSession(h.cache)
      hook.rerender({ live: true })
      await act(async () => {
        await vi.advanceTimersByTimeAsync(10)
      })
      expect(h.request.mock.calls.length).toBeGreaterThan(afterWindow)
      hook.unmount()
      const stopped = h.request.mock.calls.length
      await act(async () => {
        await vi.advanceTimersByTimeAsync(120_000)
      })
      expect(h.request).toHaveBeenCalledTimes(stopped)
    },
  )
})
