import { expect, it, vi } from 'vitest'
import {
  androidImportPayload,
  createAndroidInputAdapter,
  detectImportKind,
  loadAndroidInput,
  type AndroidInputIpc,
} from './android-input'

const payload = {
  id: 'fixture',
  name: 'nodes.txt',
  content: 'ss://test-only-secret@localhost:1234',
}

function setup() {
  const listeners: Partial<Record<'import', (value: unknown) => void>> = {}
  const unregister = vi.fn(async () => {})
  const invoke = vi.fn<AndroidInputIpc['invoke']>(async (command, args) =>
    command.endsWith('|visibility') ? { visible: true } : args.completedId ? null : payload,
  )
  const listen = vi.fn<AndroidInputIpc['listen']>(async (event, callback) => {
    listeners[event] = callback
    return { unregister }
  })
  const adapter = createAndroidInputAdapter({
    invoke,
    listen,
  })
  return { adapter, invoke, listeners, unregister, listen }
}

it('uses only fixed content commands, forwards no paths, and ignores notification content', async () => {
  const h = setup()
  expect(await h.adapter.pickDocument('config')).toEqual(payload)
  expect(await h.adapter.takeImport()).toEqual(payload)
  await expect(h.adapter.completeImport(payload.id)).resolves.toBeUndefined()
  expect(h.invoke.mock.calls).toEqual([
    ['plugin:module-api|pick_document', { kind: 'config' }],
    ['plugin:module-api|take_import', {}],
    ['plugin:module-api|take_import', { completedId: payload.id }],
  ])
  const notify = vi.fn()
  const visible = vi.fn()
  const close = await h.adapter.listen(notify, visible)
  h.listeners.import?.({ content: 'must-never-forward-notification-content' })
  await vi.waitFor(() => expect(notify).toHaveBeenCalledTimes(1))
  expect(notify.mock.calls).toEqual([[]])
  expect(h.listen.mock.calls.map(([event]) => event)).toEqual(['import'])
  expect(visible).toHaveBeenCalledWith(true)
  close()
  expect(h.unregister).toHaveBeenCalledTimes(1)
  h.listeners.import?.({})
  expect(notify).toHaveBeenCalledTimes(1)
})

it('receives the same retained id in a recreated document until an explicit completion ACK', async () => {
  let documentId = 1
  let deliveredTo = 0
  let active: typeof payload | null = payload
  const invoke = vi.fn<AndroidInputIpc['invoke']>(async (command, args) => {
    expect(command).toBe('plugin:module-api|take_import')
    if (args.completedId !== undefined) {
      expect(args).toEqual({ completedId: payload.id })
      active = null
      return null
    }
    expect(args).toEqual({})
    if (deliveredTo === documentId) return null
    deliveredTo = documentId
    return active
  })
  const ipc: AndroidInputIpc = { invoke, listen: async () => ({ unregister: async () => {} }) }
  const original = createAndroidInputAdapter(ipc)
  expect(await original.takeImport()).toEqual(payload)
  expect(await original.takeImport()).toBeNull()
  documentId++
  const recreated = createAndroidInputAdapter(ipc)
  expect(await recreated.takeImport()).toEqual(payload)
  expect(invoke.mock.calls.every(([, args]) => args.completedId === undefined)).toBe(true)
  await recreated.completeImport(payload.id)
  documentId++
  expect(await createAndroidInputAdapter(ipc).takeImport()).toBeNull()
})

it('rejects invalid completion ids and non-null ACK responses without forwarding input', async () => {
  const h = setup()
  for (const id of ['', '../private', 'content://fixture', 'bad\nid', 'x'.repeat(257)])
    await expect(h.adapter.completeImport(id)).rejects.toMatchObject({ code: 'api.import_invalid' })
  expect(h.invoke).not.toHaveBeenCalled()
  h.invoke.mockResolvedValueOnce(payload)
  await expect(h.adapter.completeImport(payload.id)).rejects.toMatchObject({
    code: 'api.import_invalid',
  })
})

it('queries visibility on import wakes without registering a second visibility listener', async () => {
  const h = setup()
  let finish!: (value: { visible: boolean }) => void
  h.invoke.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve as typeof finish
      }),
  )
  h.invoke.mockResolvedValueOnce({ visible: false })
  const visible = vi.fn()
  const registration = h.adapter.listen(vi.fn(), visible)
  await vi.waitFor(() => expect(h.invoke).toHaveBeenCalled())
  h.listeners.import?.({})
  await vi.waitFor(() => expect(visible).toHaveBeenCalledWith(false))
  finish({ visible: true })
  const close = await registration
  expect(visible.mock.calls).toEqual([[false]])
  close()
})

it('accepts cancellation and exactly 1 MiB but rejects oversized UTF-8, malformed text and paths', () => {
  expect(androidImportPayload(null)).toBeNull()
  expect(
    androidImportPayload({ ...payload, content: 'a'.repeat(1024 * 1024) })?.content.length,
  ).toBe(1024 * 1024)
  expect(() => androidImportPayload({ ...payload, content: '界'.repeat(350000) })).toThrow()
  expect(() => androidImportPayload({ ...payload, content: '\ud800' })).toThrow()
  for (const extra of [{ uri: 'content://fixture' }, { path: '/private/file' }])
    expect(() => androidImportPayload({ ...payload, ...extra })).toThrow()
  for (const name of ['/private/file', 'content://fixture', 'C:\\secret', 'file\nname'])
    expect(() => androidImportPayload({ ...payload, name })).toThrow()
})

it.each(['api.import_unreadable', 'api.import_invalid', 'api.body_too_large', 'api.busy'])(
  'projects only the fixed %s error code',
  async (code) => {
    const h = setup()
    h.invoke.mockRejectedValue({
      code,
      message: 'content://private/fixture-secret',
      path: '/private/file',
    })
    await expect(h.adapter.takeImport()).rejects.toMatchObject({ code })
    await expect(h.adapter.takeImport()).rejects.not.toThrow(/private|secret/)
    await expect(h.adapter.completeImport(payload.id)).rejects.toMatchObject({ code })
    await expect(h.adapter.completeImport(payload.id)).rejects.not.toThrow(/private|secret/)
  },
)

it('does not enable input in an ordinary browser build', async () => {
  expect(await loadAndroidInput()).toBeNull()
})

it.each([
  ['https://example.invalid/sub?token=test-only', 'subscription'],
  ['http://localhost/sub', 'subscription'],
  [payload.content, 'nodes'],
  ['proxies:\n - name: fixture\n   type: ss', 'nodes'],
  ['{"route":{"rules":[]},"outbounds":[]}', 'config'],
  ['{"outbounds":[{"type":"socks"}]}', null],
  ['{"type":"socks"}', 'nodes'],
  ['c3M6Ly9maXh0dXJl', null],
  ['unrecognized text', null],
  ['file:///private/config.json', null],
  ['content://fixture', null],
])('classifies routing hints without parsing or fetching: %s', (content, kind) => {
  expect(detectImportKind(content)).toBe(kind)
})
