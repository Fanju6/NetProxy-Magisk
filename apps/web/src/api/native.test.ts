import { afterEach, describe, expect, it, vi } from 'vitest'
import { clearMocks, mockIPC } from '@tauri-apps/api/mocks'
import { z } from 'zod'
import { ApiClient } from './client'
import { type ApiEvent, type ApiRequest } from './contracts'
import { NativeTransport, createNativeTransport, type NativeIpc } from './native'

afterEach(() => {
  clearMocks()
  vi.unstubAllGlobals()
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
const flush = async () => {
  for (let i = 0; i < 10; i++) await Promise.resolve()
}
function setup() {
  const invoke = vi.fn<(command: string, args: Record<string, unknown>) => Promise<unknown>>()
  const unregister = vi.fn().mockResolvedValue(undefined)
  let visibility!: (event: unknown) => void
  const listen = vi.fn<NativeIpc['listen']>((listener) => {
    visibility = listener
    return Promise.resolve({ unregister })
  })
  const ipc: NativeIpc = {
    invoke: <T>(name: string, args: Record<string, unknown>) => invoke(name, args) as Promise<T>,
    listen,
  }
  const transport = new NativeTransport(ipc)
  return {
    transport,
    invoke,
    listen,
    unregister,
    visibility: (event: unknown) => visibility(event),
  }
}
const event: ApiEvent = {
  name: 'status',
  body: { schema: 1, ok: true, code: 'service.status', message: '', data: {} },
}

describe('restricted native requests', () => {
  it('uses official invoke and plugin listener registration without credential commands', async () => {
    vi.stubGlobal('isTauri', true)
    const calls = vi.fn((name: string) =>
      name.endsWith('|read')
        ? { status: 200, body: event.body }
        : name.endsWith('|visibility')
          ? { visible: true }
          : undefined,
    )
    mockIPC(calls)
    const transport = createNativeTransport()
    expect(transport.kind).toBe('native')
    await transport.request({ method: 'GET', path: '/info' })
    const listener = vi.fn()
    const close = transport.observeVisibility!(listener)
    await flush()
    expect(listener).toHaveBeenCalledExactlyOnceWith(true)
    close()
    await flush()
    expect(calls.mock.calls.map(([name]) => name)).toEqual([
      'plugin:module-api|read',
      'plugin:module-api|register_listener',
      'plugin:module-api|visibility',
      'plugin:module-api|remove_listener',
    ])
  })
  it.each(['/info', '/service'] as const)(
    'maps %s to a resource, never a URL or secret',
    async (path) => {
      const { transport, invoke } = setup()
      const response = { status: 200, body: event.body }
      invoke.mockResolvedValue(response)
      expect(await transport.request({ method: 'GET', path })).toEqual(response)
      expect(invoke).toHaveBeenCalledExactlyOnceWith('plugin:module-api|read', {
        requestId: expect.any(String),
        resource: path.slice(1),
      })
      expect(transport.readOnly).toBe(true)
    },
  )
  it.each([
    { method: 'POST', path: '/service/start' },
    { method: 'PUT', path: '/selection', body: {} },
    { method: 'DELETE', path: '/subscriptions' },
    { method: 'GET', path: '/nodes/snapshot' },
    { method: 'GET', path: '/subscriptions' },
    { method: 'GET', path: '/config' },
    { method: 'GET', path: '/info', query: {} },
    { method: 'GET', path: '/info', body: null },
    { method: 'GET', path: 'https://example.invalid/' },
  ])('rejects unauthorized resources before IPC: %j', async (request) => {
    const { transport, invoke } = setup()
    await expect(transport.request(request as ApiRequest)).rejects.toMatchObject({
      code: 'client.read_only',
      status: 403,
    })
    expect(invoke).not.toHaveBeenCalled()
  })
  it('cancels only the matching request and ignores its late response', async () => {
    const { transport, invoke } = setup()
    const first = deferred<unknown>()
    const second = deferred<unknown>()
    invoke.mockImplementation((command, args) =>
      command.endsWith('|read')
        ? args.resource === 'info'
          ? first.promise
          : second.promise
        : Promise.resolve(),
    )
    const abort = new AbortController()
    const a = transport.request({ method: 'GET', path: '/info' }, abort.signal)
    const b = transport.request({ method: 'GET', path: '/service' })
    const failure = expect(a).rejects.toMatchObject({ name: 'AbortError' })
    abort.abort()
    await failure
    const id = invoke.mock.calls[0]![1].requestId
    expect(invoke).toHaveBeenLastCalledWith('plugin:module-api|cancel', { requestId: id })
    expect(id).not.toEqual(invoke.mock.calls[1]![1].requestId)
    second.resolve({ status: 200, body: event.body })
    await expect(b).resolves.toMatchObject({ status: 200 })
    first.resolve({ status: 200, body: event.body })
    await flush()
    expect(invoke.mock.calls.filter(([name]) => name.endsWith('|cancel'))).toHaveLength(1)
  })
  it('does not dispatch already cancelled reads or cancel completed ones', async () => {
    const { transport, invoke } = setup()
    const aborted = new AbortController()
    aborted.abort()
    await expect(
      transport.request({ method: 'GET', path: '/info' }, aborted.signal),
    ).rejects.toMatchObject({ name: 'AbortError' })
    expect(invoke).not.toHaveBeenCalled()
    invoke.mockResolvedValue({ status: 200, body: event.body })
    const controller = new AbortController()
    await transport.request({ method: 'GET', path: '/info' }, controller.signal)
    controller.abort()
    expect(invoke).toHaveBeenCalledTimes(1)
  })
  it.each([
    { code: 'api.unauthorized', status: 401, message: 'secret fixture' },
    { code: 'api.connection_failed', status: 0, message: 'secret fixture' },
    'secret fixture',
    new Error('secret fixture'),
  ])('sanitizes native errors and preserves structured classification', async (error) => {
    const { transport, invoke } = setup()
    invoke.mockRejectedValue(error)
    const result = await transport
      .request({ method: 'GET', path: '/info' })
      .catch((value: unknown) => value)
    expect(result).not.toBe(error)
    expect(JSON.stringify(result)).not.toContain('secret fixture')
    expect(String(result)).not.toContain('secret fixture')
    expect(result).toMatchObject({
      code: typeof error === 'object' && 'code' in error ? error.code : 'client.native_failed',
    })
    expect(invoke).toHaveBeenCalledTimes(1)
  })
  it('preserves schema and HTTP failures through ApiClient', async () => {
    const { transport, invoke } = setup()
    const client = new ApiClient(transport)
    invoke.mockResolvedValue({
      status: 401,
      body: { schema: 1, ok: false, code: 'api.unauthorized', message: 'Rejected' },
    })
    await expect(
      client.request({ method: 'GET', path: '/info' }, z.unknown()),
    ).rejects.toMatchObject({ code: 'api.unauthorized', status: 401 })
    invoke.mockResolvedValue({ status: 200, body: { schema: 2 } })
    await expect(
      client.request({ method: 'GET', path: '/info' }, z.unknown()),
    ).rejects.toMatchObject({ code: 'client.invalid_response' })
    await expect(
      client.request({ method: 'POST', path: '/service/start' }, z.unknown()),
    ).rejects.toMatchObject({ code: 'client.read_only' })
    expect(invoke).toHaveBeenCalledTimes(2)
  })
  it.each([{}, { status: 0, body: {} }, { status: 200, body: 'x'.repeat(4 * 1024 * 1024) }])(
    'rejects malformed or oversized native replies',
    async (value) => {
      const { transport, invoke } = setup()
      invoke.mockResolvedValue(value)
      await expect(transport.request({ method: 'GET', path: '/info' })).rejects.toMatchObject({
        status: expect.any(Number),
      })
    },
  )
  it('fails closed outside Tauri', () => expect(createNativeTransport().kind).toBe('unavailable'))
})

describe('native event pull lifecycle', () => {
  it.each([{ topics: [] }, { topics: ['traffic'] }, { topics: ['status', 'private'] }])(
    'rejects invalid topic sets without allocating native resources',
    async ({ topics }) => {
      const { transport, invoke } = setup()
      const fail = vi.fn()
      const close = transport.subscribe(
        topics as Parameters<NativeTransport['subscribe']>[0],
        vi.fn(),
        fail,
      )
      await flush()
      close()
      expect(fail).toHaveBeenCalledOnce()
      expect(invoke).not.toHaveBeenCalled()
    },
  )
  it('maintains one pending next_event and closes it once, rejecting late frames', async () => {
    const { transport, invoke } = setup()
    const next = deferred<unknown>()
    invoke.mockImplementation((name) =>
      name.endsWith('|next_event') ? next.promise : Promise.resolve(),
    )
    const consume = vi.fn()
    const fail = vi.fn()
    const close = transport.subscribe(['status'], consume, fail)
    await flush()
    expect(invoke.mock.calls.map(([name]) => name)).toEqual([
      'plugin:module-api|open_events',
      'plugin:module-api|next_event',
    ])
    close()
    close()
    next.resolve(event)
    await flush()
    expect(consume).not.toHaveBeenCalled()
    expect(fail).not.toHaveBeenCalled()
    expect(invoke.mock.calls.filter(([name]) => name.endsWith('|close_events'))).toHaveLength(1)
  })
  it('compensates cancellation before open resolves without starting a pull', async () => {
    const { transport, invoke } = setup()
    const open = deferred<unknown>()
    invoke.mockImplementation((name) =>
      name.endsWith('|open_events') ? open.promise : Promise.resolve(),
    )
    const consume = vi.fn()
    const fail = vi.fn()
    const close = transport.subscribe(['status'], consume, fail)
    close()
    close()
    expect(invoke.mock.calls.filter(([name]) => name.endsWith('|close_events'))).toHaveLength(1)
    open.resolve(undefined)
    await flush()
    expect(invoke.mock.calls.filter(([name]) => name.endsWith('|close_events'))).toHaveLength(2)
    expect(invoke.mock.calls.some(([name]) => name.endsWith('|next_event'))).toBe(false)
    expect(fail).not.toHaveBeenCalled()
  })
  it('replaces subscriptions and cannot close or publish into the replacement', async () => {
    const { transport, invoke } = setup()
    const pending = new Map<unknown, ReturnType<typeof deferred<unknown>>>()
    invoke.mockImplementation((name, args) => {
      if (!name.endsWith('|next_event')) return Promise.resolve()
      const next = deferred<unknown>()
      pending.set(args.subscriptionId, next)
      return next.promise
    })
    const oldConsume = vi.fn()
    const oldClose = transport.subscribe(['status'], oldConsume, vi.fn())
    await flush()
    const oldId = invoke.mock.calls[0]![1].subscriptionId
    const consume = vi.fn()
    const close = transport.subscribe(['status'], consume, vi.fn())
    await flush()
    oldClose()
    pending.get(oldId)!.resolve(event)
    await flush()
    expect(oldConsume).not.toHaveBeenCalled()
    expect(consume).not.toHaveBeenCalled()
    const closes = invoke.mock.calls.filter(([name]) => name.endsWith('|close_events'))
    expect(closes).toEqual([['plugin:module-api|close_events', { subscriptionId: oldId }]])
    close()
  })
  it.each([event, { name: 'traffic', body: {} }])(
    'delivers allowed frames and stops when consume disposes the session',
    async (frame) => {
      const { transport, invoke } = setup()
      invoke.mockResolvedValue(frame)
      const consume = vi.fn(() => close())
      const close = transport.subscribe(['status'], consume, vi.fn())
      await flush()
      expect(consume).toHaveBeenCalledExactlyOnceWith(frame)
      expect(invoke).toHaveBeenCalledTimes(3)
    },
  )
  it.each([
    { name: 'secrets', body: {} },
    { name: 'subscriptions', body: {} },
    { name: 'status', body: 'x'.repeat(4 * 1024 * 1024) },
  ])('closes malformed or unexpected streams without a private reconnect loop', async (frame) => {
    const { transport, invoke } = setup()
    invoke.mockResolvedValue(frame)
    const fail = vi.fn()
    const close = transport.subscribe(['status'], vi.fn(), fail)
    await flush()
    close()
    expect(fail).toHaveBeenCalledTimes(1)
    expect(invoke.mock.calls.map(([name]) => name)).toEqual([
      'plugin:module-api|open_events',
      'plugin:module-api|next_event',
      'plugin:module-api|close_events',
    ])
  })
  it('ignores rejection from a cancelled pending next_event', async () => {
    const { transport, invoke } = setup()
    const pending = deferred<unknown>()
    invoke.mockImplementation((name) =>
      name.endsWith('|next_event') ? pending.promise : Promise.resolve(),
    )
    const fail = vi.fn()
    const close = transport.subscribe(['status'], vi.fn(), fail)
    await flush()
    close()
    pending.reject(new Error('closed'))
    await flush()
    expect(fail).not.toHaveBeenCalled()
  })
})

describe('native visibility', () => {
  it('does not turn a newer lifecycle event into paused when the initial query fails late', async () => {
    const { transport, invoke, visibility } = setup()
    const initial = deferred<unknown>()
    invoke.mockReturnValue(initial.promise)
    const publish = vi.fn()
    const close = transport.observeVisibility(publish)
    await flush()
    visibility({ visible: true })
    initial.reject(new Error('late query failure'))
    await flush()
    expect(publish.mock.calls).toEqual([[true]])
    close()
  })
  it('registers before querying initial visibility and does not overwrite a newer event', async () => {
    const { transport, invoke, visibility, unregister } = setup()
    const initial = deferred<unknown>()
    invoke.mockReturnValue(initial.promise)
    const publish = vi.fn()
    const close = transport.observeVisibility(publish)
    await flush()
    visibility({ visible: false })
    initial.resolve({ visible: true })
    await flush()
    expect(publish.mock.calls).toEqual([[false]])
    visibility({ visible: true })
    visibility({ visible: true })
    expect(publish.mock.calls).toEqual([[false], [true]])
    close()
    close()
    visibility({ visible: false })
    expect(unregister).toHaveBeenCalledTimes(1)
    expect(publish).toHaveBeenCalledTimes(2)
  })
  it('releases a late listener after unmount and never requests the initial value', async () => {
    const { transport, listen, invoke, unregister } = setup()
    const registration = deferred<{ unregister(): Promise<void> }>()
    listen.mockReturnValue(registration.promise)
    const publish = vi.fn()
    const close = transport.observeVisibility(publish)
    close()
    registration.resolve({ unregister })
    await flush()
    expect(unregister).toHaveBeenCalledTimes(1)
    expect(invoke).not.toHaveBeenCalled()
    expect(publish).not.toHaveBeenCalled()
  })
  it('fails closed on registration or initial query failures', async () => {
    for (const at of ['listen', 'initial']) {
      const { transport, listen, invoke } = setup()
      if (at === 'listen') listen.mockRejectedValue(new Error('secret'))
      else invoke.mockRejectedValue(new Error('secret'))
      const publish = vi.fn()
      const close = transport.observeVisibility(publish)
      await flush()
      expect(publish.mock.calls).toEqual([[false]])
      close()
    }
  })
})
