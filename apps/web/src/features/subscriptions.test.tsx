import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { RouterProvider } from '@tanstack/react-router'
import { afterEach, expect, it, vi } from 'vitest'
import { ApiClient } from '@/api/client'
import type { ApiTransport } from '@/api/contracts'
import { NativeTransport } from '@/api/native'
import { AppProviders } from '@/app/providers'
import { makeRouter } from '@/app/router'
import { MockTransport } from '@/mocks/transport'

afterEach(() => vi.restoreAllMocks())
function setup(list: 'normal' | 'empty' | 'error' = 'normal') {
  const backend = new MockTransport(0)
  const request = vi.fn<ApiTransport['request']>((input, signal) => {
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
    capabilities: new NativeTransport().capabilities,
    request,
    subscribe: (...args) => backend.subscribe(args[0], args[1]),
  }
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  localStorage.setItem('netproxy.web.preferences', JSON.stringify({ theme: 'light', locale: 'en' }))
  window.location.hash = '#/subscriptions'
  const router = makeRouter()
  const view = render(
    <AppProviders api={new ApiClient(transport)}>
      <RouterProvider router={router} />
    </AppProviders>,
  )
  return { request, router, view }
}
it('reads real-shaped local and URL details, traffic, history and progress without subscription writes', async () => {
  const h = setup()
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
  fireEvent.click(within(dialog).getByRole('button', { name: 'Close' }))
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
  fireEvent.click(within(dialog).getByRole('button', { name: 'Close' }))
  fireEvent.click(screen.getByRole('button', { name: 'View details 备用线路' }))
  dialog = await screen.findByRole('dialog')
  expect(await within(dialog).findByText(/Downloading/)).toBeVisible()
  expect(
    within(dialog).getByText('Stages are not a percentage. Final results appear in history.'),
  ).toBeVisible()
  expect(within(dialog).queryByText('Succeeded')).not.toBeInTheDocument()
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
