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
  it.each([
    '/info',
    '/service',
    '/nodes/snapshot',
    '/selection',
    '/mode',
    '/catalog',
    '/subscriptions',
  ] as const)('maps %s to a resource, never a URL or secret', async (path) => {
    const { transport, invoke } = setup()
    const response = { status: 200, body: event.body }
    invoke.mockResolvedValue(response)
    expect(await transport.request({ method: 'GET', path })).toEqual(response)
    expect(invoke).toHaveBeenCalledExactlyOnceWith('plugin:module-api|read', {
      requestId: expect.any(String),
      resource: path === '/nodes/snapshot' ? 'nodes' : path.slice(1),
    })
    expect(transport.capabilities).toEqual([
      'info.read',
      'service.read',
      'service.start',
      'service.stop',
      'nodes.read',
      'selection.read',
      'selection.write',
      'mode.read',
      'mode.write',
      'nodes.delay',
      'catalog.read',
      'subscriptions.read',
      'subscriptions.detail',
      'subscriptions.history',
      'subscriptions.progress',
      'subscriptions.update',
      'subscriptions.cancel',
      'subscriptions.add',
      'subscriptions.edit',
      'subscriptions.remove',
      'subscriptions.update_all',
    ])
  })
  it.each([
    { method: 'POST', path: '/service/restart' },
    { method: 'POST', path: '/service/reload' },
    { method: 'POST', path: '/service/check' },
    { method: 'POST', path: '/service/start', body: {} },
    { method: 'PUT', path: '/selection', body: {} },
    { method: 'DELETE', path: '/subscriptions' },
    { method: 'POST', path: '/nodes/snapshot' },
    { method: 'GET', path: '/subscriptions', query: { private: 'true' } },
    { method: 'GET', path: '/subscriptions/default?private=true' },
    { method: 'GET', path: '/subscriptions/default', query: { private: 'true' } },
    { method: 'GET', path: '/subscriptions/a%2Fb' },
    { method: 'GET', path: '/subscriptions/../default' },
    { method: 'GET', path: '/subscriptions/staging' },
    { method: 'GET', path: '/subscriptions/a..b' },
    { method: 'POST', path: '/subscriptions' },
    { method: 'PUT', path: '/subscriptions/default' },
    { method: 'DELETE', path: '/subscriptions/default', body: {} },
    { method: 'POST', path: '/subscriptions/default/update', body: {} },
    { method: 'POST', path: '/subscriptions/default/cancel', query: { private: 'true' } },
    { method: 'POST', path: '/subscriptions/update-all', body: {} },
    { method: 'GET', path: '/config' },
    { method: 'GET', path: '/info', query: {} },
    { method: 'GET', path: '/info', body: null },
    { method: 'GET', path: 'https://example.invalid/' },
    { method: 'GET', path: '__proto__' },
  ])('rejects unauthorized resources before IPC: %j', async (request) => {
    const { transport, invoke } = setup()
    await expect(transport.request(request as ApiRequest)).rejects.toMatchObject({
      code: 'client.operation_denied',
      status: 403,
    })
    expect(invoke).not.toHaveBeenCalled()
  })
  it.each([
    [
      {
        method: 'POST',
        path: '/subscriptions',
        body: { url: 'https://source.example.invalid/feed', custom_headers: {} },
      },
      { action: 'add', input: { url: 'https://source.example.invalid/feed', custom_headers: {} } },
    ],
    [
      {
        method: 'PUT',
        path: '/subscriptions/demo-global',
        body: { user_agent: '', auto_update: false },
      },
      { action: 'edit', groupId: 'demo-global', input: { user_agent: '', auto_update: false } },
    ],
    [
      {
        method: 'DELETE',
        path: '/subscriptions/demo-global',
        query: { replacement_group_id: 'default' },
      },
      { action: 'remove', groupId: 'demo-global', replacementGroupId: 'default' },
    ],
    [{ method: 'POST', path: '/subscriptions/update-all' }, { action: 'update_all' }],
  ] as const)('uses a fixed typed subscription write: %j', async (request, args) => {
    const { transport, invoke } = setup()
    invoke.mockResolvedValue({ status: 200, body: event.body })
    await transport.request(request)
    expect(invoke).toHaveBeenCalledExactlyOnceWith('plugin:module-api|subscription_write', {
      requestId: expect.any(String),
      ...args,
    })
  })
  it.each([
    {
      method: 'POST',
      path: '/subscriptions',
      body: { url: 'https://source.example.invalid/feed', method: 'POST' },
    },
    { method: 'PUT', path: '/subscriptions/demo-global', body: { url: null } },
    { method: 'PUT', path: '/subscriptions/demo-global', body: { update_interval: 1 } },
    {
      method: 'PUT',
      path: '/subscriptions/demo-global',
      body: { custom_headers: { 'Bad Header': 'value' } },
    },
    {
      method: 'PUT',
      path: '/subscriptions/demo-global',
      body: { custom_headers: { Authorization: 'bad\r\nvalue' } },
    },
    { method: 'PUT', path: '/subscriptions/demo-global', body: {}, query: { private: 'true' } },
    {
      method: 'DELETE',
      path: '/subscriptions/demo-global',
      query: { replacement_group_id: 'demo-global' },
    },
    {
      method: 'DELETE',
      path: '/subscriptions/demo-global',
      query: { replacement_group_id: '../default' },
    },
    { method: 'DELETE', path: '/subscriptions/demo-global', query: { private: 'true' } },
  ])('rejects malformed subscription writes before IPC: %j', async (request) => {
    const { transport, invoke } = setup()
    await expect(transport.request(request as ApiRequest)).rejects.toMatchObject({
      code: 'client.operation_denied',
    })
    expect(invoke).not.toHaveBeenCalled()
  })
  it.each(['update', 'cancel'] as const)(
    'uses the fixed subscription %s command without HTTP parameters',
    async (action) => {
      const { transport, invoke } = setup()
      invoke.mockResolvedValue({ status: 200, body: event.body })
      await transport.request({ method: 'POST', path: `/subscriptions/demo-global/${action}` })
      expect(invoke).toHaveBeenCalledExactlyOnceWith('plugin:module-api|subscription_action', {
        requestId: expect.any(String),
        groupId: 'demo-global',
        action,
      })
    },
  )
  it.each(['detail', 'history', 'progress'] as const)(
    'uses a typed public %s resource and an original group ID',
    async (resource) => {
      const { transport, invoke } = setup()
      invoke.mockResolvedValue({ status: 200, body: event.body })
      await transport.request({
        method: 'GET',
        path: `/subscriptions/a.b-C_1${resource === 'detail' ? '' : `/${resource}`}`,
      })
      expect(invoke).toHaveBeenCalledExactlyOnceWith('plugin:module-api|subscription_read', {
        requestId: expect.any(String),
        groupId: 'a.b-C_1',
        resource,
      })
    },
  )
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
      client.request({ method: 'PUT', path: '/selection', body: {} }, z.unknown()),
    ).rejects.toMatchObject({ code: 'client.operation_denied' })
    expect(invoke).toHaveBeenCalledTimes(2)
  })
  it.each(['start', 'stop'] as const)(
    'maps only service.%s to the restricted write command',
    async (action) => {
      const { transport, invoke } = setup()
      invoke.mockResolvedValue({ status: 200, body: event.body })
      await transport.request({ method: 'POST', path: `/service/${action}` })
      expect(invoke).toHaveBeenCalledExactlyOnceWith('plugin:module-api|service_action', {
        requestId: expect.any(String),
        action,
      })
    },
  )
  it.each([
    ['PUT', '/selection', 'select_node'],
    ['POST', '/nodes/delay', 'test_delay'],
  ] as const)(
    'maps %s %s to a fixed typed command preserving original tags',
    async (method, path, name) => {
      const { transport, invoke } = setup()
      invoke.mockResolvedValue({ status: 200, body: event.body })
      for (const node_tag of ['', '香港 / 节点🚀', 'Auto/default']) {
        const target = { group_id: 'a.b-C_1', node_tag }
        await transport.request({ method, path, body: target })
        expect(invoke).toHaveBeenLastCalledWith(`plugin:module-api|${name}`, {
          requestId: expect.any(String),
          target,
        })
      }
      expect(invoke).toHaveBeenCalledTimes(3)
    },
  )
  it('forwards native mode names without a fixed enumeration', async () => {
    const { transport, invoke } = setup()
    invoke.mockResolvedValue({ status: 200, body: event.body })
    await transport.request({
      method: 'PUT',
      path: '/mode',
      body: { mode: '规则（允许广告）/自定义' },
    })
    expect(invoke).toHaveBeenCalledExactlyOnceWith('plugin:module-api|change_mode', {
      requestId: expect.any(String),
      mode: '规则（允许广告）/自定义',
    })
  })
  it.each([
    { group_id: 'staging', node_tag: '' },
    { group_id: 'a..b', node_tag: '' },
    { group_id: '../default', node_tag: '' },
    { group_id: 'a/b', node_tag: '' },
    { group_id: '_a', node_tag: '' },
    { group_id: ' a', node_tag: '' },
    { group_id: 'default' },
    { group_id: 'default', node_tag: 1 },
    { group_id: 'default', node_tag: null },
    { group_id: 'default', node_tag: '', path: '/config' },
    null,
  ])('rejects malformed targets independently of ApiClient: %j', async (body) => {
    const { transport, invoke } = setup()
    for (const request of [
      { method: 'PUT', path: '/selection', body },
      { method: 'POST', path: '/nodes/delay', body },
    ] as ApiRequest[]) {
      await expect(transport.request(request)).rejects.toMatchObject({
        code: 'client.operation_denied',
      })
    }
    expect(invoke).not.toHaveBeenCalled()
  })
  it.each([
    { method: 'PUT', path: '/mode', body: { mode: '' } },
    { method: 'PUT', path: '/mode', body: { mode: '   ' } },
    { method: 'PUT', path: '/mode', body: { mode: 1 } },
    { method: 'PUT', path: '/mode', body: { mode: 'Rule', url: 'https://example.invalid' } },
    { method: 'PUT', path: '/selection', query: {}, body: { group_id: 'default', node_tag: '' } },
    { method: 'GET', path: '/selection', query: {} },
    { method: 'GET', path: '/mode', body: {} },
    { method: 'GET', path: '/nodes/snapshot', body: {} },
    { method: 'PUT', path: '/nodes/delay', body: { group_id: 'default', node_tag: '' } },
    { method: 'POST', path: '/mode', body: { mode: 'Rule' } },
  ])('rejects invalid mode bodies and method/query bypasses: %j', async (request) => {
    const { transport, invoke } = setup()
    await expect(transport.request(request as ApiRequest)).rejects.toMatchObject({
      code: 'client.operation_denied',
    })
    expect(invoke).not.toHaveBeenCalled()
  })
  it('bounds encoded Unicode and escaped request bodies before IPC', async () => {
    const { transport, invoke } = setup()
    for (const request of [
      {
        method: 'PUT',
        path: '/selection',
        body: { group_id: 'default', node_tag: '龙'.repeat(1_400_000) },
      },
      { method: 'PUT', path: '/mode', body: { mode: '\n'.repeat(2_100_000) + 'Rule' } },
    ] as ApiRequest[]) {
      await expect(transport.request(request)).rejects.toMatchObject({
        code: 'api.body_too_large',
        status: 413,
      })
    }
    expect(invoke).not.toHaveBeenCalled()
  })
  it.each([
    { method: 'PUT', path: '/selection', body: { group_id: 'default', node_tag: '' } },
    { method: 'PUT', path: '/mode', body: { mode: 'Rule' } },
    { method: 'POST', path: '/nodes/delay', body: { group_id: 'default', node_tag: '' } },
  ] as const)(
    'does not replay M4.2 operations after errors, cancellation or late replies: %j',
    async (operation) => {
      const { transport, invoke } = setup()
      invoke.mockRejectedValue({ code: 'api.connection_failed', status: 503, message: 'private' })
      await expect(transport.request(operation)).rejects.toMatchObject({
        code: 'api.connection_failed',
      })
      expect(invoke).toHaveBeenCalledOnce()
      invoke.mockClear()
      const pending = deferred<unknown>()
      invoke.mockImplementation((name) =>
        name.endsWith('|cancel') ? Promise.resolve() : pending.promise,
      )
      const abort = new AbortController()
      const request = transport.request(operation, abort.signal)
      const rejected = expect(request).rejects.toMatchObject({ name: 'AbortError' })
      abort.abort()
      await rejected
      pending.resolve({ status: 200, body: event.body })
      await flush()
      expect(invoke).toHaveBeenCalledTimes(2)
      expect(invoke.mock.calls[1]![0]).toBe('plugin:module-api|cancel')
    },
  )
  it.each([401, 403, 409, 502, 504])(
    'preserves service error %s and committed metadata without replay',
    async (status) => {
      const { transport, invoke } = setup()
      invoke.mockResolvedValue({
        status,
        body: {
          schema: 1,
          ok: false,
          code: 'service.failed',
          message: '',
          data: { persisted: true },
        },
      })
      await expect(
        new ApiClient(transport).request({ method: 'POST', path: '/service/start' }, z.unknown()),
      ).rejects.toMatchObject({ status, code: 'service.failed', persisted: true })
      expect(invoke).toHaveBeenCalledTimes(1)
    },
  )
  it('cancels a service wait without replaying it or accepting its late result', async () => {
    const { transport, invoke } = setup()
    const reply = deferred<unknown>()
    invoke.mockImplementation((name) =>
      name.endsWith('|service_action') ? reply.promise : Promise.resolve(),
    )
    const controller = new AbortController()
    const request = transport.request({ method: 'POST', path: '/service/start' }, controller.signal)
    const rejected = expect(request).rejects.toMatchObject({ name: 'AbortError' })
    controller.abort()
    await rejected
    reply.resolve({ status: 200, body: event.body })
    await flush()
    expect(invoke.mock.calls.filter(([name]) => name.endsWith('|service_action'))).toHaveLength(1)
    expect(invoke.mock.calls.filter(([name]) => name.endsWith('|cancel'))).toHaveLength(1)
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

  it('does not overwrite a subscription created reentrantly by native cleanup', async () => {
    const { transport, invoke } = setup()
    const pending = deferred<unknown>()
    let replacement: (() => void) | undefined
    let reenter = false
    invoke.mockImplementation((name) => {
      if (name.endsWith('|next_event')) return pending.promise
      if (name.endsWith('|close_events') && reenter) {
        reenter = false
        replacement = transport.subscribe(['groups'], vi.fn(), vi.fn())
      }
      return Promise.resolve()
    })
    transport.subscribe(['status'], vi.fn(), vi.fn())
    await flush()
    reenter = true
    const superseded = transport.subscribe(['subscriptions'], vi.fn(), vi.fn())
    await flush()
    expect(invoke.mock.calls.filter(([name]) => name.endsWith('|open_events'))).toEqual([
      ['plugin:module-api|open_events', { subscriptionId: expect.any(String), topics: ['status'] }],
      ['plugin:module-api|open_events', { subscriptionId: expect.any(String), topics: ['groups'] }],
    ])
    superseded()
    expect(invoke.mock.calls.filter(([name]) => name.endsWith('|close_events'))).toHaveLength(1)
    replacement!()
    pending.resolve(event)
    await flush()
    expect(invoke.mock.calls.filter(([name]) => name.endsWith('|close_events'))).toHaveLength(2)
  })

  it('does not notify an obsolete error callback after cleanup creates a replacement', async () => {
    const { transport, invoke } = setup()
    const pending = deferred<unknown>()
    let replacement: (() => void) | undefined
    invoke.mockImplementation((name) => {
      if (name.endsWith('|next_event')) return pending.promise
      if (name.endsWith('|close_events') && !replacement)
        replacement = transport.subscribe(['groups'], vi.fn(), vi.fn())
      return Promise.resolve()
    })
    const fail = vi.fn()
    transport.subscribe(['status'], vi.fn(), fail)
    await flush()
    pending.reject(new Error('old stream failed'))
    await flush()
    expect(fail).not.toHaveBeenCalled()
    replacement!()
  })

  it('contains throwing consumer and error callbacks and closes native work once', async () => {
    const { transport, invoke } = setup()
    invoke.mockResolvedValue(event)
    const fail = vi.fn(() => {
      throw new Error('callback failed')
    })
    const close = transport.subscribe(
      ['status'],
      () => {
        throw new Error('consumer failed')
      },
      fail,
    )
    await flush()
    close()
    expect(fail).toHaveBeenCalledOnce()
    expect(invoke.mock.calls.filter(([name]) => name.endsWith('|close_events'))).toHaveLength(1)
    expect(invoke.mock.calls.filter(([name]) => name.endsWith('|open_events'))).toHaveLength(1)
  })
})

describe('native visibility', () => {
  it('replaces visibility observers and releases late old registrations without affecting the new one', async () => {
    const { transport, listen, invoke } = setup()
    const first = deferred<{ unregister(): Promise<void> }>()
    const oldUnregister = vi.fn().mockResolvedValue(undefined)
    const newUnregister = vi.fn().mockResolvedValue(undefined)
    const callbacks: ((event: unknown) => void)[] = []
    listen.mockImplementation((callback) => {
      callbacks.push(callback)
      return callbacks.length === 1 ? first.promise : Promise.resolve({ unregister: newUnregister })
    })
    invoke.mockResolvedValue({ visible: true })
    const oldPublish = vi.fn()
    const oldClose = transport.observeVisibility(oldPublish)
    await flush()
    const publish = vi.fn()
    const close = transport.observeVisibility(publish)
    await flush()
    first.resolve({ unregister: oldUnregister })
    await flush()
    oldClose()
    callbacks[0]!({ visible: false })
    callbacks[1]!({ visible: false })
    expect(oldPublish).not.toHaveBeenCalled()
    expect(publish.mock.calls).toEqual([[true], [false]])
    expect(oldUnregister).toHaveBeenCalledOnce()
    expect(newUnregister).not.toHaveBeenCalled()
    expect(invoke).toHaveBeenCalledOnce()
    close()
    close()
    expect(newUnregister).toHaveBeenCalledOnce()
  })

  it('contains visibility consumer exceptions without publishing a synthetic failure', async () => {
    const { transport, invoke, visibility, unregister } = setup()
    invoke.mockResolvedValue({ visible: true })
    const publish = vi.fn(() => {
      throw new Error('consumer failed')
    })
    const close = transport.observeVisibility(publish)
    await flush()
    expect(() => visibility({ visible: false })).not.toThrow()
    expect(publish.mock.calls).toEqual([[true], [false]])
    close()
    expect(unregister).toHaveBeenCalledOnce()
  })

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
    await flush()
    close()
    registration.resolve({ unregister })
    await flush()
    expect(unregister).toHaveBeenCalledTimes(1)
    expect(invoke).not.toHaveBeenCalled()
    expect(publish).not.toHaveBeenCalled()
  })

  it('does not allocate a listener when disposed before registration starts', async () => {
    const { transport, listen, invoke } = setup()
    const close = transport.observeVisibility(vi.fn())
    close()
    await flush()
    expect(listen).not.toHaveBeenCalled()
    expect(invoke).not.toHaveBeenCalled()
  })

  it('preserves visibility delivered before listener registration resolves', async () => {
    const { transport, listen, invoke, unregister } = setup()
    const registration = deferred<{ unregister(): Promise<void> }>()
    listen.mockImplementation((callback) => {
      callback({ visible: false })
      return registration.promise
    })
    const publish = vi.fn()
    const close = transport.observeVisibility(publish)
    await flush()
    registration.resolve({ unregister })
    await flush()
    expect(publish.mock.calls).toEqual([[false]])
    expect(invoke).not.toHaveBeenCalled()
    close()
    expect(unregister).toHaveBeenCalledOnce()
  })

  it('fails closed when listener registration throws synchronously', async () => {
    const { transport, listen, invoke } = setup()
    listen.mockImplementation(() => {
      throw new Error('registration unavailable')
    })
    const publish = vi.fn()
    const close = transport.observeVisibility(publish)
    await flush()
    expect(publish.mock.calls).toEqual([[false]])
    expect(invoke).not.toHaveBeenCalled()
    close()
  })

  it.each(['synchronous', 'rejected'] as const)(
    'contains %s listener cleanup errors',
    async (kind) => {
      const { transport, invoke, unregister, visibility } = setup()
      invoke.mockResolvedValue({ visible: true })
      if (kind === 'synchronous')
        unregister.mockImplementation(() => {
          throw new Error('gone')
        })
      else unregister.mockRejectedValue(new Error('gone'))
      const publish = vi.fn()
      const close = transport.observeVisibility(publish)
      await flush()
      expect(() => {
        close()
        close()
      }).not.toThrow()
      visibility({ visible: false })
      await flush()
      expect(unregister).toHaveBeenCalledOnce()
      expect(publish.mock.calls).toEqual([[true]])
    },
  )
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

describe('same-WebView document teardown', () => {
  it('pagehide cancels reads without AbortSignals and discards late replies', async () => {
    const { transport, invoke } = setup()
    const pending = deferred<unknown>()
    invoke.mockImplementation((name) =>
      name.endsWith('|read') ? pending.promise : Promise.resolve(),
    )
    const read = transport.request({ method: 'GET', path: '/info' })
    const failure = expect(read).rejects.toMatchObject({ name: 'AbortError' })
    window.dispatchEvent(new Event('pagehide'))
    window.dispatchEvent(new Event('pagehide'))
    await failure
    pending.resolve({ status: 200, body: event.body })
    await flush()
    expect(invoke.mock.calls.map(([name]) => name)).toEqual([
      'plugin:module-api|read',
      'plugin:module-api|cancel',
    ])
  })

  it('pagehide closes a pending stream and unregisters visibility without reconnecting', async () => {
    const { transport, invoke, visibility, unregister } = setup()
    const next = deferred<unknown>()
    invoke.mockImplementation((name) => {
      if (name.endsWith('|next_event')) return next.promise
      if (name.endsWith('|visibility')) return Promise.resolve({ visible: true })
      return Promise.resolve()
    })
    const consume = vi.fn()
    const fail = vi.fn()
    const publish = vi.fn()
    const close = transport.subscribe(['status'], consume, fail)
    const release = transport.observeVisibility(publish)
    await flush()
    window.dispatchEvent(new Event('pagehide'))
    window.dispatchEvent(new Event('pagehide'))
    close()
    release()
    visibility({ visible: false })
    next.resolve(event)
    await flush()
    expect(consume).not.toHaveBeenCalled()
    expect(fail).not.toHaveBeenCalled()
    expect(publish.mock.calls).toEqual([[true]])
    expect(unregister).toHaveBeenCalledOnce()
    expect(invoke.mock.calls.filter(([name]) => name.endsWith('|close_events'))).toHaveLength(1)
    expect(invoke.mock.calls.filter(([name]) => name.endsWith('|open_events'))).toHaveLength(1)
  })

  it('pagehide compensates late open and registration without starting old-document work', async () => {
    const { transport, invoke, listen, unregister } = setup()
    const opening = deferred<unknown>()
    const registration = deferred<{ unregister(): Promise<void> }>()
    invoke.mockImplementation((name) =>
      name.endsWith('|open_events') ? opening.promise : Promise.resolve(),
    )
    listen.mockReturnValue(registration.promise)
    const consume = vi.fn()
    const fail = vi.fn()
    const publish = vi.fn()
    transport.subscribe(['status'], consume, fail)
    transport.observeVisibility(publish)
    await flush()
    window.dispatchEvent(new Event('pagehide'))
    opening.resolve(undefined)
    registration.resolve({ unregister })
    await flush()
    expect(invoke.mock.calls.map(([name]) => name)).toEqual([
      'plugin:module-api|open_events',
      'plugin:module-api|close_events',
      'plugin:module-api|close_events',
    ])
    expect(unregister).toHaveBeenCalledOnce()
    expect(consume).not.toHaveBeenCalled()
    expect(fail).not.toHaveBeenCalled()
    expect(publish).not.toHaveBeenCalled()
  })

  it('normal completion and disposal remove their pagehide handlers', async () => {
    const { transport, invoke } = setup()
    const add = vi.spyOn(window, 'addEventListener')
    const remove = vi.spyOn(window, 'removeEventListener')
    invoke.mockResolvedValue({ status: 200, body: event.body })
    await transport.request({ method: 'GET', path: '/info' })
    const close = transport.subscribe(['status'], vi.fn(), vi.fn())
    close()
    const release = transport.observeVisibility(vi.fn())
    release()
    await flush()
    const handlers = add.mock.calls
      .filter(([name]) => name === 'pagehide')
      .map(([, handler]) => handler)
    expect(handlers).toHaveLength(3)
    for (const handler of handlers) expect(remove).toHaveBeenCalledWith('pagehide', handler)
    add.mockRestore()
    remove.mockRestore()
  })
})
