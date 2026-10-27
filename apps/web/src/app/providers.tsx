import { useEffect, useState, type ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ApiClient } from '@/api/client'
import { TooltipProvider } from '@/components/ui/tooltip'
import { ToastProvider } from '@/components/ui/toast'
import { ApiContext, PreferencesContext, type Preferences } from './context'
import { translator } from './i18n'
import { syncNativeAppearance } from './appearance'

const key = 'netproxy.web.preferences'
function initialPreferences(): Preferences {
  const fallback: Preferences = {
    theme: 'system',
    locale: navigator.language.startsWith('zh') ? 'zh' : 'en',
  }
  try {
    const saved: unknown = JSON.parse(localStorage.getItem(key) ?? 'null')
    if (
      typeof saved === 'object' &&
      saved !== null &&
      'theme' in saved &&
      'locale' in saved &&
      ['system', 'light', 'dark'].includes(String(saved.theme)) &&
      ['zh', 'en'].includes(String(saved.locale))
    ) {
      return {
        theme: saved.theme as Preferences['theme'],
        locale: saved.locale as Preferences['locale'],
      }
    }
  } catch {
    /* 本地偏好不可读时不影响业务连接。 */
  }
  return fallback
}

export function AppProviders({ api, children }: { api: ApiClient; children: ReactNode }) {
  const [preferences, setPreferences] = useState(initialPreferences)
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: { staleTime: 30_000, retry: false, refetchOnWindowFocus: false },
          mutations: { retry: false },
        },
      }),
  )
  useEffect(() => {
    const media = matchMedia('(prefers-color-scheme: dark)')
    const apply = () => {
      const dark = preferences.theme === 'dark' || (preferences.theme === 'system' && media.matches)
      document.documentElement.classList.toggle('dark', dark)
      document.documentElement.style.colorScheme = dark ? 'dark' : 'light'
      void syncNativeAppearance(dark ? 'dark' : 'light')
      document.documentElement.lang = preferences.locale === 'zh' ? 'zh-CN' : 'en'
      document
        .querySelector('meta[name="theme-color"]')
        ?.setAttribute('content', dark ? '#09090b' : '#fafafa')
    }
    apply()
    media.addEventListener('change', apply)
    try {
      localStorage.setItem(key, JSON.stringify(preferences))
    } catch {
      /* 仅持久化非敏感外观偏好。 */
    }
    return () => media.removeEventListener('change', apply)
  }, [preferences])
  return (
    <PreferencesContext
      value={{
        preferences,
        t: translator(preferences.locale),
        update: (patch) => setPreferences((value) => ({ ...value, ...patch })),
      }}
    >
      <ApiContext value={api}>
        <QueryClientProvider client={queryClient}>
          <TooltipProvider>
            <ToastProvider>{children}</ToastProvider>
          </TooltipProvider>
        </QueryClientProvider>
      </ApiContext>
    </PreferencesContext>
  )
}
