import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { RouterProvider } from '@tanstack/react-router'
import { afterEach, expect, it, vi } from 'vitest'
import { ApiClient } from '@/api/client'
import type { ApiTransport } from '@/api/contracts'
import { NativeTransport } from '@/api/native'
import { useChangeMode, useSelectNode, useServiceAction } from '@/api/queries'
import * as resourceApi from '@/api/resources'
import { AppProviders } from '@/app/providers'
import { makeRouter } from '@/app/router'
import { MockTransport } from '@/mocks/transport'

afterEach(() => vi.restoreAllMocks())
type HarnessOptions = {
  readOnly?: boolean
  controls?: boolean
  handle?: (
    input: Parameters<ApiTransport['request']>[0],
    signal: AbortSignal | undefined,
    backend: MockTransport,
  ) => ReturnType<ApiTransport['request']> | undefined
}
function ControlProbe() {
  const service = useServiceAction()
  const mode = useChangeMode()
  const selection = useSelectNode()
  return (
    <>
      <button onClick={() => service.mutate('stop')}>test-stop</button>
      <button onClick={() => mode.mutate('Direct')}>test-mode</button>
      <button onClick={() => selection.mutate({ group_id: 'demo-global', node_tag: 'Tokyo 01' })}>
        test-selection
      </button>
    </>
  )
}
function setup(list: 'normal' | 'empty' | 'error' = 'normal', options: HarnessOptions = {}) {
  const backend = new MockTransport(0)
  const request = vi.fn<ApiTransport['request']>((input, signal) => {
    const handled = options.handle?.(input, signal, backend)
    if (handled) return handled
    if (input.path === '/catalog' && list !== 'normal')
      return Promise.resolve(
        list === 'empty'
          ? {
              status: 200,
              body: { schema: 1, ok: true, code: 'catalog.groups', message: '', data: [] },
            }
          : {
              status: 503,
              body: {
                schema: 1,
                ok: false,
                code: 'api.connection_failed',
                message: '',
                data: null,
              },
            },
      )
    return backend.request(input, signal)
  })
  const transport: ApiTransport = {
    kind: 'native',
    capabilities: new NativeTransport().capabilities.filter(
      (operation) =>
        !options.readOnly ||
        ![
          'subscriptions.add',
          'subscriptions.edit',
          'subscriptions.remove',
          'subscriptions.update',
          'subscriptions.update_all',
          'subscriptions.cancel',
        ].includes(operation),
    ),
    request,
    subscribe: (...args) => backend.subscribe(args[0], args[1]),
  }
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  localStorage.setItem('netproxy.web.preferences', JSON.stringify({ theme: 'light', locale: 'en' }))
  window.location.hash = '#/subscriptions'
  const router = makeRouter()
  const view = render(
    <AppProviders api={new ApiClient(transport)}>
      {options.controls && <ControlProbe />}
      <RouterProvider router={router} />
    </AppProviders>,
  )
  return { request, router, view, backend }
}
it('reads real-shaped local and URL details, traffic, history and progress without subscription writes', async () => {
  const h = setup('normal', { readOnly: true })
  expect(await screen.findByRole('heading', { name: 'Local groups' })).toBeVisible()
  expect(screen.getByRole('heading', { name: 'Subscriptions' })).toBeVisible()
  expect(screen.getByRole('heading', { name: 'Travel backup' })).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'View details 本地配置' }))
  let dialog = await screen.findByRole('dialog')
  await waitFor(() => expect(within(dialog).getByText('Nodes')).toBeVisible())
  expect(within(dialog).queryByText('Automatic updates')).not.toBeInTheDocument()
  expect(within(dialog).queryByText('Update history')).not.toBeInTheDocument()
  expect(
    h.request.mock.calls.filter(([input]) => input.path.startsWith('/subscriptions/default')),
  ).toHaveLength(1)
  fireEvent.click(within(dialog).getAllByRole('button', { name: 'Close' }).at(-1)!)
  fireEvent.click(screen.getByRole('button', { name: 'View details Global Connect' }))
  dialog = await screen.findByRole('dialog')
  expect(await within(dialog).findByText('Subscription usage')).toBeVisible()
  expect(await within(dialog).findByText('Runtime synchronized')).toBeVisible()
  expect(within(dialog).getByText('Failed')).toBeVisible()
  expect(within(dialog).getByText('100 GB')).toBeVisible()
  expect(within(dialog).getByText('1440 minutes')).toBeVisible()
  expect(within(dialog).getByText('No active update reported')).toBeVisible()
  expect(
    h.request.mock.calls.every(([input]) => input.method === 'GET' && input.query === undefined),
  ).toBe(true)
  expect(
    screen.queryByRole('button', { name: /add|delete|cancel|update all/i }),
  ).not.toBeInTheDocument()
  fireEvent.click(within(dialog).getAllByRole('button', { name: 'Close' }).at(-1)!)
  fireEvent.click(screen.getByRole('button', { name: 'View details 备用线路' }))
  dialog = await screen.findByRole('dialog')
  expect(await within(dialog).findByText(/Downloading/)).toBeVisible()
  expect(
    within(dialog).getByText('Stages are not a percentage. Final results appear in history.'),
  ).toBeVisible()
  expect(within(dialog).queryByText('Succeeded')).not.toBeInTheDocument()
  fireEvent.click(within(dialog).getAllByRole('button', { name: 'Close' }).at(-1)!)
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  await act(() => h.router.navigate({ to: '/nodes' }))
  h.view.unmount()
})
it('distinguishes an empty Catalog from a read failure without using Mock as fallback', async () => {
  const empty = setup('empty')
  expect(await screen.findByText('Nothing here yet')).toBeVisible()
  expect(screen.queryByRole('heading', { name: 'Global Connect' })).not.toBeInTheDocument()
  empty.view.unmount()
  const error = setup('error')
  expect(await screen.findByRole('alert')).toHaveTextContent('The operation failed.')
  expect(screen.queryByRole('heading', { name: 'Global Connect' })).not.toBeInTheDocument()
  error.view.unmount()
})

async function openDetails(name = 'Global Connect') {
  fireEvent.click(await screen.findByRole('button', { name: `View details ${name}` }))
  const dialog = await screen.findByRole('dialog')
  await waitFor(() => expect(within(dialog).getByText('Nodes')).toBeVisible())
  return dialog
}
async function updateAll() {
  fireEvent.click(await screen.findByRole('button', { name: 'More actions' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Update all subscriptions' }))
}
const writes = (request: ReturnType<typeof setup>['request']) =>
  request.mock.calls.filter(([input]) => input.method !== 'GET')

it('completes an actual Mock update through ModuleEvents without aborting its own request or replaying it', async () => {
  let liveAtResponse = false
  const h = setup('normal', {
    handle: (input, signal, backend) =>
      input.method === 'POST' && input.path === '/subscriptions/demo-global/update'
        ? backend.request(input, signal).then((response) => {
            liveAtResponse = signal?.aborted === false
            return response
          })
        : undefined,
  })
  const dialog = await openDetails()
  fireEvent.click(within(dialog).getByRole('button', { name: 'Update subscription' }))
  expect(await within(dialog).findByText('Subscription operation in progress')).toBeVisible()
  await waitFor(() =>
    expect(within(dialog).getByText('Subscription updated', { exact: true })).toBeVisible(),
  )
  expect(liveAtResponse).toBe(true)
  expect(writes(h.request)).toHaveLength(1)
  await waitFor(() =>
    expect(within(dialog).getByRole('button', { name: 'Update subscription' })).toBeEnabled(),
  )
  expect(
    within(dialog).queryByText('Subscription result awaiting confirmation'),
  ).not.toBeInTheDocument()
})

it('keeps the operation visible after closing details and blocks service, mode and selection writes while updating', async () => {
  const h = setup('normal', { controls: true })
  const dialog = await openDetails()
  fireEvent.click(within(dialog).getByRole('button', { name: 'Update subscription' }))
  fireEvent.click(within(dialog).getAllByRole('button', { name: 'Close' }).at(-1)!)
  expect(await screen.findByText('Subscription operation in progress')).toBeVisible()
  for (const name of ['test-stop', 'test-mode', 'test-selection'])
    fireEvent.click(screen.getByRole('button', { name }))
  expect(writes(h.request).map(([input]) => input.path)).toEqual([
    '/subscriptions/demo-global/update',
  ])
  expect(await screen.findByText('Subscription updated', { exact: true })).toBeVisible()
})

it.each(['service', 'mode', 'selection'] as const)(
  'does not admit an update while a %s write is in progress',
  async (kind) => {
    let release!: () => void
    const held = new Promise<void>((resolve) => {
      release = resolve
    })
    const h = setup('normal', {
      controls: true,
      handle: (input, signal, backend) =>
        input.path ===
          (kind === 'service' ? '/service/stop' : kind === 'mode' ? '/mode' : '/selection') &&
        input.method !== 'GET'
          ? held.then(() => backend.request(input, signal))
          : undefined,
    })
    const dialog = await openDetails()
    fireEvent.click(
      screen.getByRole('button', {
        name: kind === 'service' ? 'test-stop' : `test-${kind}`,
        hidden: true,
      }),
    )
    fireEvent.click(within(dialog).getByRole('button', { name: 'Update subscription' }))
    expect(writes(h.request).some(([input]) => input.path.endsWith('/update'))).toBe(false)
    await act(async () => {
      release()
      await held
    })
    await waitFor(() => expect(writes(h.request)).toHaveLength(1))
  },
)

it('keeps a lost response uncertain and only performs reads when checking the result', async () => {
  const h = setup('normal', {
    handle: (input, signal, backend) =>
      input.method === 'POST' && input.path === '/subscriptions/demo-global/update'
        ? backend.request(input, signal).then(() => {
            throw new TypeError('anonymous response lost')
          })
        : undefined,
  })
  const dialog = await openDetails()
  fireEvent.click(within(dialog).getByRole('button', { name: 'Update subscription' }))
  expect(await within(dialog).findByText('Subscription result awaiting confirmation')).toBeVisible()
  expect(
    within(dialog).queryByText('Subscription updated', { exact: true }),
  ).not.toBeInTheDocument()
  expect(screen.getAllByRole('alert')).toHaveLength(1)
  expect(screen.getByRole('alert')).not.toHaveTextContent('anonymous response lost')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Review current state' }))
  await waitFor(() =>
    expect(
      h.request.mock.calls.filter(([input]) => input.path === '/catalog').length,
    ).toBeGreaterThan(1),
  )
  expect(writes(h.request)).toHaveLength(1)
  fireEvent.click(within(dialog).getAllByRole('button', { name: 'Close' }).at(-1)!)
  expect(await screen.findByText('Subscription result awaiting confirmation')).toBeVisible()
  expect(screen.getAllByRole('alert')).toHaveLength(1)
})

it('supports Mock add, public-only edit and confirmed deletion without nested panels or private GETs', async () => {
  const h = setup()
  fireEvent.click(await screen.findByRole('button', { name: 'Add subscription' }))
  let dialog = await screen.findByRole('dialog')
  fireEvent.change(within(dialog).getByRole('textbox', { name: 'Name' }), {
    target: { value: 'Anonymous source' },
  })
  fireEvent.change(within(dialog).getByRole('textbox', { name: 'Subscription URL' }), {
    target: { value: 'https://example.invalid/feed?token=anonymous' },
  })
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }))
  expect(await screen.findByRole('heading', { name: 'Anonymous source' })).toBeVisible()
  dialog = await openDetails('Anonymous source')
  await waitFor(() =>
    expect(within(dialog).getByRole('button', { name: 'Edit subscription' })).toBeEnabled(),
  )
  fireEvent.click(within(dialog).getByRole('button', { name: 'Edit subscription' }))
  dialog = await screen.findByRole('dialog')
  expect(screen.getAllByRole('dialog')).toHaveLength(1)
  expect(
    within(dialog).queryByRole('textbox', { name: 'Subscription URL' }),
  ).not.toBeInTheDocument()
  fireEvent.change(within(dialog).getByRole('textbox', { name: 'Name' }), {
    target: { value: 'Renamed source' },
  })
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }))
  expect(await screen.findByRole('heading', { name: 'Renamed source' })).toBeVisible()
  const edit = writes(h.request).find(([input]) => input.method === 'PUT')![0]
  expect(edit.body).toEqual({ name: 'Renamed source' })
  dialog = await openDetails('Renamed source')
  await waitFor(() =>
    expect(within(dialog).getByRole('button', { name: 'Delete subscription' })).toBeEnabled(),
  )
  fireEvent.click(within(dialog).getByRole('button', { name: 'Delete subscription' }))
  dialog = await screen.findByRole('dialog')
  expect(within(dialog).getByText('Renamed source', { exact: true })).toBeVisible()
  expect(
    within(dialog).getByText('Delete this subscription and its nodes? This cannot be undone.'),
  ).toBeVisible()
  expect(writes(h.request).some(([input]) => input.method === 'DELETE')).toBe(false)
  fireEvent.click(within(dialog).getByRole('button', { name: 'Delete subscription' }))
  expect(await screen.findByText('Subscription deleted', { exact: true })).toBeVisible()
  await waitFor(() =>
    expect(screen.queryByRole('heading', { name: 'Renamed source' })).not.toBeInTheDocument(),
  )
  expect(writes(h.request).map(([input]) => input.method)).toEqual(['POST', 'PUT', 'DELETE'])
  expect(h.request.mock.calls.every(([input]) => !input.query || !('private' in input.query))).toBe(
    true,
  )
  expect(localStorage.getItem('netproxy.web.preferences')).not.toContain('anonymous')
})

it('does not expose URL edit or deletion for a local group', async () => {
  setup()
  const dialog = await openDetails('本地配置')
  for (const name of [
    'Edit subscription',
    'Delete subscription',
    'Update subscription',
    'Request cancellation',
  ])
    expect(within(dialog).queryByRole('button', { name })).not.toBeInTheDocument()
})

it('requires an explicit valid replacement before deleting the active subscription', async () => {
  const h = setup()
  let dialog = await openDetails()
  fireEvent.click(within(dialog).getByRole('button', { name: 'Delete subscription' }))
  dialog = await screen.findByRole('dialog')
  const confirm = within(dialog).getByRole('button', { name: 'Delete subscription' })
  expect(confirm).toBeDisabled()
  expect(within(dialog).getByText('Global Connect', { exact: true })).toBeVisible()
  fireEvent.click(within(dialog).getByRole('combobox', { name: 'Replacement group' }))
  expect(screen.queryByRole('option', { name: 'Global Connect' })).not.toBeInTheDocument()
  expect(screen.queryByRole('option', { name: 'Travel backup' })).not.toBeInTheDocument()
  fireEvent.click(await screen.findByRole('option', { name: '本地配置' }))
  await waitFor(() => expect(confirm).toBeEnabled())
  fireEvent.click(confirm)
  expect(await screen.findByText('Subscription deleted', { exact: true })).toBeVisible()
  expect(writes(h.request).find(([input]) => input.method === 'DELETE')![0]).toEqual({
    method: 'DELETE',
    path: '/subscriptions/demo-global',
    query: { replacement_group_id: 'default' },
  })
  await waitFor(() =>
    expect(screen.queryByRole('heading', { name: 'Global Connect' })).not.toBeInTheDocument(),
  )
})

it('distinguishes 304 responses from new content updates', async () => {
  setup()
  const dialog = await openDetails()
  const update = within(dialog).getByRole('button', { name: 'Update subscription' })
  fireEvent.click(update)
  expect(await within(dialog).findByText('Subscription updated', { exact: true })).toBeVisible()
  await waitFor(() => expect(update).toBeEnabled())
  fireEvent.click(update)
  expect(
    await within(dialog).findByText('Subscription content has not changed', { exact: true }),
  ).toBeVisible()
})

it('renders partial update-all results individually without a false all-success notice', async () => {
  const h = setup()
  await updateAll()
  expect(
    await screen.findByText('Some subscriptions did not update. Review each result.'),
  ).toBeVisible()
  const operation = screen.getByRole('region', { name: 'Subscription operation' })
  expect(within(operation).getByText('Global Connect', { exact: true })).toBeVisible()
  expect(within(operation).getByText('备用线路', { exact: true })).toBeVisible()
  expect(within(operation).getByText('Succeeded', { exact: true })).toBeVisible()
  expect(within(operation).getByText('Failed', { exact: true })).toBeVisible()
  expect(screen.queryByText('All subscriptions updated', { exact: true })).not.toBeInTheDocument()
  expect(writes(h.request)).toHaveLength(1)
})

it('requests cancellation without claiming that the transaction was rolled back', async () => {
  const h = setup()
  const dialog = await openDetails()
  fireEvent.click(within(dialog).getByRole('button', { name: 'Update subscription' }))
  fireEvent.click(await within(dialog).findByRole('button', { name: 'Request cancellation' }))
  expect(
    await within(dialog).findByText('Cancellation requested; check the final result.'),
  ).toBeVisible()
  expect(writes(h.request).map(([input]) => input.path)).toEqual([
    '/subscriptions/demo-global/update',
    '/subscriptions/demo-global/cancel',
  ])
  expect(within(dialog).queryByText('Cancelled', { exact: true })).not.toBeInTheDocument()
  fireEvent.click(within(dialog).getAllByRole('button', { name: 'Close' }).at(-1)!)
  expect(await screen.findByText('Cancellation requested; check the final result.')).toBeVisible()
})

it('preserves a persisted runtime failure and renders its warning only inside the active panel', async () => {
  const h = setup('normal', {
    handle: (input, signal, backend) =>
      input.method === 'POST' && input.path === '/subscriptions/demo-global/update'
        ? backend.request(input, signal).then(() => ({
            status: 502,
            body: {
              schema: 1,
              ok: false,
              code: 'subscription.runtime_sync_failed',
              message: 'anonymous private original error',
              data: {
                group_id: 'demo-global',
                persisted: true,
                runtime_sync_state: 'failed',
                runtime_sync_pending: true,
              },
            },
          }))
        : undefined,
  })
  const dialog = await openDetails()
  fireEvent.click(within(dialog).getByRole('button', { name: 'Update subscription' }))
  expect(await within(dialog).findByRole('alert')).toHaveTextContent(
    'Saved, but runtime application failed.',
  )
  expect(screen.getAllByRole('alert', { hidden: true })).toHaveLength(1)
  expect(screen.queryByText('Subscription updated', { exact: true })).not.toBeInTheDocument()
  expect(screen.getByRole('alert')).not.toHaveTextContent('anonymous private original error')
  fireEvent.click(within(dialog).getAllByRole('button', { name: 'Close' }).at(-1)!)
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Saved, but runtime application failed.',
  )
  expect(screen.getAllByRole('alert')).toHaveLength(1)
  expect(writes(h.request)).toHaveLength(1)
})

it('tracks cancellation of an external update without creating an own-operation result', async () => {
  const h = setup()
  const dialog = await openDetails('备用线路')
  const cancel = within(dialog).getByRole('button', { name: 'Request cancellation' })
  fireEvent.click(cancel)
  expect(
    await within(dialog).findByText('Cancellation requested; check the final result.'),
  ).toBeVisible()
  expect(cancel).toBeDisabled()
  expect(within(dialog).getByText(/Downloading/)).toBeVisible()
  expect(
    within(dialog).queryByText('Subscription updated', { exact: true }),
  ).not.toBeInTheDocument()
  expect(writes(h.request).map(([input]) => input.path)).toEqual([
    '/subscriptions/demo-empty/cancel',
  ])
  fireEvent.click(within(dialog).getAllByRole('button', { name: 'Close' }).at(-1)!)
  expect(await screen.findByText('Cancellation requested; check the final result.')).toBeVisible()
  const other = await openDetails()
  expect(
    within(other).queryByText('Cancellation requested; check the final result.'),
  ).not.toBeInTheDocument()
})

it('shows an unknown external cancellation result without automatically resending it', async () => {
  const h = setup('normal', {
    handle: (input) =>
      input.method === 'POST' && input.path === '/subscriptions/demo-empty/cancel'
        ? Promise.reject(new TypeError('anonymous lost cancellation response'))
        : undefined,
  })
  const dialog = await openDetails('备用线路')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Request cancellation' }))
  expect(
    await within(dialog).findByText(
      'Cancellation result is unknown. The update may still be running.',
    ),
  ).toBeVisible()
  expect(within(dialog).getByRole('button', { name: 'Request cancellation' })).toBeEnabled()
  expect(within(dialog).getByRole('alert')).not.toHaveTextContent(
    'anonymous lost cancellation response',
  )
  expect(writes(h.request)).toHaveLength(1)
})

it('uses the batch deadline for progress while still allowing cancellation of the active group', async () => {
  const details = vi.spyOn(resourceApi, 'useResourceDetails')
  let release!: () => void
  const held = new Promise<void>((resolve) => {
    release = resolve
  })
  const h = setup('normal', {
    handle: (input, signal, backend) =>
      input.method === 'POST' && input.path === '/subscriptions/update-all'
        ? held.then(() => backend.request(input, signal))
        : undefined,
  })
  await updateAll()
  try {
    const dialog = await openDetails('备用线路')
    for (const name of ['Update subscription', 'Edit subscription', 'Delete subscription'])
      expect(within(dialog).getByRole('button', { name })).toBeDisabled()
    const call = details.mock.calls.findLast(
      ([id, , , deadline]) => id === 'demo-empty' && typeof deadline === 'number',
    )
    expect(call?.[3]).toBeGreaterThan(Date.now() + 600_000)
    const cancel = within(dialog).getByRole('button', { name: 'Request cancellation' })
    expect(cancel).toBeEnabled()
    fireEvent.click(cancel)
    expect(
      await within(dialog).findByText('Cancellation requested; check the final result.'),
    ).toBeVisible()
    expect(writes(h.request).map(([input]) => input.path)).toEqual([
      '/subscriptions/update-all',
      '/subscriptions/demo-empty/cancel',
    ])
  } finally {
    await act(async () => {
      release()
      await held
    })
  }
})
