import { StrictMode, type ReactNode } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, expect, it, vi } from 'vitest'
import { ApiClient } from '@/api/client'
import { apiOperations, type ApiTransport } from '@/api/contracts'
import type { AndroidImportPayload, AndroidInputAdapter } from '@/api/android-input'
import { ApiContext, PreferencesContext } from '@/app/context'
import { useAndroidImport } from '@/app/android-import-context'
import { translator } from '@/app/i18n'
import { TooltipProvider } from '@/components/ui/tooltip'
import { AndroidImports } from './android-imports'

vi.mock('@/components/json-editor', () => ({
  JsonEditor: ({
    value,
    onChange,
    label,
  }: {
    value: string
    onChange: (value: string) => void
    label: string
  }) => (
    <textarea aria-label={label} value={value} onChange={(event) => onChange(event.target.value)} />
  ),
}))

afterEach(() => vi.unstubAllGlobals())

const nodePayload = {
  id: 'fixture-one',
  name: 'nodes.txt',
  content: 'ss://test-only-private@localhost:1234',
}
const configPayload = {
  id: 'fixture-config',
  name: 'config.json',
  content: '{"log":{"level":"debug"}}',
}
const ok = (data: unknown) => ({
  status: 200,
  body: { schema: 1, ok: true, code: 'fixture.ok', message: '', data },
})

function Controls() {
  const input = useAndroidImport()
  return (
    input.available && (
      <>
        <button disabled={input.busy} onClick={() => input.pick('config')}>
          Pick configuration
        </button>
        <button disabled={input.busy} onClick={() => input.pick('nodes')}>
          Pick nodes
        </button>
      </>
    )
  )
}

function setup(
  options: {
    payload?: AndroidImportPayload | null
    hidden?: boolean
    adapter?: null
    strict?: boolean
    registration?: Promise<void>
  } = {},
) {
  let notify = () => {}
  let visibility: (visible: boolean) => void = vi.fn()
  const release = vi.fn()
  const takeImport = vi.fn<AndroidInputAdapter['takeImport']>().mockResolvedValue(null)
  const completeImport = vi.fn<AndroidInputAdapter['completeImport']>().mockResolvedValue(undefined)
  if (options.payload) takeImport.mockResolvedValueOnce(options.payload)
  const pickDocument = vi.fn<AndroidInputAdapter['pickDocument']>().mockResolvedValue(nodePayload)
  const adapter: AndroidInputAdapter = {
    takeImport,
    completeImport,
    pickDocument,
    listen: vi.fn(async (onImport, onVisibility) => {
      notify = onImport
      visibility = onVisibility
      onVisibility(!options.hidden)
      await options.registration
      return release
    }),
  }
  const request = vi.fn<ApiTransport['request']>(async (input) => {
    if (input.path === '/config' && input.method === 'GET')
      return ok({
        target: 'singbox/config.json',
        revision: 'original-revision',
        content: '{"log":{"level":"info"}}',
      })
    if (input.path === '/config/validate' || input.path === '/config')
      return ok({ target: 'singbox/config.json', revision: 'updated-revision' })
    if (input.path === '/nodes/snapshot')
      return ok({
        groups: [{ group: { id: 'default', name: 'Local default', type: 'local' }, nodes: [] }],
      })
    if (input.path === '/nodes/import') return ok({ persisted: true })
    if (input.path === '/subscriptions' && input.method === 'POST') return ok({ id: 'fixture-new' })
    if (input.path === '/catalog' || input.path === '/subscriptions') return ok([])
    throw new Error('Unexpected request')
  })
  const transport: ApiTransport = {
    kind: 'native',
    capabilities: apiOperations,
    request,
    subscribe: () => () => {},
  }
  const cache = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  cache.setQueryData(['events-connection'], 'live')
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <PreferencesContext
        value={{
          preferences: { locale: 'en', theme: 'light' },
          t: translator('en'),
          update: () => {},
        }}
      >
        <ApiContext value={new ApiClient(transport)}>
          <QueryClientProvider client={cache}>
            <TooltipProvider>{children}</TooltipProvider>
          </QueryClientProvider>
        </ApiContext>
      </PreferencesContext>
    )
  }
  const contents = (
    <AndroidImports adapter={options.adapter === null ? null : adapter}>
      <Controls />
    </AndroidImports>
  )
  const view = render(
    options.strict ? (
      <StrictMode>
        <Wrapper>{contents}</Wrapper>
      </StrictMode>
    ) : (
      <Wrapper>{contents}</Wrapper>
    ),
  )
  return {
    view,
    adapter,
    cache,
    request,
    takeImport,
    completeImport,
    pickDocument,
    release,
    notify: () => notify(),
    visible: (value: boolean) => {
      visibility(value)
      notify()
    },
  }
}

const writes = (h: ReturnType<typeof setup>) =>
  h.request.mock.calls.filter(([input]) => input.method !== 'GET')

it('takes shared input once, previews readable names, and cancels without writes or caching', async () => {
  const h = setup({ payload: nodePayload })
  expect(await screen.findByRole('textbox', { name: 'Import content' })).toHaveValue(
    nodePayload.content,
  )
  expect(screen.getByText('nodes.txt')).toBeVisible()
  expect(screen.queryByText(nodePayload.id)).not.toBeInTheDocument()
  expect(h.takeImport).toHaveBeenCalledTimes(1)
  expect(writes(h)).toHaveLength(0)
  expect(JSON.stringify(h.cache.getQueryCache().getAll())).not.toContain('test-only-private')
  expect(h.cache.getMutationCache().getAll()).toHaveLength(0)
  expect(JSON.stringify(localStorage)).not.toContain('test-only-private')
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  await waitFor(() => expect(h.takeImport).toHaveBeenCalledTimes(2))
  expect(h.completeImport).toHaveBeenCalledExactlyOnceWith(nodePayload.id)
  h.view.unmount()
  expect(h.release).toHaveBeenCalledTimes(1)
})

it('waits for foreground, serializes notification bursts and never replaces an existing preview', async () => {
  const h = setup({ hidden: true })
  await waitFor(() => expect(h.adapter.listen).toHaveBeenCalled())
  act(() => h.notify())
  expect(h.takeImport).not.toHaveBeenCalled()
  let resolve!: (payload: AndroidImportPayload | null) => void
  h.takeImport.mockImplementationOnce(
    () =>
      new Promise((done) => {
        resolve = done
      }),
  )
  act(() => h.visible(true))
  await waitFor(() => expect(h.takeImport).toHaveBeenCalledTimes(1))
  act(() => {
    h.notify()
    h.notify()
  })
  expect(h.takeImport).toHaveBeenCalledTimes(1)
  await act(async () => resolve(nodePayload))
  await screen.findByRole('textbox', { name: 'Import content' })
  h.takeImport.mockResolvedValueOnce({ ...configPayload, id: 'fixture-two' })
  act(() => {
    h.visible(false)
    h.notify()
    h.visible(true)
  })
  expect(screen.getByRole('textbox', { name: 'Import content' })).toHaveValue(nodePayload.content)
  expect(h.takeImport).toHaveBeenCalledTimes(1)
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() =>
    expect(screen.getByRole('textbox', { name: 'Import content' })).toHaveValue(
      configPayload.content,
    ),
  )
  expect(h.takeImport).toHaveBeenCalledTimes(2)
})

it('drains cold queued shares on close even without another notification', async () => {
  const h = setup({ payload: nodePayload })
  h.takeImport.mockResolvedValueOnce(configPayload)
  await screen.findByRole('textbox', { name: 'Import content' })
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() =>
    expect(screen.getByRole('textbox', { name: 'Import content' })).toHaveValue(
      configPayload.content,
    ),
  )
  expect(h.takeImport).toHaveBeenCalledTimes(2)
})

it('waits for the close ACK before taking the next FIFO entry, including during wake bursts', async () => {
  const h = setup({ payload: nodePayload })
  h.takeImport.mockResolvedValueOnce(configPayload)
  let acknowledge!: () => void
  h.completeImport.mockImplementationOnce(
    () =>
      new Promise<void>((resolve) => {
        acknowledge = resolve
      }),
  )
  await screen.findByRole('textbox', { name: 'Import content' })
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(h.completeImport).toHaveBeenCalledExactlyOnceWith(nodePayload.id)
  act(() => {
    h.notify()
    h.visible(false)
    h.visible(true)
    h.notify()
  })
  expect(h.takeImport).toHaveBeenCalledTimes(1)
  await act(async () => acknowledge())
  await waitFor(() =>
    expect(screen.getByRole('textbox', { name: 'Import content' })).toHaveValue(
      configPayload.content,
    ),
  )
  expect(h.completeImport.mock.invocationCallOrder[0]).toBeLessThan(
    h.takeImport.mock.invocationCallOrder[1]!,
  )
  expect(writes(h)).toHaveLength(0)
})

it('keeps intake blocked after an ACK failure and retries only the ACK on explicit close', async () => {
  const h = setup({ payload: nodePayload })
  h.takeImport.mockResolvedValueOnce(configPayload)
  h.completeImport.mockRejectedValueOnce(new Error('content://private/test-only-secret'))
  await screen.findByRole('textbox', { name: 'Import content' })
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Could not finish closing the import.')
  expect(screen.getByRole('alert')).not.toHaveTextContent('test-only-secret')
  act(() => {
    h.notify()
    h.visible(false)
    h.visible(true)
  })
  expect(h.takeImport).toHaveBeenCalledTimes(1)
  expect(h.completeImport).toHaveBeenCalledTimes(1)
  expect(writes(h)).toHaveLength(0)
  fireEvent.click(screen.getByRole('button', { name: 'Close' }))
  await waitFor(() =>
    expect(screen.getByRole('textbox', { name: 'Import content' })).toHaveValue(
      configPayload.content,
    ),
  )
  expect(h.completeImport.mock.calls).toEqual([[nodePayload.id], [nodePayload.id]])
  expect(h.takeImport).toHaveBeenCalledTimes(2)
  expect(writes(h)).toHaveLength(0)
})

it('redelivers the same retained preview after recreation without acknowledging unmount', async () => {
  const original = setup({ payload: nodePayload })
  await screen.findByRole('textbox', { name: 'Import content' })
  original.view.unmount()
  expect(original.completeImport).not.toHaveBeenCalled()
  const recreated = setup({ payload: { ...nodePayload } })
  expect(await screen.findByRole('textbox', { name: 'Import content' })).toHaveValue(
    nodePayload.content,
  )
  expect(recreated.completeImport).not.toHaveBeenCalled()
  expect(writes(recreated)).toHaveLength(0)
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() =>
    expect(recreated.completeImport).toHaveBeenCalledExactlyOnceWith(nodePayload.id),
  )
})

it('keeps edits mounted and unchanged when a later share or resume wakes the app', async () => {
  const h = setup({ payload: configPayload })
  fireEvent.click(await screen.findByRole('button', { name: 'Continue' }))
  const editor = await screen.findByRole('textbox', { name: 'Import core configuration JSON' })
  fireEvent.change(editor, { target: { value: '{"log":{"level":"trace"}}' } })
  act(() => {
    h.notify()
    h.visible(false)
    h.visible(true)
  })
  expect(screen.getByRole('textbox', { name: 'Import core configuration JSON' })).toBe(editor)
  expect(editor).toHaveValue('{"log":{"level":"trace"}}')
  expect(h.takeImport).toHaveBeenCalledTimes(1)
  expect(h.request.mock.calls.filter(([input]) => input.method === 'GET')).toHaveLength(1)
  expect(h.completeImport).not.toHaveBeenCalled()
})

it('routes nodes to the existing default import without a synthetic group or automatic writes', async () => {
  const h = setup({ payload: nodePayload })
  fireEvent.click(await screen.findByRole('button', { name: 'Continue' }))
  const editor = await screen.findByRole('textbox', { name: 'Node content' })
  expect(editor).toHaveValue(nodePayload.content)
  expect(writes(h)).toHaveLength(0)
  const save = screen.getByRole('button', { name: 'Save' })
  await waitFor(() => expect(save).toBeEnabled())
  fireEvent.click(save)
  await screen.findByText('Operation completed. Nodes refreshed.')
  expect(writes(h)).toHaveLength(1)
  expect(writes(h)[0]?.[0]).toMatchObject({
    path: '/nodes/import',
    body: { content: nodePayload.content, allow_insecure: false },
  })
  expect(writes(h)[0]?.[0].body).not.toHaveProperty('group_id')
  expect(JSON.stringify(h.cache.getQueryCache().getAll())).not.toContain('test-only-private')
  expect(h.cache.getMutationCache().getAll()).toHaveLength(0)
})

it('uses the original config revision and requires explicit overwrite plus successful validation', async () => {
  const h = setup({ payload: configPayload })
  fireEvent.click(await screen.findByRole('button', { name: 'Continue' }))
  expect(
    await screen.findByRole('textbox', { name: 'Import core configuration JSON' }),
  ).toHaveValue(configPayload.content)
  expect(writes(h)).toHaveLength(0)
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await screen.findByText('Overwrite current configuration?')
  expect(writes(h)).toHaveLength(0)
  fireEvent.click(screen.getByRole('button', { name: 'Confirm overwrite' }))
  await screen.findByText('Saved')
  expect(writes(h).map(([input]) => [input.method, input.path])).toEqual([
    ['POST', '/config/validate'],
    ['PUT', '/config'],
  ])
  expect(writes(h)[1]?.[0].body).toEqual({
    content: configPayload.content,
    revision: 'original-revision',
  })
  expect(h.cache.getMutationCache().getAll()).toHaveLength(0)
})

it('prefills subscription URL without downloading and waits for explicit Save', async () => {
  const url = 'https://example.invalid/sub?token=test-only-subscription'
  const h = setup({ payload: { id: 'fixture-url', name: 'Shared text', content: url } })
  fireEvent.click(await screen.findByRole('button', { name: 'Continue' }))
  expect(await screen.findByRole('textbox', { name: 'Subscription URL' })).toHaveValue(url)
  expect(h.request).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(writes(h)).toHaveLength(1))
  expect(writes(h)[0]?.[0]).toMatchObject({ method: 'POST', path: '/subscriptions', body: { url } })
  expect(JSON.stringify(h.cache.getQueryCache().getAll())).not.toContain('test-only-subscription')
  expect(h.cache.getMutationCache().getAll()).toHaveLength(0)
})

it('does not overwrite when imported config validation fails', async () => {
  const h = setup({ payload: configPayload })
  const original = h.request.getMockImplementation()!
  h.request.mockImplementation((input, signal) =>
    input.path === '/config/validate'
      ? Promise.resolve({
          status: 400,
          body: {
            schema: 1,
            ok: false,
            code: 'config.invalid',
            message: 'private fixture details',
            data: {},
          },
        })
      : original(input, signal),
  )
  fireEvent.click(await screen.findByRole('button', { name: 'Continue' }))
  await screen.findByRole('textbox', { name: 'Import core configuration JSON' })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  fireEvent.click(await screen.findByRole('button', { name: 'Confirm overwrite' }))
  await screen.findByRole('alert')
  expect(writes(h).map(([input]) => input.path)).toEqual(['/config/validate'])
  expect(screen.getByRole('textbox', { name: 'Import core configuration JSON' })).toHaveValue(
    configPayload.content,
  )
})

it('requires a user choice for base64 or Provider JSON without parsing it itself', async () => {
  const h = setup({ payload: { ...nodePayload, content: 'c3M6Ly9maXh0dXJl' } })
  expect(await screen.findByRole('button', { name: 'Continue' })).toBeDisabled()
  fireEvent.click(screen.getByRole('radio', { name: 'Node text' }))
  fireEvent.click(screen.getByRole('button', { name: 'Continue' }))
  expect(await screen.findByRole('textbox', { name: 'Node content' })).toHaveValue(
    'c3M6Ly9maXh0dXJl',
  )
  expect(writes(h)).toHaveLength(0)
})

it('allows a fake file adapter, ignores picker cancellation and hides controls without capability', async () => {
  const h = setup()
  const pick = await screen.findByRole('button', { name: 'Pick nodes' })
  await waitFor(() => expect(pick).toBeEnabled())
  h.pickDocument.mockResolvedValueOnce(null)
  fireEvent.click(pick)
  await waitFor(() => expect(h.pickDocument).toHaveBeenCalledWith('nodes'))
  await waitFor(() => expect(pick).toBeEnabled())
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(h.completeImport).not.toHaveBeenCalled()
  fireEvent.click(pick)
  expect(await screen.findByRole('textbox', { name: 'Import content' })).toHaveValue(
    nodePayload.content,
  )
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(h.completeImport).toHaveBeenCalledExactlyOnceWith(nodePayload.id))
  h.view.unmount()
  const browser = setup({ adapter: null })
  expect(screen.queryByRole('button', { name: 'Pick nodes' })).not.toBeInTheDocument()
  expect(browser.takeImport).not.toHaveBeenCalled()
})

it('ignores a late take after unmount', async () => {
  const h = setup()
  await waitFor(() => expect(h.takeImport).toHaveBeenCalledTimes(1))
  let resolve!: (payload: AndroidImportPayload | null) => void
  h.takeImport.mockImplementationOnce(
    () =>
      new Promise((done) => {
        resolve = done
      }),
  )
  act(() => h.notify())
  h.view.unmount()
  await act(async () => resolve(nodePayload))
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(h.release).toHaveBeenCalledTimes(1)
  expect(h.completeImport).not.toHaveBeenCalled()
})

it('unregisters a late registration without consuming after unmount', async () => {
  let finish!: () => void
  const h = setup({
    registration: new Promise<void>((resolve) => {
      finish = resolve
    }),
  })
  await waitFor(() => expect(h.adapter.listen).toHaveBeenCalledTimes(1))
  h.view.unmount()
  await act(async () => finish())
  expect(h.release).toHaveBeenCalledTimes(1)
  expect(h.takeImport).not.toHaveBeenCalled()
})

it('does not consume twice under StrictMode listener setup and cleanup', async () => {
  const h = setup({ payload: nodePayload, strict: true })
  await screen.findByRole('textbox', { name: 'Import content' })
  expect(h.takeImport).toHaveBeenCalledTimes(1)
  h.view.unmount()
  expect(h.release).toHaveBeenCalledTimes(2)
  expect(h.completeImport).not.toHaveBeenCalled()
})
