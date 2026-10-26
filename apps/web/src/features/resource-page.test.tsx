import { useState, type ReactNode } from 'react'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { afterEach, expect, it, vi } from 'vitest'
import userEvent from '@testing-library/user-event'
import { ApiClient } from '@/api/client'
import { catalogNodesKey } from '@/api/resources'
import { apiOperations, type ApiTransport, type GroupSummary } from '@/api/contracts'
import { ApiContext, PreferencesContext } from '@/app/context'
import { HeaderActionsProvider } from '@/app/header-actions'
import { translator } from '@/app/i18n'
import { TooltipProvider } from '@/components/ui/tooltip'
import type { JsonEditorProps } from '@/components/json-editor'
import { SubscriptionsPage } from './subscriptions'
import { ResourceProviderTag } from './resource-provider-tag'
import { AndroidImports } from './android-imports'
import type { AndroidInputAdapter } from '@/api/android-input'

vi.mock('@/components/json-editor', () => ({
  JsonEditor: ({ value, onChange, readOnly, label }: JsonEditorProps) => (
    <textarea
      aria-label={label}
      value={value}
      readOnly={readOnly}
      onChange={(event) => onChange?.(event.target.value)}
    />
  ),
}))

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

function group(id: string, name: string, type: 'local' | 'subscription') {
  return {
    id,
    name,
    provider_tag: `np/${id}`,
    type,
    node_count: 1,
    revision: 1,
    auto_update: false,
    update_interval: 86400,
    update_via_proxy: 'auto',
    usage: null,
    profile_title: '',
    profile_web_page_url: '',
    last_attempt_at: '',
    last_success_at: '',
    next_update_at: '',
    last_error: '',
    updated_at: '2026-10-18T00:00:00Z',
    progress: null,
  } satisfies GroupSummary & { provider_tag: string }
}

function ResourcePageHarness({ children }: { children: ReactNode }) {
  const [target, setTarget] = useState<HTMLElement | null>(null)
  return (
    <HeaderActionsProvider target={target}>
      <header ref={setTarget} />
      {children}
    </HeaderActionsProvider>
  )
}

function setup(
  element?: ReactNode,
  options: { catalogWrite?: boolean; androidInput?: AndroidInputAdapter } = {},
) {
  let groups = [
    group('default', 'Local default', 'local'),
    group('local-second', 'Local second', 'local'),
    group('source-fixture', 'Subscription fixture', 'subscription'),
  ]
  const nodes = (id: string) =>
    groups.find((entry) => entry.id === id)?.node_count
      ? [{ tag: `${id} node`, protocol: 'socks' }]
      : []
  const ok = (data: unknown) => ({
    status: 200,
    body: { schema: 1, ok: true, code: 'fixture.ok', message: '', data },
  })
  const request = vi.fn<ApiTransport['request']>(async (input) => {
    if (input.method === 'GET') {
      if (input.path === '/catalog' || input.path === '/nodes/snapshot')
        return ok({ groups: groups.map((entry) => ({ group: entry, nodes: nodes(entry.id) })) })
      if (input.path === '/nodes/get')
        return ok({ type: 'socks', tag: input.query?.node_tag, password: 'test-only-node-secret' })
      if (input.path === '/nodes/export')
        return ok({ link: 'socks://fixture:test-only-link@localhost:1080' })
      const target = /^\/subscriptions\/([^/]+)(?:\/(history|progress))?$/.exec(input.path)
      if (target) {
        if (target[2] === 'history') return ok([])
        if (target[2] === 'progress') return ok({ id: target[1], progress: null })
        const entry = groups.find((item) => item.id === target[1])
        return ok({ group: entry, nodes: [] })
      }
    }
    if (input.method === 'DELETE' && input.path === '/subscriptions/source-fixture') {
      groups = groups.filter((entry) => entry.id !== 'source-fixture')
      return ok({ id: 'source-fixture' })
    }
    if (input.method === 'POST' && input.path === '/catalog') {
      const { id, name } = input.body as { id: string; name: string }
      groups = [...groups, { ...group(id, name, 'local'), node_count: 0 }]
      return ok({ id })
    }
    if (input.path.startsWith('/catalog/') && input.method !== 'GET') {
      const id = input.path.slice('/catalog/'.length)
      if (input.method === 'DELETE') groups = groups.filter((entry) => entry.id !== id)
      else
        groups = groups.map((entry) =>
          entry.id === id
            ? {
                ...entry,
                name: (input.body as { name: string }).name,
                revision: entry.revision + 1,
              }
            : entry,
        )
      return ok({ id })
    }
    if (input.path === '/nodes' || input.path === '/nodes/import') return ok({ persisted: true })
    throw new Error(`Unexpected fixture request: ${input.method} ${input.path}`)
  })
  const transport: ApiTransport = {
    kind: 'native',
    capabilities: apiOperations.filter(
      (operation) => options.catalogWrite !== false || operation !== 'catalog.write',
    ),
    request,
    subscribe: () => () => {},
  }
  const cache = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  cache.setQueryData(['events-connection'], 'live')
  const root = createRootRoute({
    component: () => (
      <ResourcePageHarness>
        {options.androidInput ? (
          <AndroidImports adapter={options.androidInput}>
            {element ?? <SubscriptionsPage />}
          </AndroidImports>
        ) : (
          (element ?? <SubscriptionsPage />)
        )}
      </ResourcePageHarness>
    ),
  })
  const router = createRouter({ routeTree: root, history: createMemoryHistory() })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  const view = render(
    <PreferencesContext
      value={{
        preferences: { locale: 'en', theme: 'light' },
        update: () => {},
        t: translator('en'),
      }}
    >
      <ApiContext value={new ApiClient(transport)}>
        <QueryClientProvider client={cache}>
          <TooltipProvider>
            <RouterProvider router={router} />
          </TooltipProvider>
        </QueryClientProvider>
      </ApiContext>
    </PreferencesContext>,
  )
  return { cache, request, view }
}

async function details(name = 'Local second') {
  fireEvent.click(await screen.findByRole('button', { name: `View details ${name}` }))
  const panel = await screen.findByRole('dialog')
  await within(panel).findByRole('heading', { name: 'Catalog nodes' })
  return panel
}

async function nodeAction(tag: string, name: string) {
  const trigger = await screen.findByRole('button', { name: `Node actions ${tag}` })
  await waitFor(() => expect(trigger).toBeEnabled())
  fireEvent.click(trigger)
  fireEvent.click(await screen.findByRole('menuitem', { name }))
}

function closePanel() {
  fireEvent.click(screen.getAllByRole('button', { name: 'Close' }).at(-1)!)
}

async function submitNode(name = 'Save') {
  const button = await screen.findByRole('button', { name })
  await waitFor(() => expect(button).toBeEnabled())
  fireEvent.click(button)
}

it('opens subscription creation from the resource header without issuing a write', async () => {
  const h = setup()
  const trigger = await screen.findByRole('button', { name: 'Resource actions' })
  expect(within(screen.getByRole('banner')).getByRole('button', { name: 'Resource actions' })).toBe(
    trigger,
  )
  fireEvent.click(trigger)
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Add subscription' }))
  const panel = await screen.findByRole('dialog')
  expect(within(panel).getByRole('textbox', { name: 'Subscription URL' })).toBeVisible()
  expect(h.request.mock.calls.every(([input]) => input.method === 'GET')).toBe(true)
})

it('opens a system document from the resource header menu without importing automatically', async () => {
  const pickDocument = vi.fn<AndroidInputAdapter['pickDocument']>().mockResolvedValue({
    id: 'file-fixture',
    name: 'nodes.txt',
    content: 'ss://test-only-file@localhost:1234',
  })
  const h = setup(undefined, {
    androidInput: {
      pickDocument,
      takeImport: async () => null,
      completeImport: async () => {},
      listen: async (_notify, visibility) => {
        visibility(true)
        return () => {}
      },
    },
  })
  fireEvent.click(await screen.findByRole('button', { name: 'Resource actions' }))
  const item = await screen.findByRole('menuitem', { name: 'Import from file' })
  await waitFor(() => expect(item).not.toHaveAttribute('aria-disabled', 'true'))
  fireEvent.click(item)
  expect(await screen.findByRole('textbox', { name: 'Import content' })).toHaveValue(
    'ss://test-only-file@localhost:1234',
  )
  expect(pickDocument).toHaveBeenCalledWith('nodes')
  expect(h.request.mock.calls.every(([input]) => input.method === 'GET')).toBe(true)
})

it('reads Catalog-only nodes into their resource details without runtime selection or navigation', async () => {
  const h = setup()
  const panel = await details()
  expect(await within(panel).findByText('local-second node', { exact: true })).toBeVisible()
  expect(within(panel).queryByText('default node', { exact: true })).not.toBeInTheDocument()
  expect(within(panel).getByText('np/local-second')).toBeVisible()
  expect(screen.queryByText('Active group')).not.toBeInTheDocument()
  expect(within(panel).queryByRole('link')).not.toBeInTheDocument()
  expect(h.request.mock.calls.some(([input]) => input.path === '/nodes/snapshot')).toBe(true)
  expect(h.request.mock.calls.some(([input]) => /selection|policy-groups/.test(input.path))).toBe(
    false,
  )
})

it('refreshes the Catalog node list along with resource metadata', async () => {
  const h = setup()
  const panel = await details()
  await within(panel).findByRole('button', { name: 'Node actions local-second node' })
  const before = h.request.mock.calls.filter(([input]) => input.path === '/nodes/snapshot').length
  fireEvent.click(within(panel).getByRole('button', { name: 'More actions' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Refresh resources' }))
  await waitFor(() =>
    expect(
      h.request.mock.calls.filter(([input]) => input.path === '/nodes/snapshot').length,
    ).toBeGreaterThan(before),
  )
})

it('edits and adds nodes in the opened local resource and returns to that resource', async () => {
  const h = setup()
  await details()
  await nodeAction('local-second node', 'Edit node')
  const editor = await screen.findByRole('textbox', { name: 'Node content' })
  expect((editor as HTMLTextAreaElement).value).toContain('test-only-node-secret')
  await submitNode()
  await screen.findByText('Operation completed. Nodes refreshed.')
  expect(h.request.mock.calls.find(([input]) => input.path === '/nodes/get')?.[0].query).toEqual({
    group_id: 'local-second',
    node_tag: 'local-second node',
  })
  const saved = h.request.mock.calls.find(([input]) => input.method === 'PUT')?.[0]
  expect(saved).toMatchObject({
    path: '/nodes',
    body: { group_id: 'local-second', node_tag: 'local-second node', allow_insecure: false },
  })
  expect(
    JSON.stringify(
      h.cache
        .getQueryCache()
        .getAll()
        .map((query) => query.state.data),
    ),
  ).not.toContain('test-only-node-secret')
  expect(h.cache.getMutationCache().getAll()).toHaveLength(0)
  closePanel()
  await screen.findByRole('heading', { name: 'Local second' })
  const add = await screen.findByRole('button', { name: 'Add node manually' })
  await waitFor(() => expect(add).toBeEnabled())
  fireEvent.click(add)
  const input = await screen.findByRole('textbox', { name: 'Node content' })
  expect(screen.queryByRole('combobox', { name: 'Local group' })).not.toBeInTheDocument()
  fireEvent.change(input, { target: { value: '{"type":"socks","tag":"new fixture"}' } })
  await submitNode()
  await screen.findByText('Operation completed. Nodes refreshed.')
  expect(h.request.mock.calls.find(([call]) => call.method === 'POST')?.[0]).toMatchObject({
    path: '/nodes',
    body: { group_id: 'local-second' },
  })
})

it('shares subscription nodes locally without exposing edit or delete actions', async () => {
  const h = setup()
  await details('Subscription fixture')
  await nodeAction('source-fixture node', 'Share link')
  const content = await screen.findByRole('textbox', { name: 'Node content' })
  expect(content).toHaveAttribute('readonly')
  expect(content).toHaveValue('socks://fixture:test-only-link@localhost:1080')
  expect(h.request.mock.calls.find(([input]) => input.path === '/nodes/export')?.[0].query).toEqual(
    { group_id: 'source-fixture', node_tag: 'source-fixture node' },
  )
  expect(
    JSON.stringify(
      h.cache
        .getQueryCache()
        .getAll()
        .map((query) => query.state.data),
    ),
  ).not.toContain('test-only-link')
  closePanel()
  const trigger = await screen.findByRole('button', { name: 'Node actions source-fixture node' })
  await waitFor(() => expect(trigger).toBeEnabled())
  fireEvent.click(trigger)
  expect(await screen.findByRole('menuitem', { name: 'Share link' })).toBeVisible()
  expect(screen.queryByRole('menuitem', { name: 'Edit node' })).not.toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Delete node' })).not.toBeInTheDocument()
})

it('confirms a local node deletion without issuing a resource selection write', async () => {
  const h = setup()
  await details()
  await nodeAction('local-second node', 'Delete node')
  expect(h.request.mock.calls.filter(([input]) => input.method === 'DELETE')).toHaveLength(0)
  await submitNode('Confirm delete')
  await screen.findByText('Operation completed. Nodes refreshed.')
  expect(
    h.request.mock.calls.filter(([input]) => input.method !== 'GET').map(([input]) => input),
  ).toEqual([
    {
      method: 'DELETE',
      path: '/nodes',
      query: { group_id: 'local-second', node_tag: 'local-second node' },
    },
  ])
})

it('keeps text import on the resource page with its existing default-group API semantics', async () => {
  const h = setup()
  fireEvent.click(await screen.findByRole('button', { name: 'Resource actions' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Import text' }))
  const input = await screen.findByRole('textbox', { name: 'Node content' })
  fireEvent.change(input, { target: { value: 'socks://localhost:1080#fixture' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await screen.findByText('Operation completed. Nodes refreshed.')
  expect(h.request.mock.calls.find(([call]) => call.method === 'POST')?.[0]).toEqual({
    method: 'POST',
    path: '/nodes/import',
    body: { content: 'socks://localhost:1080#fixture', allow_insecure: false },
  })
})

it('imports shared node text into a selected local group through the existing append API', async () => {
  const h = setup()
  const user = userEvent.setup()
  fireEvent.click(await screen.findByRole('button', { name: 'Resource actions' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Import text' }))
  const input = await screen.findByRole('textbox', { name: 'Node content' })
  fireEvent.change(input, { target: { value: 'socks://localhost:1080#fixture' } })
  await user.click(screen.getByRole('combobox', { name: 'Local group' }))
  await user.click(await screen.findByRole('option', { name: 'Local second' }))
  await submitNode()
  await screen.findByText('Operation completed. Nodes refreshed.')
  expect(h.request.mock.calls.find(([call]) => call.method === 'POST')?.[0]).toEqual({
    method: 'POST',
    path: '/nodes',
    body: {
      group_id: 'local-second',
      content: 'socks://localhost:1080#fixture',
      allow_insecure: false,
    },
  })
})

it('deletes a subscription without an active resource or replacement group', async () => {
  const h = setup()
  const panel = await details('Subscription fixture')
  fireEvent.click(within(panel).getByRole('button', { name: 'More actions' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Delete subscription' }))
  expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
  fireEvent.click(await screen.findByRole('button', { name: 'Delete subscription' }))
  await waitFor(() =>
    expect(h.request.mock.calls.some(([input]) => input.method === 'DELETE')).toBe(true),
  )
  expect(h.request.mock.calls.find(([input]) => input.method === 'DELETE')?.[0]).toEqual({
    method: 'DELETE',
    path: '/subscriptions/source-fixture',
  })
})

it('cancels Catalog node reads on loss of the live foreground session', async () => {
  const h = setup()
  await details()
  await screen.findByRole('button', { name: 'Node actions local-second node' })
  const original = h.request.getMockImplementation()!
  let reading: AbortSignal | undefined
  h.request.mockImplementation((input, signal) => {
    if (input.path !== '/nodes/snapshot') return original(input, signal)
    reading = signal
    return new Promise((_resolve, reject) => {
      signal?.addEventListener('abort', () => reject(new DOMException('Cancelled', 'AbortError')), {
        once: true,
      })
    })
  })
  act(() => {
    void h.cache.invalidateQueries({ queryKey: catalogNodesKey })
  })
  await waitFor(() => expect(reading).toBeDefined())
  await act(async () => h.cache.setQueryData(['events-connection'], 'paused'))
  await waitFor(() => expect(reading?.aborted).toBe(true))
  expect(await screen.findByRole('button', { name: 'Add node manually' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Node actions local-second node' })).toBeDisabled()
})

it('copies the server ProviderTag unchanged after a display-name rename', async () => {
  const copy = vi.fn().mockResolvedValue(undefined)
  vi.stubGlobal(
    'navigator',
    Object.create(navigator, { clipboard: { value: { writeText: copy }, configurable: true } }),
  )
  const entry = group('stable-fixture', 'Renamed display name', 'local')
  setup(<ResourceProviderTag group={entry} />)
  fireEvent.click(await screen.findByRole('button', { name: 'Copy ProviderTag' }))
  expect(await screen.findByText('ProviderTag copied')).toBeVisible()
  expect(copy).toHaveBeenCalledExactlyOnceWith('np/stable-fixture')
  vi.unstubAllGlobals()
})

it('does not synthesize a ProviderTag from a resource id or name', async () => {
  const entry = { ...group('fixture', 'Not a tag', 'local'), provider_tag: '' }
  setup(<ResourceProviderTag group={entry} />)
  expect(await screen.findByRole('button', { name: 'Copy ProviderTag' })).toBeDisabled()
  expect(screen.queryByText('np/fixture')).not.toBeInTheDocument()
})

it('creates, renames and deletes local resources through fixed Catalog business requests', async () => {
  const h = setup()
  fireEvent.click(await screen.findByRole('button', { name: 'Resource actions' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Create local group' }))
  fireEvent.change(await screen.findByRole('textbox', { name: 'Group ID' }), {
    target: { value: 'local-created' },
  })
  fireEvent.change(screen.getByRole('textbox', { name: 'Group name' }), {
    target: { value: 'Created fixture' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await screen.findByText('Group saved')
  expect(
    h.request.mock.calls.find(
      ([input]) => input.path === '/catalog' && input.method === 'POST',
    )?.[0],
  ).toEqual({
    method: 'POST',
    path: '/catalog',
    body: { id: 'local-created', name: 'Created fixture' },
  })
  closePanel()
  let panel = await screen.findByRole('dialog', { name: 'Created fixture' })
  expect(await within(panel).findByText('np/local-created')).toBeVisible()
  fireEvent.click(within(panel).getByRole('button', { name: 'More actions' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Rename group' }))
  fireEvent.change(await screen.findByRole('textbox', { name: 'Group name' }), {
    target: { value: 'Renamed fixture' },
  })
  expect(screen.queryByRole('textbox', { name: 'Group ID' })).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await screen.findByText('Group saved')
  expect(h.request.mock.calls.find(([input]) => input.method === 'PUT')?.[0]).toEqual({
    method: 'PUT',
    path: '/catalog/local-created',
    body: { name: 'Renamed fixture' },
  })
  closePanel()
  panel = await screen.findByRole('dialog', { name: 'Renamed fixture' })
  expect(await within(panel).findByText('np/local-created')).toBeVisible()
  fireEvent.click(within(panel).getByRole('button', { name: 'More actions' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Delete group' }))
  expect(h.request.mock.calls.filter(([input]) => input.method === 'DELETE')).toHaveLength(0)
  fireEvent.click(await screen.findByRole('button', { name: 'Confirm delete' }))
  await screen.findByText('Group deleted')
  expect(h.request.mock.calls.find(([input]) => input.method === 'DELETE')?.[0]).toEqual({
    method: 'DELETE',
    path: '/catalog/local-created',
  })
  closePanel()
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(
    screen.queryByRole('button', { name: 'View details Renamed fixture' }),
  ).not.toBeInTheDocument()
})

it('never offers default resource deletion or local-group mutations for a subscription', async () => {
  setup()
  let panel = await details('Local default')
  fireEvent.click(within(panel).getByRole('button', { name: 'More actions' }))
  expect(await screen.findByRole('menuitem', { name: 'Rename group' })).toBeVisible()
  expect(screen.queryByRole('menuitem', { name: 'Delete group' })).not.toBeInTheDocument()
  fireEvent.keyDown(screen.getByRole('menu'), { key: 'Escape' })
  closePanel()
  panel = await details('Subscription fixture')
  fireEvent.click(within(panel).getByRole('button', { name: 'More actions' }))
  expect(await screen.findByRole('menuitem', { name: 'Edit subscription' })).toBeVisible()
  expect(screen.queryByRole('menuitem', { name: 'Rename group' })).not.toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Delete group' })).not.toBeInTheDocument()
})

it('rejects invalid or occupied resource ids without issuing a write', async () => {
  const h = setup()
  fireEvent.click(await screen.findByRole('button', { name: 'Resource actions' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Create local group' }))
  for (const id of ['../private', 'staging', 'default']) {
    fireEvent.change(await screen.findByRole('textbox', { name: 'Group ID' }), {
      target: { value: id },
    })
    fireEvent.change(screen.getByRole('textbox', { name: 'Group name' }), {
      target: { value: 'Fixture' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Enter an available, valid group ID and name.',
    )
  }
  expect(h.request.mock.calls.every(([input]) => input.method === 'GET')).toBe(true)
})

it('keeps a lost group write response uncertain without replaying it', async () => {
  const h = setup()
  const original = h.request.getMockImplementation()!
  h.request.mockImplementation((input, signal) =>
    input.method === 'POST' && input.path === '/catalog'
      ? original(input, signal).then(() => {
          throw new TypeError('lost fixture response')
        })
      : original(input, signal),
  )
  fireEvent.click(await screen.findByRole('button', { name: 'Resource actions' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Create local group' }))
  fireEvent.change(await screen.findByRole('textbox', { name: 'Group ID' }), {
    target: { value: 'lost-fixture' },
  })
  fireEvent.change(screen.getByRole('textbox', { name: 'Group name' }), {
    target: { value: 'Lost fixture' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await screen.findByText('Result not confirmed. Refresh resources to check.')
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  expect(screen.getByRole('alert')).not.toHaveTextContent('lost fixture response')
  expect(h.request.mock.calls.filter(([input]) => input.method === 'POST')).toHaveLength(1)
})

it('hides Catalog group writes when the transport does not support the fixed capability', async () => {
  setup(undefined, { catalogWrite: false })
  fireEvent.click(await screen.findByRole('button', { name: 'Resource actions' }))
  expect(await screen.findByRole('menuitem', { name: 'Refresh resources' })).toBeVisible()
  expect(screen.queryByRole('menuitem', { name: 'Create local group' })).not.toBeInTheDocument()
  fireEvent.keyDown(screen.getByRole('menu'), { key: 'Escape' })
  const panel = await details()
  fireEvent.click(within(panel).getByRole('button', { name: 'More actions' }))
  expect(await screen.findByRole('menuitem', { name: 'Refresh resources' })).toBeVisible()
  expect(screen.queryByRole('menuitem', { name: 'Rename group' })).not.toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Delete group' })).not.toBeInTheDocument()
})

it('retains a referenced local resource after the server rejects its deletion', async () => {
  const h = setup()
  const original = h.request.getMockImplementation()!
  h.request.mockImplementation((input, signal) =>
    input.method === 'DELETE' && input.path === '/catalog/local-second'
      ? Promise.resolve({
          status: 409,
          body: {
            schema: 1,
            ok: false,
            code: 'catalog.provider_in_use',
            message: 'private original details',
            data: {},
          },
        })
      : original(input, signal),
  )
  const panel = await details()
  fireEvent.click(within(panel).getByRole('button', { name: 'More actions' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Delete group' }))
  fireEvent.click(await screen.findByRole('button', { name: 'Confirm delete' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'The group is still referenced by a policy or outbound.',
  )
  expect(screen.getByRole('alert')).not.toHaveTextContent('private original details')
  expect(screen.queryByText('Group deleted')).not.toBeInTheDocument()
  closePanel()
  expect(await screen.findByRole('dialog', { name: 'Local second' })).toBeVisible()
  expect(
    screen.getByRole('button', { name: 'View details Local second', hidden: true }),
  ).toBeInTheDocument()
})

it('offers manual ProviderTag copying when clipboard access fails', async () => {
  const copy = vi.fn().mockRejectedValue(new Error('fixture clipboard denied'))
  vi.stubGlobal(
    'navigator',
    Object.create(navigator, { clipboard: { value: { writeText: copy }, configurable: true } }),
  )
  setup(<ResourceProviderTag group={group('local-fixture', 'Fixture', 'local')} />)
  fireEvent.click(await screen.findByRole('button', { name: 'Copy ProviderTag' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Copy failed. Select the ProviderTag and copy it manually.',
  )
  expect(screen.getByText('np/local-fixture')).toHaveClass('select-text')
  expect(screen.getByRole('button', { name: 'Copy ProviderTag' })).toBeEnabled()
  expect(screen.getByRole('alert')).not.toHaveTextContent('fixture clipboard denied')
})

it('copies ProviderTag in subscription editing without reading private settings', async () => {
  const copy = vi.fn().mockResolvedValue(undefined)
  vi.stubGlobal(
    'navigator',
    Object.create(navigator, { clipboard: { value: { writeText: copy }, configurable: true } }),
  )
  const h = setup()
  const panel = await details('Subscription fixture')
  fireEvent.click(within(panel).getByRole('button', { name: 'More actions' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Edit subscription' }))
  fireEvent.click(await screen.findByRole('button', { name: 'Copy ProviderTag' }))
  expect(await screen.findByText('ProviderTag copied')).toBeVisible()
  expect(copy).toHaveBeenCalledExactlyOnceWith('np/source-fixture')
  expect(h.request.mock.calls.every(([input]) => input.method === 'GET')).toBe(true)
  expect(h.request.mock.calls.some(([input]) => input.query?.private !== undefined)).toBe(false)
})

it.each(['x'.repeat(257), '\u754c'.repeat(86), 'fixture\u0001name'])(
  'rejects an invalid local resource name before admission: %s',
  async (name) => {
    const h = setup()
    fireEvent.click(await screen.findByRole('button', { name: 'Resource actions' }))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Create local group' }))
    fireEvent.change(await screen.findByRole('textbox', { name: 'Group ID' }), {
      target: { value: 'name-fixture' },
    })
    fireEvent.change(screen.getByRole('textbox', { name: 'Group name' }), {
      target: { value: name },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Enter an available, valid group ID and name.',
    )
    expect(h.request.mock.calls.every(([input]) => input.method === 'GET')).toBe(true)
  },
)

it('does not replay local group deletion after an ambiguous server response', async () => {
  const h = setup()
  const original = h.request.getMockImplementation()!
  h.request.mockImplementation((input, signal) =>
    input.method === 'DELETE'
      ? Promise.resolve({
          status: 502,
          body: {
            schema: 1,
            ok: false,
            code: 'fixture.failed',
            message: 'private fixture',
            data: {},
          },
        })
      : original(input, signal),
  )
  const panel = await details()
  fireEvent.click(within(panel).getByRole('button', { name: 'More actions' }))
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Delete group' }))
  fireEvent.click(await screen.findByRole('button', { name: 'Confirm delete' }))
  expect(await screen.findByText('Result not confirmed. Refresh resources to check.')).toBeVisible()
  const confirm = screen.getByRole('button', { name: 'Confirm delete' })
  expect(confirm).toBeDisabled()
  fireEvent.click(confirm)
  expect(h.request.mock.calls.filter(([input]) => input.method === 'DELETE')).toHaveLength(1)
})

it.each(['lost', 'persisted', 'invalid'] as const)(
  'keeps a local node %s write unconfirmed and its draft out of shared caches',
  async (failure) => {
    const h = setup()
    const original = h.request.getMockImplementation()!
    h.request.mockImplementation((input, signal) => {
      if (input.method !== 'PUT') return original(input, signal)
      if (failure === 'lost') return Promise.reject(new TypeError('private node fixture'))
      return Promise.resolve({
        status: failure === 'persisted' ? 502 : 200,
        body: {
          schema: 1,
          ok: failure !== 'persisted',
          code: failure === 'persisted' ? 'node.runtime_sync_failed' : 'fixture.invalid',
          message: 'private node fixture',
          data: failure === 'persisted' ? { persisted: true, runtime_synced: false } : null,
        },
      })
    })
    await details()
    await nodeAction('local-second node', 'Edit node')
    const editor = await screen.findByRole('textbox', { name: 'Node content' })
    await submitNode()
    expect(
      await screen.findByText('Result not confirmed. Close and refresh resources to check.'),
    ).toBeVisible()
    expect(editor).toHaveAttribute('readonly')
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
    expect(screen.queryByText('Operation completed. Nodes refreshed.')).not.toBeInTheDocument()
    expect(screen.getByRole('alert')).not.toHaveTextContent('private node fixture')
    expect(
      JSON.stringify(
        h.cache
          .getQueryCache()
          .getAll()
          .map((query) => query.state.data),
      ),
    ).not.toContain('test-only-node-secret')
    expect(h.cache.getMutationCache().getAll()).toHaveLength(0)
    expect(h.request.mock.calls.filter(([input]) => input.method === 'PUT')).toHaveLength(1)
  },
)

it('keeps a rejected node draft editable for explicit correction without automatically retrying', async () => {
  const h = setup()
  const original = h.request.getMockImplementation()!
  h.request.mockImplementation((input, signal) =>
    input.method === 'PUT'
      ? Promise.resolve({
          status: 422,
          body: {
            schema: 1,
            ok: false,
            code: 'node.invalid',
            message: 'private fixture',
            data: {},
          },
        })
      : original(input, signal),
  )
  await details()
  await nodeAction('local-second node', 'Edit node')
  const editor = await screen.findByRole('textbox', { name: 'Node content' })
  await submitNode()
  await screen.findByRole('alert')
  await waitFor(() => expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled())
  expect(editor).not.toHaveAttribute('readonly')
  expect((editor as HTMLTextAreaElement).value).toContain('test-only-node-secret')
  expect(h.request.mock.calls.filter(([input]) => input.method === 'PUT')).toHaveLength(1)
  h.request.mockImplementation(original)
  fireEvent.change(editor, { target: { value: '{"type":"socks","tag":"corrected"}' } })
  await submitNode()
  await screen.findByText('Operation completed. Nodes refreshed.')
  expect(h.request.mock.calls.filter(([input]) => input.method === 'PUT')).toHaveLength(2)
})

it('disables node writes while rechecking a stale Catalog snapshot and retains the draft on failure', async () => {
  const h = setup()
  await details()
  await nodeAction('local-second node', 'Edit node')
  const editor = await screen.findByRole('textbox', { name: 'Node content' })
  await waitFor(() => expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled())
  const original = h.request.getMockImplementation()!
  let rejectRead!: (error: Error) => void
  h.request.mockImplementation((input, signal) =>
    input.path === '/nodes/snapshot'
      ? new Promise((_resolve, reject) => {
          rejectRead = reject
        })
      : original(input, signal),
  )
  act(() => {
    void h.cache.invalidateQueries({ queryKey: catalogNodesKey })
  })
  await waitFor(() => expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled())
  await act(async () => rejectRead(new TypeError('private snapshot fixture')))
  expect(await screen.findByRole('alert')).not.toHaveTextContent('private snapshot fixture')
  expect((editor as HTMLTextAreaElement).value).toContain('test-only-node-secret')
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  expect(h.request.mock.calls.every(([input]) => input.method === 'GET')).toBe(true)
})
