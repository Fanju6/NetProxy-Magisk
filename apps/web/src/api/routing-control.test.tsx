import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider, onlineManager } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiContext } from '@/app/context'
import { createFixtures } from '@/mocks/fixtures'
import { ApiClient } from './client'
import { apiOperations, type ApiRequest, type ApiTransport } from './contracts'
import {
  moduleQueries,
  useChangeMode,
  usePolicyDelay,
  useSelectPolicyGroup,
  useServiceAction,
} from './queries'
import {
  observeRouting,
  routingBusy,
  routingOperationKey,
  type RoutingOperation,
} from './routing-control'

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
  onlineManager.setOnline(true)
})
const target = { group_tag: 'Internet', member_tag: 'Hong Kong 01' }
function setup() {
  const fixtures = createFixtures()
  const mode = { mode: 'Rule', available: ['Rule', 'Custom mode'], runtime_mode: 'Rule' }
  const writes: {
    request: ApiRequest
    signal?: AbortSignal | undefined
    task: ReturnType<typeof deferred<Response>>
  }[] = []
  const reads: ApiRequest[] = []
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
      return Promise.resolve(
        response(
          structuredClone(
            request.path === '/service'
              ? fixtures.service
              : request.path === '/policy-groups'
                ? fixtures.policy
                : mode,
          ),
        ),
      )
    }),
    subscribe: () => () => {},
  }
  const cache = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity, gcTime: Infinity },
      mutations: { retry: 3, retryDelay: 0 },
    },
  })
  clients.push(cache)
  cache.setQueryData(['service'], fixtures.service)
  cache.setQueryData(['policy-groups'], fixtures.policy)
  cache.setQueryData(['events-connection'], 'live')
  const api = new ApiClient(transport)
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <ApiContext value={api}>
        <QueryClientProvider client={cache}>{children}</QueryClientProvider>
      </ApiContext>
    )
  }
  const select = () => {
    fixtures.policy.groups[0]!.selected = target.member_tag
  }
  return { fixtures, mode, writes, reads, transport, cache, api, Wrapper, select }
}
describe('native policy routing transactions', () => {
  it('does not confirm a POST snapshot with an old authoritative GET', async () => {
    const h = setup()
    const hook = renderHook(useSelectPolicyGroup, { wrapper: h.Wrapper })
    act(() => hook.result.current.mutate(target))
    await waitFor(() => expect(h.writes).toHaveLength(1))
    const selected = structuredClone(h.fixtures.policy)
    selected.groups[0]!.selected = target.member_tag
    await act(async () => h.writes[0]!.task.resolve(response(selected)))
    await waitFor(() => expect(h.cache.isMutating()).toBe(0))
    expect(hook.result.current.isUncertain).toBe(true)
    expect(h.cache.getQueryData(['policy-groups'])).toEqual(h.fixtures.policy)
    h.select()
    await act(async () => hook.result.current.verify())
    await waitFor(() => expect(hook.result.current.isUncertain).toBe(false))
    expect(h.writes).toHaveLength(1)
  })
  it('serializes policy, mode and service across hook instances synchronously', async () => {
    const h = setup()
    const hook = renderHook(
      () => ({
        one: useSelectPolicyGroup(),
        two: useSelectPolicyGroup(),
        mode: useChangeMode(),
        service: useServiceAction(),
      }),
      { wrapper: h.Wrapper },
    )
    act(() => {
      hook.result.current.one.mutate(target)
      hook.result.current.two.mutate(target)
      hook.result.current.mode.mutate('Custom mode')
      hook.result.current.service.mutate('stop')
    })
    await waitFor(() => expect(h.writes).toHaveLength(1))
    expect(h.writes[0]!.request).toEqual({
      method: 'POST',
      path: '/policy-groups/select',
      body: target,
    })
    await act(async () => h.writes[0]!.task.reject(new Error('lost')))
  })
  it('blocks a policy write when a service action already owns admission', async () => {
    const h = setup()
    const hook = renderHook(
      () => ({ policy: useSelectPolicyGroup(), service: useServiceAction() }),
      { wrapper: h.Wrapper },
    )
    act(() => {
      hook.result.current.service.mutate('stop')
      hook.result.current.policy.mutate(target)
    })
    await waitFor(() => expect(h.writes).toHaveLength(1))
    expect(h.writes[0]!.request.path).toBe('/service/stop')
    await act(async () => h.writes[0]!.task.reject(new Error('lost')))
  })
  it.each([401, 403, 409, 422])('retains explicit rejection %s without retry', async (status) => {
    const h = setup()
    const hook = renderHook(useSelectPolicyGroup, { wrapper: h.Wrapper })
    act(() => hook.result.current.mutate(target))
    await waitFor(() => expect(h.writes).toHaveLength(1))
    await act(async () =>
      h.writes[0]!.task.resolve({
        status,
        body: { schema: 1, ok: false, code: 'policy.rejected', message: '', data: {} },
      }),
    )
    await waitFor(() => expect(hook.result.current.isPending).toBe(false))
    expect(hook.result.current.isUncertain).toBe(false)
    expect(hook.result.current.error).toMatchObject({ code: 'policy.rejected', status })
    expect(h.writes).toHaveLength(1)
  })
  it.each(['policy', 'mode'] as const)(
    'retains persisted %s runtime failure after readback',
    async (kind) => {
      const h = setup()
      const hook = renderHook(() => ({ policy: useSelectPolicyGroup(), mode: useChangeMode() }), {
        wrapper: h.Wrapper,
      })
      act(() =>
        kind === 'policy'
          ? hook.result.current.policy.mutate(target)
          : hook.result.current.mode.mutate('Custom mode'),
      )
      await waitFor(() => expect(h.writes).toHaveLength(1))
      if (kind === 'policy') h.select()
      else h.mode.mode = 'Custom mode'
      await act(async () =>
        h.writes[0]!.task.resolve({
          status: 502,
          body: {
            schema: 1,
            ok: false,
            code: kind + '.runtime_sync_failed',
            message: '',
            data: { persisted: true },
          },
        }),
      )
      await waitFor(() => expect(hook.result.current[kind].isUncertain).toBe(false))
      expect(hook.result.current[kind].persisted).toBe(true)
      expect(hook.result.current[kind].error).toMatchObject({ code: kind + '.runtime_sync_failed' })
      expect(h.writes).toHaveLength(1)
    },
  )
  it('finishes accepted selection after its original hook unmounts', async () => {
    const h = setup()
    const hook = renderHook(useSelectPolicyGroup, { wrapper: h.Wrapper })
    act(() => hook.result.current.mutate(target))
    await waitFor(() => expect(h.writes).toHaveLength(1))
    hook.unmount()
    h.select()
    await act(async () => h.writes[0]!.task.resolve(response(h.fixtures.policy)))
    await waitFor(() =>
      expect(h.cache.getQueryData<RoutingOperation>(routingOperationKey)?.phase).toBe('idle'),
    )
    expect(h.writes[0]!.signal).toBeUndefined()
  })
  it('leaves malformed or wrong-core POST results uncertain, without replay', async () => {
    const h = setup()
    const hook = renderHook(useSelectPolicyGroup, { wrapper: h.Wrapper })
    act(() => hook.result.current.mutate(target))
    await waitFor(() => expect(h.writes).toHaveLength(1))
    await act(async () =>
      h.writes[0]!.task.resolve(response({ ...h.fixtures.policy, core_started_at_millis: 99 })),
    )
    await waitFor(() => expect(hook.result.current.isUncertain).toBe(true))
    expect(hook.result.current.error).toMatchObject({ code: 'client.invalid_response' })
    expect(h.writes).toHaveLength(1)
  })
  it('ends exclusive observation without claiming success or issuing requests', () => {
    const h = setup()
    h.cache.setQueryData<RoutingOperation>(routingOperationKey, {
      epoch: 1,
      target: { kind: 'policy', value: target },
      phase: 'uncertain',
      deadline: Date.now() - 1,
      responseReceived: false,
      error: null,
    })
    expect(routingBusy(h.cache)).toBe(false)
    expect(h.cache.getQueryData<RoutingOperation>(routingOperationKey)?.phase).toBe('unconfirmed')
    observeRouting(h.cache, h.fixtures.policy)
    expect(h.cache.getQueryData<RoutingOperation>(routingOperationKey)?.phase).toBe('unconfirmed')
    h.select()
    observeRouting(h.cache, h.fixtures.policy)
    expect(h.cache.getQueryData<RoutingOperation>(routingOperationKey)?.phase).toBe('idle')
    expect(h.writes).toHaveLength(0)
    expect(h.reads).toHaveLength(0)
  })
  it('rejects old-core GETs and respects native mode names', async () => {
    const h = setup()
    const pending = deferred<Response>()
    vi.mocked(h.transport.request).mockReturnValueOnce(pending.promise)
    const read = h.cache
      .fetchQuery({ ...moduleQueries(h.api).policyGroups, staleTime: 0 })
      .catch(() => undefined)
    h.cache.setQueryData(['service'], { ...h.fixtures.service, core_started_at_millis: 99 })
    pending.resolve(response(h.fixtures.policy))
    await read
    expect(h.cache.getQueryData(['policy-groups'])).toEqual(h.fixtures.policy)
    const hook = renderHook(useChangeMode, { wrapper: h.Wrapper })
    act(() => hook.result.current.mutate('Custom mode'))
    await waitFor(() => expect(h.writes).toHaveLength(1))
    h.mode.mode = 'Custom mode'
    await act(async () => h.writes[0]!.task.resolve(response(h.mode)))
    await waitFor(() => expect(hook.result.current.isUncertain).toBe(false))
  })
})
describe('policy delay task', () => {
  it('sends a native tag once and reads back a complete snapshot', async () => {
    const h = setup()
    const hook = renderHook(usePolicyDelay, { wrapper: h.Wrapper })
    act(() => {
      hook.result.current.mutate('Fastest')
      hook.result.current.mutate('Fastest')
    })
    await waitFor(() => expect(h.writes).toHaveLength(1))
    expect(h.writes[0]!.request).toEqual({
      method: 'POST',
      path: '/policy-groups/delay',
      body: { group_tag: 'Fastest' },
    })
    await act(async () => h.writes[0]!.task.resolve(response(h.fixtures.policy)))
    await waitFor(() => expect(hook.result.current.outcome).toBe('cached'))
    expect(h.writes).toHaveLength(1)
  })
  it.each(['missing group', 'old core', 'offline'] as const)(
    'rejects %s delay snapshots',
    async (failure) => {
      const h = setup()
      const hook = renderHook(usePolicyDelay, { wrapper: h.Wrapper })
      act(() => hook.result.current.mutate('Fastest'))
      await waitFor(() => expect(h.writes).toHaveLength(1))
      const result = structuredClone(h.fixtures.policy)
      if (failure === 'missing group') result.groups = []
      if (failure === 'old core') result.core_started_at_millis = 99
      if (failure === 'offline') result.live = false
      await act(async () => h.writes[0]!.task.resolve(response(result)))
      await waitFor(() =>
        expect(hook.result.current.error).toMatchObject({ code: 'client.invalid_response' }),
      )
      expect(h.writes).toHaveLength(1)
    },
  )
  it('cancels on unmount and ignores late responses', async () => {
    const h = setup()
    const hook = renderHook(usePolicyDelay, { wrapper: h.Wrapper })
    act(() => hook.result.current.mutate('Fastest'))
    await waitFor(() => expect(h.writes).toHaveLength(1))
    hook.unmount()
    expect(h.writes[0]!.signal?.aborted).toBe(true)
    await act(async () => h.writes[0]!.task.resolve(response(h.fixtures.policy)))
    expect(h.writes).toHaveLength(1)
  })
  it('cancels when foreground or core identity is lost', async () => {
    const h = setup()
    const hook = renderHook(usePolicyDelay, { wrapper: h.Wrapper })
    act(() => hook.result.current.mutate('Fastest'))
    await waitFor(() => expect(h.writes).toHaveLength(1))
    act(() => h.cache.setQueryData(['events-connection'], 'paused'))
    await waitFor(() => expect(h.writes[0]!.signal?.aborted).toBe(true))
    await act(async () => h.writes[0]!.task.resolve(response(h.fixtures.policy)))
    expect(hook.result.current.outcome).toBeNull()
  })
})
