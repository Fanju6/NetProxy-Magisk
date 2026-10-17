import { act, cleanup, render, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider, onlineManager } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiContext, PreferencesContext } from '@/app/context'
import { translator } from '@/app/i18n'
import { createFixtures } from '@/mocks/fixtures'
import { ApiClient } from './client'
import {
  ApiError,
  apiOperations,
  type ApiEvent,
  type ApiRequest,
  type ApiTransport,
  type NodeSnapshot,
} from './contracts'
import { ModuleEvents } from './events'
import { moduleQueries, useChangeMode, useNodeDelay, useSelectNode } from './queries'
import { routingOperationKey, type RoutingOperation } from './routing-control'

type Response = Awaited<ReturnType<ApiTransport['request']>>
const response = (data: unknown): Response => ({
  status: 200,
  body: { schema: 1, ok: true, code: 'fixture', message: '', data },
})
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((yes, no) => {
    resolve = yes
    reject = no
  })
  return { promise, resolve, reject }
}
const clients: QueryClient[] = []
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  vi.useRealTimers()
})
function setup() {
  const fixtures = createFixtures()
  const state = { mode: 'Rule', available: ['Rule', 'Custom mode'], runtime_mode: 'Rule' }
  const writes: {
    request: ApiRequest
    signal: AbortSignal | undefined
    task: ReturnType<typeof deferred<Response>>
  }[] = []
  const reads: ApiRequest[] = []
  const sessions: { consume: (event: ApiEvent) => void; close: ReturnType<typeof vi.fn> }[] = []
  const transport: ApiTransport = {
    kind: 'native',
    capabilities: apiOperations,
    request: vi.fn((request, signal) => {
      if (request.method !== 'GET') {
        const task = deferred<Response>()
        writes.push({ request, signal, task })
        return task.promise
      }
      reads.push(request)
      const data =
        request.path === '/service'
          ? fixtures.service
          : request.path === '/nodes/snapshot'
            ? fixtures.snapshot
            : request.path === '/selection'
              ? fixtures.snapshot.selection
              : state
      return Promise.resolve(response(structuredClone(data)))
    }),
    subscribe: (_topics, consume) => {
      const session = { consume, close: vi.fn() }
      sessions.push(session)
      return session.close
    },
  }
  const cache = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity, gcTime: Infinity },
      mutations: { retry: 3, retryDelay: 0 },
    },
  })
  clients.push(cache)
  cache.setQueryData(['service'], fixtures.service)
  cache.setQueryData(['nodes'], fixtures.snapshot)
  cache.setQueryData(['events-connection'], 'live')
  const api = new ApiClient(transport)
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <PreferencesContext
        value={{
          preferences: { locale: 'en', theme: 'light' },
          t: translator('en'),
          update: vi.fn(),
        }}
      >
        <ApiContext value={api}>
          <QueryClientProvider client={cache}>{children}</QueryClientProvider>
        </ApiContext>
      </PreferencesContext>
    )
  }
  const select = (tag = 'Tokyo 01') => {
    fixtures.snapshot.selection = {
      ...fixtures.snapshot.selection,
      selector_mode: tag ? 'manual' : 'urltest',
      selected_node_ref: tag ? `demo-global/${tag}` : '',
      selected: tag ? `Global Connect/${tag}` : 'Auto/Global Connect',
    }
  }
  const frame = (name: ApiEvent['name'], data: unknown, session = sessions.at(-1)!) => {
    session.consume({ name, body: response(data).body })
  }
  return { fixtures, state, writes, reads, sessions, transport, cache, api, Wrapper, select, frame }
}

describe('routing transactions', () => {
  it('does not claim live when the core changed during authoritative resource verification', async () => {
    const h = setup()
    h.cache.setQueryData<RoutingOperation>(routingOperationKey, {
      epoch: 2,
      target: { kind: 'mode', value: 'Rule' },
      phase: 'idle',
      deadline: Date.now() + 150000,
      responseReceived: true,
      error: null,
    })
    const modeRead = deferred<Response>()
    const normal = vi.mocked(h.transport.request).getMockImplementation()!
    vi.mocked(h.transport.request).mockImplementation((request, signal) =>
      request.method === 'GET' && request.path === '/mode'
        ? modeRead.promise
        : normal(request, signal),
    )
    render(<ModuleEvents />, { wrapper: h.Wrapper })
    act(() => {
      h.frame('status', h.fixtures.service)
      h.frame('groups', h.fixtures.snapshot)
      h.frame(
        'subscriptions',
        h.fixtures.snapshot.groups.map(({ group }) => group),
      )
    })
    await waitFor(() =>
      expect(
        vi.mocked(h.transport.request).mock.calls.some(([request]) => request.path === '/mode'),
      ).toBe(true),
    )
    act(() => {
      h.frame('status', { ...h.fixtures.service, core_started_at_millis: 999 })
      h.frame('groups', h.fixtures.snapshot)
    })
    await act(async () => modeRead.resolve(response(h.state)))
    await waitFor(() => expect(h.cache.getQueryData(['events-connection'])).toBe('reconnecting'))
    expect(h.sessions[0]!.close).toHaveBeenCalledTimes(1)
    expect(h.writes).toHaveLength(0)
  })
  it('reads service identity before mode verification when applying a mode rebuilt the core', async () => {
    const h = setup()
    const hook = renderHook(useChangeMode, { wrapper: h.Wrapper })
    act(() => hook.result.current.mutate('Custom mode'))
    await waitFor(() => expect(h.writes).toHaveLength(1))
    h.fixtures.service.core_started_at_millis = 999
    h.state.mode = 'Custom mode'
    await act(async () => h.writes[0]!.task.resolve(response(h.state)))
    await waitFor(() => expect(h.cache.isMutating()).toBe(0))
    expect(h.reads.map((entry) => entry.path)).toEqual(['/service', '/mode'])
    expect(h.cache.getQueryData(['mode'])).toMatchObject({ mode: 'Custom mode' })
    expect(hook.result.current.isUncertain).toBe(false)
  })
  it('does not regress to an old same-core Hub replay after GET already confirmed the write', async () => {
    const h = setup()
    h.select()
    h.state.mode = 'Custom mode'
    h.cache.setQueryData(['nodes'], structuredClone(h.fixtures.snapshot))
    h.cache.setQueryData(['mode'], h.state)
    h.cache.setQueryData<RoutingOperation>(routingOperationKey, {
      epoch: 2,
      target: { kind: 'selection', value: { group_id: 'demo-global', node_tag: 'Tokyo 01' } },
      phase: 'idle',
      deadline: Date.now() + 150000,
      responseReceived: true,
      error: null,
    })
    const seen: string[] = []
    const release = h.cache.getQueryCache().subscribe(() => {
      seen.push(h.cache.getQueryData<NodeSnapshot>(['nodes'])!.selection.selector_mode)
    })
    render(<ModuleEvents />, { wrapper: h.Wrapper })
    const stale = createFixtures()
    act(() => {
      h.frame('status', stale.service)
      h.frame('groups', stale.snapshot)
      h.frame(
        'subscriptions',
        stale.snapshot.groups.map(({ group }) => group),
      )
    })
    await waitFor(() => expect(h.cache.getQueryData(['events-connection'])).toBe('live'))
    expect(h.cache.getQueryData<NodeSnapshot>(['nodes'])!.selection.selected_node_ref).toBe(
      'demo-global/Tokyo 01',
    )
    expect(h.cache.getQueryData(['mode'])).toMatchObject({ mode: 'Custom mode' })
    expect(seen.every((value) => value === 'manual')).toBe(true)
    release()
  })
  it('finishes an accepted selection after the original page unmounts', async () => {
    const h = setup()
    const hook = renderHook(useSelectNode, { wrapper: h.Wrapper })
    act(() => hook.result.current.mutate({ group_id: 'demo-global', node_tag: 'Tokyo 01' }))
    await waitFor(() => expect(h.writes).toHaveLength(1))
    hook.unmount()
    h.select()
    await act(async () =>
      h.writes[0]!.task.resolve(
        response({ group_id: 'demo-global', mode: 'manual', selected: 'Global Connect/Tokyo 01' }),
      ),
    )
    await waitFor(() =>
      expect(h.cache.getQueryData<RoutingOperation>(routingOperationKey)?.phase).toBe('idle'),
    )
    expect(h.cache.getQueryData<NodeSnapshot>(['nodes'])!.selection.selected_node_ref).toBe(
      'demo-global/Tokyo 01',
    )
    expect(h.writes).toHaveLength(1)
  })
  it('ignores an old-core mode GET even when no user operation changed its epoch', async () => {
    const h = setup()
    const task = deferred<Response>()
    vi.mocked(h.transport.request).mockImplementationOnce(() => task.promise)
    const pending = h.cache.fetchQuery(moduleQueries(h.api).mode).catch(() => {})
    h.cache.setQueryData(['service'], { ...h.fixtures.service, core_started_at_millis: 999 })
    task.resolve(response(h.state))
    await pending
    expect(h.cache.getQueryData(['mode'])).toBeUndefined()
  })
  it('holds initial groups until status establishes the session and removes old runtime data', () => {
    const h = setup()
    const next = { ...h.fixtures.service, core_started_at_millis: 999 }
    // GET 可能已读到新核心；旧节点缓存仍不能因 service 缓存已经更新而被沿用。
    h.cache.setQueryData(['service'], next)
    render(<ModuleEvents />, { wrapper: h.Wrapper })
    act(() => h.frame('status', next))
    expect(h.cache.getQueryData<NodeSnapshot>(['nodes'])!.runtime_groups).toEqual([])
    act(() => h.frame('groups', h.fixtures.snapshot))
    expect(h.cache.getQueryData<NodeSnapshot>(['nodes'])!.runtime_groups).toEqual(
      h.fixtures.snapshot.runtime_groups,
    )
  })
  it('accepts reordered initial groups only after the first status snapshot', () => {
    const h = setup()
    render(<ModuleEvents />, { wrapper: h.Wrapper })
    const nodes = structuredClone(h.fixtures.snapshot)
    nodes.selection.runtime_selected = 'Global Connect/Tokyo 01'
    act(() => h.frame('groups', nodes))
    expect(h.cache.getQueryData<NodeSnapshot>(['nodes'])!.selection.runtime_selected).not.toBe(
      nodes.selection.runtime_selected,
    )
    act(() => h.frame('status', h.fixtures.service))
    expect(h.cache.getQueryData<NodeSnapshot>(['nodes'])!.selection.runtime_selected).toBe(
      nodes.selection.runtime_selected,
    )
  })
  it('serializes selection and mode across hook instances without optimistic state or replay', async () => {
    const h = setup()
    const hooks = renderHook(() => ({ selection: useSelectNode(), mode: useChangeMode() }), {
      wrapper: h.Wrapper,
    })
    const wasOnline = onlineManager.isOnline()
    onlineManager.setOnline(false)
    try {
      act(() => {
        hooks.result.current.selection.mutate({ group_id: 'demo-global', node_tag: 'Tokyo 01' })
        hooks.result.current.mode.mutate('Custom mode')
        hooks.result.current.selection.mutate({ group_id: 'demo-global', node_tag: '' })
      })
      await waitFor(() => expect(h.writes).toHaveLength(1))
      expect(h.writes[0]!.request).toEqual({
        method: 'PUT',
        path: '/selection',
        body: { group_id: 'demo-global', node_tag: 'Tokyo 01' },
      })
      expect(h.cache.getQueryData<NodeSnapshot>(['nodes'])!.selection.selector_mode).toBe('urltest')
      h.select()
      await act(async () =>
        h.writes[0]!.task.resolve(
          response({
            group_id: 'demo-global',
            mode: 'manual',
            selected: 'Global Connect/Tokyo 01',
          }),
        ),
      )
      await waitFor(() => expect(hooks.result.current.selection.isUncertain).toBe(false))
      expect(h.reads.map((item) => item.path)).toEqual(
        expect.arrayContaining(['/selection', '/nodes/snapshot', '/service']),
      )
      expect(h.cache.getQueryData<NodeSnapshot>(['nodes'])!.selection.selected_node_ref).toBe(
        'demo-global/Tokyo 01',
      )
      expect(h.writes).toHaveLength(1)
    } finally {
      onlineManager.setOnline(wasOnline)
    }
  })
  it('persists Auto with empty raw tag while stopped', async () => {
    const h = setup()
    h.fixtures.service.state = 'stopped'
    h.fixtures.service.core_started_at_millis = 0
    h.cache.setQueryData(['service'], h.fixtures.service)
    const hook = renderHook(useSelectNode, { wrapper: h.Wrapper })
    act(() => hook.result.current.mutate({ group_id: 'demo-global', node_tag: '' }))
    await waitFor(() => expect(h.writes).toHaveLength(1))
    expect(h.writes[0]!.request.body).toEqual({ group_id: 'demo-global', node_tag: '' })
    await act(async () =>
      h.writes[0]!.task.resolve(
        response({ group_id: 'demo-global', mode: 'urltest', selected: 'Auto/Global Connect' }),
      ),
    )
    await waitFor(() =>
      expect(hook.result.current.isPending || hook.result.current.isUncertain).toBe(false),
    )
    expect(h.writes).toHaveLength(1)
  })
  it.each(['selection', 'mode'] as const)(
    'retains %s persisted runtime failure and reads committed state',
    async (kind) => {
      const h = setup()
      const hook = renderHook(() => ({ selection: useSelectNode(), mode: useChangeMode() }), {
        wrapper: h.Wrapper,
      })
      act(() =>
        kind === 'selection'
          ? hook.result.current.selection.mutate({ group_id: 'demo-global', node_tag: 'Tokyo 01' })
          : hook.result.current.mode.mutate('Custom mode'),
      )
      await waitFor(() => expect(h.writes).toHaveLength(1))
      if (kind === 'selection') h.select()
      else h.state.mode = 'Custom mode'
      const code = kind === 'selection' ? 'node.runtime_sync_failed' : 'mode.runtime_sync_failed'
      await act(async () =>
        h.writes[0]!.task.resolve({
          status: 502,
          body: {
            schema: 1,
            ok: false,
            code,
            message: 'fixture',
            data: { persisted: true, runtime_synced: false },
          },
        }),
      )
      await waitFor(() => expect(hook.result.current[kind].isPending).toBe(false))
      expect(hook.result.current[kind].persisted).toBe(true)
      expect(hook.result.current[kind].error).toMatchObject({ code, persisted: true })
      expect(h.cache.getQueryData<RoutingOperation>(routingOperationKey)?.phase).toBe('idle')
      expect(h.writes).toHaveLength(1)
    },
  )
  it('keeps an interrupted write uncertain until authoritative GET confirms; never automatically resends', async () => {
    const h = setup()
    const hook = renderHook(useChangeMode, { wrapper: h.Wrapper })
    act(() => hook.result.current.mutate('Custom mode'))
    await waitFor(() => expect(h.writes).toHaveLength(1))
    await act(async () =>
      h.writes[0]!.task.reject(new ApiError('api.connection_failed', 'fixture', 503)),
    )
    await waitFor(() => expect(hook.result.current.isUncertain).toBe(true))
    act(() => hook.result.current.mutate('Custom mode'))
    expect(h.writes).toHaveLength(1)
    h.state.mode = 'Custom mode'
    await act(async () => hook.result.current.verify())
    await waitFor(() => expect(hook.result.current.isUncertain).toBe(false))
    expect(h.writes).toHaveLength(1)
  })
  it.each([401, 403, 409, 422])('does not replay a definite %s rejection', async (status) => {
    const h = setup()
    const hook = renderHook(useChangeMode, { wrapper: h.Wrapper })
    act(() => hook.result.current.mutate('Custom mode'))
    await waitFor(() => expect(h.writes).toHaveLength(1))
    await act(async () =>
      h.writes[0]!.task.resolve({
        status,
        body: {
          schema: 1,
          ok: false,
          code: 'mode.invalid',
          message: 'fixture',
        },
      }),
    )
    await waitFor(() => expect(hook.result.current.isPending).toBe(false))
    expect(hook.result.current.isUncertain).toBe(false)
    expect(hook.result.current.error).toMatchObject({ status })
    expect(h.writes).toHaveLength(1)
  })
  it('ignores old selection reads and SSE callbacks after a new routing epoch', async () => {
    const h = setup()
    render(<ModuleEvents />, { wrapper: h.Wrapper })
    const hook = renderHook(useSelectNode, { wrapper: h.Wrapper })
    const old = h.sessions[0]!
    const task = deferred<Response>()
    vi.mocked(h.transport.request).mockImplementationOnce(() => task.promise)
    const oldRead = h.cache
      .fetchQuery({ ...moduleQueries(h.api).nodes, staleTime: 0 })
      .catch(() => {})
    act(() => hook.result.current.mutate({ group_id: 'demo-global', node_tag: 'Tokyo 01' }))
    await waitFor(() => expect(h.writes).toHaveLength(1))
    h.select()
    await act(async () =>
      h.writes[0]!.task.resolve(
        response({ group_id: 'demo-global', mode: 'manual', selected: 'Global Connect/Tokyo 01' }),
      ),
    )
    await waitFor(() =>
      expect(h.cache.getQueryData<NodeSnapshot>(['nodes'])!.selection.selector_mode).toBe('manual'),
    )
    await act(async () => {
      task.resolve(response(createFixtures().snapshot))
      await oldRead
      h.frame('groups', createFixtures().snapshot, old)
    })
    expect(h.cache.getQueryData<NodeSnapshot>(['nodes'])!.selection.selector_mode).toBe('manual')
  })
})

describe('delay queries', () => {
  it('tests Auto by empty node tag and does not invent a fresh result for missing items', async () => {
    const h = setup()
    const hook = renderHook(useNodeDelay, { wrapper: h.Wrapper })
    act(() => hook.result.current.mutate({ group_id: 'demo-global', node_tag: '' }))
    await waitFor(() => expect(h.writes).toHaveLength(1))
    expect(h.writes[0]!.request.body).toEqual({ group_id: 'demo-global', node_tag: '' })
    await act(async () =>
      h.writes[0]!.task.resolve(
        response({
          target: 'Auto/Global Connect',
          groups: [
            {
              tag: 'Auto/Global Connect',
              type: 'urltest',
              selectable: false,
              expanded: false,
              items: [{ tag: 'Global Connect/Tokyo 01', type: 'trojan' }],
            },
          ],
        }),
      ),
    )
    expect(hook.result.current.results).toEqual({})
    expect(hook.result.current.outcome).toBe('empty')
    expect(h.cache.getQueryData<NodeSnapshot>(['nodes'])!.runtime_groups).toEqual(
      h.fixtures.snapshot.runtime_groups,
    )
  })
  const result = (delay = 64) => ({
    target: 'Global Connect/Tokyo 01',
    groups: [
      {
        tag: 'Auto/Global Connect',
        type: '',
        selectable: false,
        expanded: false,
        items: [
          {
            tag: 'Global Connect/Tokyo 01',
            type: 'trojan',
            url_test_time: 1792065600000,
            url_test_delay: delay,
          },
        ],
      },
    ],
  })
  it('coalesces duplicate delay clicks, validates runtime names, and retains valid results after failure', async () => {
    const h = setup()
    const hook = renderHook(useNodeDelay, { wrapper: h.Wrapper })
    const target = { group_id: 'demo-global', node_tag: 'Tokyo 01' }
    act(() => {
      hook.result.current.mutate(target)
      hook.result.current.mutate(target)
    })
    await waitFor(() => expect(h.writes).toHaveLength(1))
    expect(h.writes[0]!.request).toEqual({ method: 'POST', path: '/nodes/delay', body: target })
    await act(async () => h.writes[0]!.task.resolve(response(result())))
    expect(hook.result.current.results['demo-global/Tokyo 01']?.delay).toBe(64)
    expect(hook.result.current.outcome).toBe('cached')
    act(() => hook.result.current.mutate(target))
    await waitFor(() => expect(h.writes).toHaveLength(2))
    await act(async () =>
      h.writes[1]!.task.reject(new ApiError('node.delay_failed', 'fixture', 502)),
    )
    expect(hook.result.current.results['demo-global/Tokyo 01']?.delay).toBe(64)
    expect(hook.result.current.error).toMatchObject({ code: 'node.delay_failed' })
    expect(h.writes).toHaveLength(2)
  })
  it.each(['target', 'group', 'item'])('rejects a mismatched delay %s', async (field) => {
    const h = setup()
    const hook = renderHook(useNodeDelay, { wrapper: h.Wrapper })
    act(() => hook.result.current.mutate({ group_id: 'demo-global', node_tag: 'Tokyo 01' }))
    await waitFor(() => expect(h.writes).toHaveLength(1))
    const body = result()
    if (field === 'target') body.target = 'Other/Tokyo 01'
    if (field === 'group') body.groups[0]!.tag = 'Auto/Other'
    if (field === 'item') body.groups[0]!.items[0]!.tag = 'Global Connect/Foreign'
    await act(async () => h.writes[0]!.task.resolve(response(body)))
    expect(hook.result.current.error).toMatchObject({ code: 'client.invalid_response' })
    expect(hook.result.current.results).toEqual({})
  })
  it('cancels on unmount and ignores a late delay response', async () => {
    const h = setup()
    const hook = renderHook(useNodeDelay, { wrapper: h.Wrapper })
    act(() => hook.result.current.mutate({ group_id: 'demo-global', node_tag: 'Tokyo 01' }))
    await waitFor(() => expect(h.writes).toHaveLength(1))
    hook.unmount()
    expect(h.writes[0]!.signal?.aborted).toBe(true)
    await act(async () => h.writes[0]!.task.resolve(response(result())))
    expect(h.writes).toHaveLength(1)
  })
  it('clears old-core results and cancels when native foreground is lost', async () => {
    const h = setup()
    const hook = renderHook(useNodeDelay, { wrapper: h.Wrapper })
    const target = { group_id: 'demo-global', node_tag: 'Tokyo 01' }
    act(() => hook.result.current.mutate(target))
    await waitFor(() => expect(h.writes).toHaveLength(1))
    await act(async () => h.writes[0]!.task.resolve(response(result())))
    act(() =>
      h.cache.setQueryData(['service'], { ...h.fixtures.service, core_started_at_millis: 12345 }),
    )
    await waitFor(() => expect(hook.result.current.results).toEqual({}))
    act(() => hook.result.current.mutate(target))
    await waitFor(() => expect(h.writes).toHaveLength(2))
    act(() => h.cache.setQueryData(['events-connection'], 'paused'))
    await waitFor(() => expect(h.writes[1]!.signal?.aborted).toBe(true))
    await act(async () => h.writes[1]!.task.resolve(response(result())))
    expect(hook.result.current.results).toEqual({})
  })
})
