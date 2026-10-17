import { render, screen, waitFor } from '@testing-library/react'
import { RouterProvider } from '@tanstack/react-router'
import { expect, it, vi } from 'vitest'
import { ApiClient } from '@/api/client'
import { type ApiTransport } from '@/api/contracts'
import { AppProviders } from '@/app/providers'
import { makeRouter } from '@/app/router'
import { createFixtures } from '@/mocks/fixtures'

it('shows native data but disables every business write and reads no unsupported endpoint', async () => {
  const { service, snapshot } = createFixtures()
  const info = {
    api_version: 1,
    version: 'fixture',
    version_code: '1',
    instance_id: 'fixture',
    pid: 1,
    platform: 'android',
    capabilities: ['events'],
    memory: {
      heap_alloc_bytes: 1,
      heap_inuse_bytes: 1,
      heap_released_bytes: 0,
      stack_inuse_bytes: 1,
    },
  }
  const request = vi.fn<ApiTransport['request']>(async ({ path }) => {
    if (path !== '/info' && path !== '/service') throw new Error('Unsupported resource requested')
    return {
      status: 200,
      body: {
        schema: 1,
        ok: true,
        code: 'fixture',
        message: '',
        data: path === '/info' ? info : service,
      },
    }
  })
  const transport: ApiTransport = {
    kind: 'native',
    capabilities: ['info.read', 'service.read'],
    request,
    subscribe: (_topics, consume) => {
      for (const [name, data] of [
        ['status', service],
        ['groups', snapshot],
        ['subscriptions', snapshot.groups.map(({ group }) => group)],
      ] as const)
        consume({ name, body: { schema: 1, ok: true, code: 'fixture', message: '', data } })
      return () => {}
    },
  }
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  localStorage.setItem('netproxy.web.preferences', JSON.stringify({ theme: 'light', locale: 'en' }))
  window.location.hash = '#/'
  const router = makeRouter()
  const view = render(
    <AppProviders api={new ApiClient(transport)}>
      <RouterProvider router={router} />
    </AppProviders>,
  )
  expect(await screen.findByRole('note')).toHaveTextContent('Native connection · core controls')
  expect(screen.queryByText('Demo mode')).not.toBeInTheDocument()
  expect(await screen.findByRole('button', { name: 'Stop service' })).toBeDisabled()
  expect(screen.getByRole('combobox', { name: 'Outbound mode' })).toBeDisabled()
  await router.navigate({ to: '/nodes' })
  expect(await screen.findByRole('button', { name: 'Select Tokyo 01' })).toBeDisabled()
  for (const button of screen.getAllByRole('button', { name: /^Select / }))
    expect(button).toBeDisabled()
  await router.navigate({ to: '/subscriptions' })
  expect(await screen.findByRole('heading', { name: '本地配置' })).toBeVisible()
  await router.navigate({ to: '/settings' })
  await waitFor(() => expect(screen.getByRole('heading', { name: 'Settings' })).toBeVisible())
  expect(screen.queryByRole('switch', { name: 'Start on boot' })).not.toBeInTheDocument()
  expect(
    request.mock.calls.every(
      ([req]) => req.method === 'GET' && ['/info', '/service'].includes(req.path),
    ),
  ).toBe(true)
  view.unmount()
})
