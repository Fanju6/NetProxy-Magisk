import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { RouterProvider } from '@tanstack/react-router'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'
import { ApiClient } from '@/api/client'
import type { ApiTransport } from '@/api/contracts'
import { AppProviders } from '@/app/providers'
import { makeRouter } from '@/app/router'
import { MockTransport } from '@/mocks/transport'

// Keep editor internals out of the page workflow tests.
vi.mock('@/components/json-editor', () => ({
  JsonEditor: ({
    value,
    onChange,
    readOnly,
    label,
  }: {
    value: string
    onChange?: (value: string) => void
    readOnly?: boolean
    label?: string
  }) => (
    <textarea
      aria-label={label}
      value={value}
      readOnly={readOnly}
      onChange={(event) => onChange?.(event.target.value)}
    />
  ),
}))

afterEach(() => vi.restoreAllMocks())

async function setup(locale: 'en' | 'zh' = 'en') {
  const backend = new MockTransport(0)
  const request = vi.fn<ApiTransport['request']>((input, signal) => backend.request(input, signal))
  const transport: ApiTransport = {
    kind: 'mock',
    capabilities: backend.capabilities,
    request,
    subscribe: (topics, consume) => backend.subscribe(topics, consume),
  }
  localStorage.setItem('netproxy.web.preferences', JSON.stringify({ theme: 'light', locale }))
  window.location.hash = '#/nodes'
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  const router = makeRouter()
  const view = render(
    <AppProviders api={new ApiClient(transport)}>
      <RouterProvider router={router} />
    </AppProviders>,
  )
  await screen.findByRole(
    'button',
    { name: locale === 'en' ? 'Add node' : '添加节点' },
    { timeout: 10000 },
  )
  return { request, ...view }
}

async function details(name = 'Local relay') {
  fireEvent.click(screen.getByRole('button', { name: `View details ${name}` }))
  return screen.findByRole('dialog')
}

function closePanel() {
  fireEvent.click(screen.getAllByRole('button', { name: 'Close' }).at(-1)!)
}

async function nodeAction(panel: HTMLElement, name: string) {
  fireEvent.click(within(panel).getByRole('button', { name: 'More actions' }))
  fireEvent.click(await screen.findByRole('menuitem', { name }))
}

it('defaults to a list and keeps group and individual tests beside direct selection', async () => {
  const h = await setup()
  for (const grid of screen.getAllByTestId('node-grid')) expect(grid).toHaveClass('grid-cols-1')
  expect(screen.getByRole('button', { name: 'Test group Global Connect' })).toHaveClass('size-12')
  expect(screen.getByRole('button', { name: 'Test latency for Tokyo 01' })).toHaveClass('size-12')
  const select = screen.getByRole('button', { name: 'Select Tokyo 01' })
  await waitFor(() => expect(select).toBeEnabled())
  fireEvent.click(select)
  await waitFor(() =>
    expect(
      h.request.mock.calls.some(
        ([input]) =>
          input.path === '/selection' &&
          input.method === 'PUT' &&
          (input.body as { node_tag: string }).node_tag === 'Tokyo 01',
      ),
    ).toBe(true),
  )
})

it('offers manual creation and text import without an operation or node picker', async () => {
  const h = await setup()
  fireEvent.click(screen.getByRole('button', { name: 'Add node' }))
  expect(await screen.findByRole('menuitem', { name: 'Add node manually' })).toBeVisible()
  fireEvent.click(screen.getByRole('menuitem', { name: 'Import text' }))
  expect(await screen.findByRole('dialog')).toBeVisible()
  expect(screen.queryByRole('combobox', { name: 'Operation' })).not.toBeInTheDocument()
  expect(screen.queryByRole('combobox', { name: 'Node' })).not.toBeInTheDocument()
  fireEvent.change(screen.getByRole('textbox', { name: 'Node content' }), {
    target: { value: 'socks://127.0.0.1:1080#Imported%20fixture' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  expect(await screen.findByText('Operation completed. Nodes refreshed.')).toBeVisible()
  expect(
    h.request.mock.calls.filter(
      ([input]) => input.method === 'POST' && input.path === '/nodes/import',
    ),
  ).toHaveLength(1)
  expect(screen.queryByRole('textbox', { name: 'Node content' })).not.toBeInTheDocument()
})

it('edits the node opened from its own details and confirms before discarding its draft', async () => {
  const h = await setup()
  const panel = await details()
  await nodeAction(panel, 'Edit node')
  const editor = await screen.findByRole('textbox', { name: 'Node content' })
  expect(editor.closest('[data-panel]')).toHaveClass('editor-panel')
  expect(h.request.mock.calls.find(([input]) => input.path === '/nodes/get')?.[0].query).toEqual({
    group_id: 'default',
    node_tag: 'Local relay',
  })
  const original = (editor as HTMLTextAreaElement).value
  fireEvent.change(editor, { target: { value: `${original}\n ` } })
  closePanel()
  expect(screen.getByRole('button', { name: 'Keep editing' })).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Keep editing' }))
  expect(editor).toHaveValue(`${original}\n `)
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  expect(await screen.findByText('Operation completed. Nodes refreshed.')).toBeVisible()
  expect(
    h.request.mock.calls
      .filter(([input]) => input.method === 'PUT' && input.path === '/nodes')
      .map(([input]) => input.body),
  ).toEqual([
    {
      group_id: 'default',
      node_tag: 'Local relay',
      content: `${original}\n `,
      allow_insecure: false,
    },
  ])
})

it('shares subscription nodes but retains local-only edit/delete and explicit delete confirmation', async () => {
  const h = await setup()
  let panel = await details('Tokyo 01')
  expect(within(panel).queryByRole('button', { name: 'Edit node' })).not.toBeInTheDocument()
  await nodeAction(panel, 'Share link')
  const share = await screen.findByRole('textbox', { name: 'Node content' })
  expect(share).toHaveAttribute('readonly')
  expect(h.request.mock.calls.find(([input]) => input.path === '/nodes/export')?.[0].query).toEqual(
    { group_id: 'demo-global', node_tag: 'Tokyo 01' },
  )
  closePanel()
  await waitFor(() =>
    expect(screen.queryByRole('textbox', { name: 'Node content' })).not.toBeInTheDocument(),
  )
  panel = await details()
  await nodeAction(panel, 'Delete node')
  expect(await screen.findByRole('button', { name: 'Confirm delete' })).toBeVisible()
  expect(h.request.mock.calls.filter(([input]) => input.method === 'DELETE')).toHaveLength(0)
  fireEvent.click(screen.getByRole('button', { name: 'Confirm delete' }))
  expect(await screen.findByText('Operation completed. Nodes refreshed.')).toBeVisible()
  expect(
    h.request.mock.calls
      .filter(([input]) => input.method === 'DELETE')
      .map(([input]) => input.query),
  ).toEqual([{ group_id: 'default', node_tag: 'Local relay' }])
})

it('copies a share link only after an explicit click without putting it in persistent storage', async () => {
  await setup()
  userEvent.setup()
  const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
  const panel = await details('Tokyo 01')
  await nodeAction(panel, 'Share link')
  const input = await screen.findByRole('textbox', { name: 'Node content' })
  await waitFor(() => expect(input).not.toHaveValue(''))
  const link = (input as HTMLTextAreaElement).value
  expect(copy).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'Copy link' }))
  expect(await screen.findByText('Link copied')).toBeVisible()
  expect(copy).toHaveBeenCalledExactlyOnceWith(link)
  expect(JSON.stringify(localStorage)).not.toContain(link)
})

it('keeps Chinese add labels and long names available', async () => {
  await setup('zh')
  fireEvent.click(screen.getByRole('button', { name: '添加节点' }))
  expect(await screen.findByRole('menuitem', { name: '手动添加节点' })).toBeVisible()
  expect(screen.getByRole('menuitem', { name: '导入文本' })).toBeVisible()
})
