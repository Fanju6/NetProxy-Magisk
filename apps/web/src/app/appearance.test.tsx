import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { clearMocks, mockIPC } from '@tauri-apps/api/mocks'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiClient, unavailableTransport } from '@/api/client'
import { syncNativeAppearance } from './appearance'
import { usePreferences, type Preferences } from './context'
import { AppProviders } from './providers'

afterEach(() => {
  clearMocks()
  vi.unstubAllGlobals()
  vi.unstubAllEnvs()
})

function native() {
  vi.stubEnv('MODE', 'native')
  vi.stubGlobal('isTauri', true)
  const calls = vi.fn().mockResolvedValue(undefined)
  mockIPC(calls)
  return calls
}

function media(initialDark = false) {
  let matches = initialDark
  const listeners = new Set<() => void>()
  const addEventListener = vi.fn((_name: string, listener: () => void) => listeners.add(listener))
  const removeEventListener = vi.fn((_name: string, listener: () => void) =>
    listeners.delete(listener),
  )
  vi.stubGlobal(
    'matchMedia',
    vi.fn(() => ({
      get matches() {
        return matches
      },
      addEventListener,
      removeEventListener,
    })),
  )
  return {
    addEventListener,
    removeEventListener,
    change(dark: boolean) {
      act(() => {
        matches = dark
        listeners.forEach((listener) => listener())
      })
    },
  }
}

function ThemeControls() {
  const { update } = usePreferences()
  return (['light', 'dark', 'system'] as const).map((theme) => (
    <button key={theme} onClick={() => update({ theme })}>
      {theme}
    </button>
  ))
}

function page(theme?: Preferences['theme']) {
  if (theme)
    localStorage.setItem('netproxy.web.preferences', JSON.stringify({ theme, locale: 'en' }))
  return render(
    <AppProviders api={new ApiClient(unavailableTransport)}>
      <ThemeControls />
    </AppProviders>,
  )
}

describe('native system bar appearance', () => {
  it.each(['production', 'demo', 'development', 'test'])(
    'does not invoke native IPC from a %s build, even inside Tauri',
    async (mode) => {
      const calls = native()
      vi.stubEnv('MODE', mode)
      await syncNativeAppearance('dark')
      expect(calls).not.toHaveBeenCalled()
    },
  )

  it('does not invoke IPC from a native build outside Tauri', async () => {
    const calls = native()
    vi.stubGlobal('isTauri', false)
    await syncNativeAppearance('light')
    expect(calls).not.toHaveBeenCalled()
  })

  it('only invokes the restricted plugin command with the resolved enum', async () => {
    const calls = native()
    await syncNativeAppearance('light')
    await syncNativeAppearance('dark')
    expect(calls.mock.calls).toEqual([
      ['plugin:module-api|set_appearance', { theme: 'light' }],
      ['plugin:module-api|set_appearance', { theme: 'dark' }],
    ])
  })

  it('keeps native failures out of page errors', async () => {
    const calls = native()
    calls.mockRejectedValue(new Error('private native failure'))
    await expect(syncNativeAppearance('dark')).resolves.toBeUndefined()
  })

  it('uses the same effective theme as the DOM and follows system media changes', async () => {
    const calls = native()
    const system = media(true)
    const view = page()
    await waitFor(() =>
      expect(calls).toHaveBeenLastCalledWith('plugin:module-api|set_appearance', { theme: 'dark' }),
    )
    expect(document.documentElement).toHaveClass('dark')
    expect(document.documentElement.style.colorScheme).toBe('dark')
    system.change(false)
    await waitFor(() =>
      expect(calls).toHaveBeenLastCalledWith('plugin:module-api|set_appearance', {
        theme: 'light',
      }),
    )
    expect(document.documentElement).not.toHaveClass('dark')
    expect(document.documentElement.style.colorScheme).toBe('light')
    view.unmount()
    expect(system.removeEventListener).toHaveBeenCalledWith('change', expect.any(Function))
    calls.mockClear()
    system.change(true)
    await Promise.resolve()
    expect(calls).not.toHaveBeenCalled()
  })

  it('keeps explicit choices across opposite system changes, then resumes system matching', async () => {
    const calls = native()
    const system = media(true)
    page('light')
    await waitFor(() =>
      expect(calls).toHaveBeenLastCalledWith('plugin:module-api|set_appearance', {
        theme: 'light',
      }),
    )
    system.change(false)
    system.change(true)
    await waitFor(() => expect(calls).toHaveBeenCalledTimes(3))
    expect(calls.mock.calls.every(([, args]) => args.theme === 'light')).toBe(true)
    expect(document.documentElement).not.toHaveClass('dark')
    fireEvent.click(screen.getByRole('button', { name: 'dark' }))
    await waitFor(() =>
      expect(calls).toHaveBeenLastCalledWith('plugin:module-api|set_appearance', { theme: 'dark' }),
    )
    system.change(false)
    await waitFor(() => expect(calls).toHaveBeenCalledTimes(5))
    expect(calls).toHaveBeenLastCalledWith('plugin:module-api|set_appearance', { theme: 'dark' })
    expect(document.documentElement).toHaveClass('dark')
    fireEvent.click(screen.getByRole('button', { name: 'system' }))
    await waitFor(() =>
      expect(calls).toHaveBeenLastCalledWith('plugin:module-api|set_appearance', {
        theme: 'light',
      }),
    )
    expect(document.documentElement).not.toHaveClass('dark')
  })
})
