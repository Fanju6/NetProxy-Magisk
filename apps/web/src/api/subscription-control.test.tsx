import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiContext } from '@/app/context'
import { MockTransport } from '@/mocks/transport'
import { ApiClient } from './client'
import { groupSchema, type ApiTransport } from './contracts'
import { z } from 'zod'
import { useServiceAction, useChangeMode } from './queries'
import {
  beginSubscriptionUpdate,
  beginSubscriptionWrite,
  subscriptionBusy,
  subscriptionOperationKey,
  useSubscriptionActions,
  verifySubscription,
  type SubscriptionOperation,
} from './subscription-control'

const caches: QueryClient[] = []
afterEach(() => {
  cleanup()
  caches.splice(0).forEach((cache) => cache.clear())
  vi.useRealTimers()
})
const ok = (data: unknown) => ({
  status: 200,
  body: { schema: 1, ok: true, code: 'subscription.updated', message: '', data },
})
async function setup() {
  const backend = new MockTransport(0)
  const request = vi.fn<ApiTransport['request']>((input, signal) => backend.request(input, signal))
  const api = new ApiClient({
    kind: 'native',
    capabilities: backend.capabilities,
    request,
    subscribe: () => () => {},
  })
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } })
  caches.push(cache)
  cache.setQueryData(
    ['subscriptions'],
    await api.request({ method: 'GET', path: '/subscriptions' }, z.array(groupSchema)),
  )
  cache.setQueryData(['events-connection'], 'live')
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <ApiContext value={api}>
        <QueryClientProvider client={cache}>{children}</QueryClientProvider>
      </ApiContext>
    )
  }
  return { cache, api, backend, request, Wrapper }
}
describe('subscription update and cancellation', () => {
  it('admits once, re-reads the Catalog and preserves 304 result semantics', async () => {
    const h = await setup()
    const result = {
      group_id: 'demo-global',
      revision: 1,
      node_count: 3,
      persisted: true,
      not_modified: true,
      runtime_synced: true,
      runtime_sync_state: 'applied',
      runtime_sync_pending: false,
    }
    const before = h.cache
      .getQueryData<z.infer<typeof groupSchema>[]>(['subscriptions'])!
      .find((group) => group.id === 'demo-global')!
    result.revision = before.revision
    result.node_count = before.node_count
    h.request.mockImplementation((request, signal) =>
      request.method === 'POST' ? Promise.resolve(ok(result)) : h.backend.request(request, signal),
    )
    const signal = new AbortController().signal
    const first = beginSubscriptionUpdate(h.api, h.cache, 'demo-global', signal)!
    expect(beginSubscriptionUpdate(h.api, h.cache, 'demo-global', signal)).toBeNull()
    await first
    const operation = h.cache.getQueryData<SubscriptionOperation>(subscriptionOperationKey)!
    expect(operation.phase).toBe('idle')
    expect(operation.result?.not_modified).toBe(true)
    expect(h.request.mock.calls.filter(([request]) => request.method === 'POST')).toHaveLength(1)
    expect(h.request.mock.calls.some(([request]) => request.path.endsWith('/history'))).toBe(true)
  })
  it('retains persisted runtime failure, never retries and does not infer success from old progress', async () => {
    const h = await setup()
    h.request.mockImplementation((request, signal) =>
      request.method === 'POST'
        ? Promise.resolve({
            status: 502,
            body: {
              schema: 1,
              ok: false,
              code: 'subscription.runtime_sync_failed',
              message: 'PRIVATE',
              data: { persisted: true, runtime_synced: false, cause: 'PRIVATE' },
            },
          })
        : h.backend.request(request, signal),
    )
    await beginSubscriptionUpdate(h.api, h.cache, 'demo-global', new AbortController().signal)
    const state = h.cache.getQueryData<SubscriptionOperation>(subscriptionOperationKey)!
    expect(state.error?.persisted).toBe(true)
    expect(state.error?.code).toBe('subscription.runtime_sync_failed')
    expect(state.phase).toBe('idle')
    expect(JSON.stringify(state)).not.toContain('PRIVATE')
    expect(h.request.mock.calls.filter(([input]) => input.method === 'POST')).toHaveLength(1)
  })
  it('does not replay a lost response or attribute newer history to an unidentified request', async () => {
    const h = await setup()
    h.request.mockImplementation((input, signal) =>
      input.method === 'POST'
        ? Promise.reject(new Error('PRIVATE'))
        : h.backend.request(input, signal),
    )
    await beginSubscriptionUpdate(h.api, h.cache, 'demo-global', new AbortController().signal)
    await verifySubscription(h.api, h.cache)
    expect(h.cache.getQueryData<SubscriptionOperation>(subscriptionOperationKey)?.phase).toBe(
      'uncertain',
    )
    const current = h.cache.getQueryData<SubscriptionOperation>(subscriptionOperationKey)!
    h.cache.setQueryData(subscriptionOperationKey, { ...current, deadline: Date.now() - 1 })
    expect(subscriptionBusy(h.cache)).toBe(false)
    expect(h.cache.getQueryData<SubscriptionOperation>(subscriptionOperationKey)?.phase).toBe(
      'unconfirmed',
    )
    expect(h.request.mock.calls.filter(([input]) => input.method === 'POST')).toHaveLength(1)
    expect(JSON.stringify(h.cache.getQueryData(subscriptionOperationKey))).not.toContain('PRIVATE')
  })
  it('allows explicit cancellation while a single update is pending, but never calls it completed', async () => {
    const h = await setup()
    const hook = renderHook(() => useSubscriptionActions(), { wrapper: h.Wrapper })
    act(() => {
      expect(hook.result.current.update('demo-global')).toBe(true)
    })
    await waitFor(() =>
      expect(h.request.mock.calls.some(([input]) => input.path.endsWith('/update'))).toBe(true),
    )
    await new Promise((resolve) => setTimeout(resolve, 30))
    await act(async () => {
      await hook.result.current.cancel('demo-global')
    })
    await waitFor(() => expect(hook.result.current.operation?.phase).toBe('idle'))
    expect(hook.result.current.operation?.cancel).toBe('requested')
    expect(hook.result.current.operation?.error?.code).toBe('subscription.cancelled')
    expect(h.request.mock.calls.filter(([input]) => input.path.endsWith('/cancel'))).toHaveLength(1)
    hook.unmount()
  })
  it('rejects local/unknown targets before transport', async () => {
    const h = await setup()
    h.request.mockClear()
    for (const group of ['default', 'missing', 'a?private=true', '../default'])
      expect(
        beginSubscriptionUpdate(h.api, h.cache, group, new AbortController().signal),
      ).toBeNull()
    expect(h.request).not.toHaveBeenCalled()
  })
  it('coordinates subscription, service and routing admission within one event loop in both directions', async () => {
    const h = await setup()
    const hook = renderHook(
      () => ({ sub: useSubscriptionActions(), service: useServiceAction(), mode: useChangeMode() }),
      { wrapper: h.Wrapper },
    )
    h.request.mockClear()
    act(() => {
      hook.result.current.sub.update('demo-global')
      hook.result.current.service.mutate('start')
      hook.result.current.mode.mutate('Proxy')
    })
    await waitFor(() =>
      expect(h.request.mock.calls.filter(([input]) => input.method !== 'GET')).toHaveLength(1),
    )
    expect(h.request.mock.calls.find(([input]) => input.method !== 'GET')![0].path).toBe(
      '/subscriptions/demo-global/update',
    )
    hook.unmount()
    await new Promise((resolve) => setTimeout(resolve, 300))
    h.cache.setQueryData(subscriptionOperationKey, null)
    const next = renderHook(
      () => ({ sub: useSubscriptionActions(), service: useServiceAction() }),
      { wrapper: h.Wrapper },
    )
    h.request.mockClear()
    act(() => {
      next.result.current.service.mutate('start')
      expect(next.result.current.sub.update('demo-global')).toBe(false)
    })
    await waitFor(() =>
      expect(h.request.mock.calls.some(([input]) => input.path === '/service/start')).toBe(true),
    )
    next.unmount()
  })
  it('creates a subscription without caching its input, then preserves omitted fields and explicit clears', async () => {
    const h = await setup()
    const input = {
      name: 'New public name',
      url: 'https://source.example.invalid/feed?token=WRITE-ONLY',
      hwid: 'WRITE-ONLY',
      custom_headers: { Authorization: 'WRITE-ONLY' },
      auto_update: false,
    }
    await beginSubscriptionWrite(
      h.api,
      h.cache,
      { action: 'add', input },
      new AbortController().signal,
    )
    let state = h.cache.getQueryData<SubscriptionOperation>(subscriptionOperationKey)!
    expect(state.phase).toBe('idle')
    expect(state.result?.persisted).toBe(true)
    const id = state.groupId!
    expect(id).toBeTruthy()
    expect(h.cache.getMutationCache().getAll()).toHaveLength(0)
    expect(
      JSON.stringify(
        h.cache
          .getQueryCache()
          .getAll()
          .map((query) => query.state.data),
      ),
    ).not.toContain('WRITE-ONLY')
    h.request.mockClear()
    await beginSubscriptionWrite(
      h.api,
      h.cache,
      { action: 'edit', groupId: id, input: { name: 'Renamed' } },
      new AbortController().signal,
    )
    expect(h.request.mock.calls.find(([request]) => request.method === 'PUT')![0].body).toEqual({
      name: 'Renamed',
    })
    await beginSubscriptionWrite(
      h.api,
      h.cache,
      { action: 'edit', groupId: id, input: { hwid: '', user_agent: '', custom_headers: {} } },
      new AbortController().signal,
    )
    state = h.cache.getQueryData<SubscriptionOperation>(subscriptionOperationKey)!
    expect(state.phase).toBe('idle')
    expect(
      h.request.mock.calls.filter(([request]) => request.method === 'PUT').at(-1)![0].body,
    ).toEqual({ hwid: '', user_agent: '', custom_headers: {} })
  })
  it('keeps persisted creation and edit failures without replay or rollback', async () => {
    const h = await setup()
    await beginSubscriptionWrite(
      h.api,
      h.cache,
      {
        action: 'add',
        input: { name: 'Failed source', url: 'https://source.example.invalid/fail' },
      },
      new AbortController().signal,
    )
    const current = h.cache.getQueryData<SubscriptionOperation>(subscriptionOperationKey)!
    expect(current.error?.persisted).toBe(true)
    expect(current.phase).toBe('idle')
    const groups = h.cache.getQueryData<z.infer<typeof groupSchema>[]>(['subscriptions'])!
    const added = groups.find((group) => group.name === 'Failed source')!
    expect(added.node_count).toBe(0)
    const old = groups.find((group) => group.id === 'demo-global')!
    await beginSubscriptionWrite(
      h.api,
      h.cache,
      { action: 'edit', groupId: old.id, input: { url: 'https://source.example.invalid/fail' } },
      new AbortController().signal,
    )
    expect(
      h.cache.getQueryData<SubscriptionOperation>(subscriptionOperationKey)?.error?.persisted,
    ).toBe(true)
    expect(
      h.cache
        .getQueryData<z.infer<typeof groupSchema>[]>(['subscriptions'])!
        .find((group) => group.id === old.id)?.node_count,
    ).toBe(old.node_count)
    expect(h.request.mock.calls.filter(([request]) => request.method === 'PUT')).toHaveLength(1)
  })
  it('deletes a URL resource without a replacement or a policy selection', async () => {
    const h = await setup()
    await beginSubscriptionWrite(
      h.api,
      h.cache,
      { action: 'remove', groupId: 'demo-global' },
      new AbortController().signal,
    )
    expect(h.cache.getQueryData<SubscriptionOperation>(subscriptionOperationKey)?.phase).toBe(
      'idle',
    )
    expect(
      h.request.mock.calls
        .filter(([request]) => request.method === 'DELETE')
        .map(([request]) => request),
    ).toEqual([{ method: 'DELETE', path: '/subscriptions/demo-global' }])
    expect(
      h.cache
        .getQueryData<z.infer<typeof groupSchema>[]>(['subscriptions'])!
        .some((group) => group.id === 'demo-global'),
    ).toBe(false)
  })
  it('never repeats a deletion with a lost response; the missing group is a current fact, not a task ID', async () => {
    const h = await setup()
    h.request.mockImplementation(async (request, signal) => {
      const response = await h.backend.request(request, signal)
      if (request.method === 'DELETE') throw new Error('Connection lost')
      return response
    })
    await beginSubscriptionWrite(
      h.api,
      h.cache,
      { action: 'remove', groupId: 'demo-global' },
      new AbortController().signal,
    )
    await verifySubscription(h.api, h.cache)
    expect(h.cache.getQueryData<SubscriptionOperation>(subscriptionOperationKey)?.phase).toBe(
      'uncertain',
    )
    expect(h.request.mock.calls.filter(([request]) => request.method === 'DELETE')).toHaveLength(1)
  })
  it('keeps independent updated/failed outcomes and original error on update-all partial failure', async () => {
    const h = await setup()
    await beginSubscriptionWrite(
      h.api,
      h.cache,
      { action: 'update_all' },
      new AbortController().signal,
    )
    const state = h.cache.getQueryData<SubscriptionOperation>(subscriptionOperationKey)!
    expect(state.phase).toBe('idle')
    expect(state.error?.code).toBe('subscription.convert_failed')
    expect(state.result?.updated).toEqual(['demo-global'])
    expect(state.result?.failed).toEqual(['demo-empty'])
    expect(
      h.request.mock.calls.filter(([request]) => request.path === '/subscriptions/update-all'),
    ).toHaveLength(1)
  })
  it('rejects invalid create/patch fields and local edits before admitting a transaction', async () => {
    const h = await setup()
    for (const input of [
      { url: '' },
      { url: 'file:///tmp/source' },
      { url: 'https://example.invalid', update_interval: 2 },
      { hwid: null },
    ])
      expect(
        beginSubscriptionWrite(
          h.api,
          h.cache,
          { action: 'add', input: input as never },
          new AbortController().signal,
        ),
      ).toBeNull()
    expect(
      beginSubscriptionWrite(
        h.api,
        h.cache,
        { action: 'edit', groupId: 'default', input: { name: 'Changed' } },
        new AbortController().signal,
      ),
    ).toBeNull()
  })
})
