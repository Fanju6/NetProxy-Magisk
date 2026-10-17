import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import type { SubscriptionInput } from '@/api/subscription-input'
import { PreferencesContext } from '@/app/context'
import { translator } from '@/app/i18n'
import { demoGroup } from '@/mocks/fixtures'
import { SubscriptionEditor, type SubscriptionEditorProps } from './subscription-editor'

afterEach(() => vi.restoreAllMocks())
const group = demoGroup('anonymous-subscription', 'Anonymous subscription', 'subscription', 3)
function renderEditor(props: Partial<SubscriptionEditorProps> = {}) {
  const onSubmit = vi.fn<(input: SubscriptionInput) => boolean | Promise<boolean>>(() => false)
  const onClose = vi.fn()
  const view = render(
    <PreferencesContext
      value={{
        preferences: { theme: 'light', locale: 'en' },
        update: () => {},
        t: translator('en'),
      }}
    >
      <SubscriptionEditor busy={false} onSubmit={onSubmit} onClose={onClose} {...props} />
    </PreferencesContext>,
  )
  return { ...view, onSubmit, onClose }
}
function save() {
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
}
function modify(field: string) {
  const summary = screen.getByText('Advanced replacements')
  if (!summary.parentElement?.hasAttribute('open')) fireEvent.click(summary)
  fireEvent.click(screen.getByRole('switch', { name: `Modify ${field}` }))
}

it('sends only changed public fields; unread settings are never filled or cleared', async () => {
  const h = renderEditor({ group })
  expect(screen.getByText(/Private settings are not displayed/)).toBeVisible()
  expect(screen.getByRole('combobox', { name: 'Download connection' })).toHaveTextContent(
    'Automatic (proxy when available)',
  )
  expect(screen.queryByRole('textbox', { name: 'Subscription URL' })).not.toBeInTheDocument()
  fireEvent.change(screen.getByRole('textbox', { name: 'Name' }), { target: { value: 'Renamed' } })
  save()
  await waitFor(() => expect(h.onSubmit).toHaveBeenCalledWith({ name: 'Renamed' }))
  expect(h.onClose).not.toHaveBeenCalled()
})

it('distinguishes omitted settings from explicit text/header clearing and false', async () => {
  const h = renderEditor({ group })
  for (const name of [
    'User-Agent',
    'HWID',
    'Include expression',
    'Exclude expression',
    'Custom headers (JSON object)',
    'Subscription TLS verification',
  ])
    modify(name)
  save()
  await waitFor(() =>
    expect(h.onSubmit).toHaveBeenCalledWith({
      user_agent: '',
      hwid: '',
      include: '',
      exclude: '',
      custom_headers: {},
      allow_insecure: false,
    }),
  )
})

it('requires a valid nonempty URL for creation and replacement without echoing secrets', async () => {
  const h = renderEditor()
  save()
  expect(await screen.findByRole('alert')).toBeVisible()
  expect(h.onSubmit).not.toHaveBeenCalled()
  fireEvent.change(screen.getByRole('textbox', { name: 'Subscription URL' }), {
    target: { value: 'file:///anonymous-private-secret' },
  })
  save()
  expect(screen.getByRole('alert')).not.toHaveTextContent('anonymous-private-secret')
  expect(h.onSubmit).not.toHaveBeenCalled()
  fireEvent.change(screen.getByRole('textbox', { name: 'Subscription URL' }), {
    target: { value: 'https://example.invalid/feed?key=anonymous' },
  })
  save()
  await waitFor(() =>
    expect(h.onSubmit).toHaveBeenCalledWith({
      url: 'https://example.invalid/feed?key=anonymous',
      auto_update: true,
      update_interval: 86400,
      update_via_proxy: 'auto',
    }),
  )
  h.unmount()
  const edit = renderEditor({ group })
  modify('Subscription URL')
  save()
  expect(await screen.findByRole('alert')).toBeVisible()
  expect(edit.onSubmit).not.toHaveBeenCalled()
})

it('drops private inputs after accepted admission and never persists them', async () => {
  const setItem = vi.spyOn(Storage.prototype, 'setItem')
  const onSubmit = vi.fn(() => true)
  const h = renderEditor({ group, onSubmit })
  modify('User-Agent')
  fireEvent.change(screen.getByRole('textbox', { name: 'User-Agent' }), {
    target: { value: 'anonymous-sensitive-agent' },
  })
  save()
  await waitFor(() => expect(h.onClose).toHaveBeenCalledOnce())
  expect(screen.queryByDisplayValue('anonymous-sensitive-agent')).not.toBeInTheDocument()
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  expect(setItem).not.toHaveBeenCalled()
  expect(window.location.href).not.toContain('anonymous-sensitive-agent')
})

it('discards optional replacements when disabled and drops drafts on close/unmount', () => {
  const h = renderEditor({ group })
  modify('HWID')
  fireEvent.change(screen.getByRole('textbox', { name: 'HWID' }), {
    target: { value: 'anonymous-hwid' },
  })
  modify('HWID')
  expect(screen.queryByDisplayValue('anonymous-hwid')).not.toBeInTheDocument()
  modify('HWID')
  expect(screen.getByRole('textbox', { name: 'HWID' })).toHaveValue('')
  fireEvent.change(screen.getByRole('textbox', { name: 'HWID' }), {
    target: { value: 'anonymous-hwid' },
  })
  fireEvent.click(screen.getAllByRole('button', { name: 'Close' }).at(-1)!)
  expect(h.onClose).not.toHaveBeenCalled()
  expect(screen.getByDisplayValue('anonymous-hwid')).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Discard changes' }))
  expect(h.onClose).toHaveBeenCalledOnce()
  expect(screen.queryByDisplayValue('anonymous-hwid')).not.toBeInTheDocument()
  h.unmount()
  renderEditor({ group })
  expect(screen.queryByRole('textbox', { name: 'HWID' })).not.toBeInTheDocument()
})

it('does not duplicate submission or invoke close after unmount during admission', async () => {
  let admit!: (value: boolean) => void
  const onSubmit = vi.fn(
    () =>
      new Promise<boolean>((resolve) => {
        admit = resolve
      }),
  )
  const h = renderEditor({ group, onSubmit })
  save()
  fireEvent.submit(screen.getByRole('textbox', { name: 'Name' }).closest('form')!)
  expect(onSubmit).toHaveBeenCalledOnce()
  h.unmount()
  await act(async () => {
    admit(true)
  })
  expect(h.onClose).not.toHaveBeenCalled()
})

it('handles callback rejection without exposing the error or losing the draft', async () => {
  renderEditor({ group, onSubmit: () => Promise.reject(new Error('anonymous-private-secret')) })
  fireEvent.change(screen.getByRole('textbox', { name: 'Name' }), {
    target: { value: 'Keep draft' },
  })
  save()
  expect(await screen.findByRole('alert')).not.toHaveTextContent('anonymous-private-secret')
  expect(screen.getByRole('textbox', { name: 'Name' })).toHaveValue('Keep draft')
})

it('never edits a local group and rejects malformed custom headers safely', async () => {
  const local = renderEditor({ group: demoGroup('default', 'Local', 'local', 1) })
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  local.unmount()
  const h = renderEditor({ group })
  modify('Custom headers (JSON object)')
  fireEvent.change(screen.getByRole('textbox', { name: 'Custom headers (JSON object)' }), {
    target: { value: '{"Authorization": "anonymous-secret"' },
  })
  save()
  expect(await screen.findByRole('alert')).not.toHaveTextContent('anonymous-secret')
  expect(h.onSubmit).not.toHaveBeenCalled()
})

it('does not admit operations while busy or disabled', () => {
  const busy = renderEditor({ group, busy: true })
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  fireEvent.submit(screen.getByRole('textbox', { name: 'Name' }).closest('form')!)
  expect(busy.onSubmit).not.toHaveBeenCalled()
  busy.unmount()
  const disabled = renderEditor({ group, disabled: true })
  fireEvent.submit(screen.getByRole('textbox', { name: 'Name' }).closest('form')!)
  expect(disabled.onSubmit).not.toHaveBeenCalled()
})

it('closes once if a delayed admission finishes after dismissal', async () => {
  let admit!: (value: boolean) => void
  const h = renderEditor({
    group,
    onSubmit: () =>
      new Promise<boolean>((resolve) => {
        admit = resolve
      }),
  })
  save()
  fireEvent.click(screen.getAllByRole('button', { name: 'Close' }).at(-1)!)
  expect(h.onClose).toHaveBeenCalledOnce()
  await act(async () => {
    admit(true)
  })
  expect(h.onClose).toHaveBeenCalledOnce()
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
})

it('uses the existing mobile drawer and keeps inputs touch-sized', async () => {
  const media = window.matchMedia('(max-width: 767px)')
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    ...media,
    media: query,
    matches: query === '(max-width: 767px)',
  }))
  const h = renderEditor()
  expect(await screen.findByRole('dialog')).toBeVisible()
  expect(screen.getByRole('textbox', { name: 'Subscription URL' })).toHaveClass('h-12')
  expect(screen.getByRole('textbox', { name: 'Subscription URL' })).toHaveClass('text-base')
  fireEvent.click(screen.getAllByRole('button', { name: 'Close' }).at(-1)!)
  expect(h.onClose).toHaveBeenCalledOnce()
})
