import { Fragment, lazy, Suspense, useEffect, useState } from 'react'
import { Link, Outlet, useLocation, useRouter } from '@tanstack/react-router'
import { LayoutDashboard, Network, Rss, Settings, ArrowLeft } from 'lucide-react'
import { useApi, usePreferences } from './context'
import { ModuleEvents } from '@/api/events'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { HeaderActionsProvider } from './header-actions'
import { LoadingState } from '@/components/states'

const AndroidImports =
  import.meta.env.MODE === 'native'
    ? lazy(() =>
        import('@/features/android-imports').then((module) => ({ default: module.AndroidImports })),
      )
    : Fragment

const routes = [
  { to: '/', key: 'dashboard', icon: LayoutDashboard },
  { to: '/nodes', key: 'proxies', icon: Network },
  { to: '/subscriptions', key: 'subscriptions', icon: Rss },
  { to: '/settings', key: 'settings', icon: Settings },
] as const

export function AppShell() {
  const { t } = usePreferences()
  const api = useApi()
  const router = useRouter()
  const location = useLocation()
  const pathname = location.pathname
  const [actionsTarget, setActionsTarget] = useState<HTMLDivElement | null>(null)
  const current =
    routes.find(
      (route) => route.to === pathname || (route.to !== '/' && pathname.startsWith(`${route.to}/`)),
    ) ?? routes[0]
  const secondary = pathname.startsWith('/settings/')
  const section = 'section' in location.search ? location.search.section : undefined
  const title =
    pathname === '/settings/configuration'
      ? t(
          section === 'rules'
            ? 'routes'
            : section === 'core'
              ? 'kernel'
              : 'configurationManagement',
        )
      : pathname === '/settings/policies'
        ? t(
            section === 'wifi'
              ? 'network'
              : section === 'apps'
                ? 'appPolicy'
                : section === 'root'
                  ? 'rootPolicy'
                  : 'inbound',
          )
        : pathname === '/settings/logs'
          ? t('logs')
          : t(current.key)

  useEffect(() => {
    let baseline = window.innerHeight
    const update = () => {
      const editing = document.activeElement?.matches('input, textarea, [contenteditable="true"]')
      const height = window.visualViewport?.height ?? window.innerHeight
      if (!editing) baseline = Math.max(height, window.innerHeight)
      const keyboard = Boolean(editing && baseline - height > 120)
      const root = document.documentElement
      root.toggleAttribute('data-keyboard-open', keyboard)
      // Android WebView can shrink only the visual viewport, leaving dvh behind the IME.
      const top = window.visualViewport?.offsetTop ?? 0
      for (const [name, value] of Object.entries({
        '--keyboard-viewport-height': height,
        '--keyboard-viewport-top': top,
        '--keyboard-inset': Math.max(0, window.innerHeight - height - top),
      })) {
        if (keyboard) root.style.setProperty(name, `${value}px`)
        else root.style.removeProperty(name)
      }
    }
    window.visualViewport?.addEventListener('resize', update)
    window.visualViewport?.addEventListener('scroll', update)
    window.addEventListener('resize', update)
    document.addEventListener('focusin', update)
    document.addEventListener('focusout', update)
    return () => {
      window.visualViewport?.removeEventListener('resize', update)
      window.visualViewport?.removeEventListener('scroll', update)
      window.removeEventListener('resize', update)
      document.removeEventListener('focusin', update)
      document.removeEventListener('focusout', update)
      document.documentElement.removeAttribute('data-keyboard-open')
      for (const name of [
        '--keyboard-viewport-height',
        '--keyboard-viewport-top',
        '--keyboard-inset',
      ])
        document.documentElement.style.removeProperty(name)
    }
  }, [])

  return (
    <Suspense fallback={<LoadingState />}>
      <AndroidImports>
        <HeaderActionsProvider target={actionsTarget}>
          <div className="app-shell">
            <a
              href="#main"
              onClick={(event) => {
                event.preventDefault()
                document.getElementById('main')?.focus()
              }}
              className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-50 focus:bg-background focus:p-3"
            >
              {t('skip')}
            </a>
            <aside className="desktop-sidebar">
              <Link to="/" className="flex items-center gap-3 px-3 py-3">
                <img
                  src={`${import.meta.env.BASE_URL}netproxy.svg`}
                  width="28"
                  height="28"
                  alt=""
                />
                <span className="font-semibold">NETPROXY</span>
                <span className="ml-auto text-xs text-muted-foreground">V9</span>
              </Link>
              <nav aria-label={t('mainNavigation')} className="mt-6 space-y-1">
                {routes.map((route) => (
                  <Link
                    key={route.to}
                    to={route.to}
                    aria-current={current.to === route.to ? 'page' : undefined}
                    className={cn(
                      'nav-link',
                      current.to === route.to && 'bg-accent font-medium text-foreground',
                    )}
                  >
                    <route.icon className="size-5" />
                    {t(route.key)}
                  </Link>
                ))}
              </nav>
            </aside>
            <div className="min-w-0 flex-1">
              <header className="app-header">
                <div className="flex min-w-0 items-center gap-2">
                  {secondary && (
                    <Button
                      variant="ghost"
                      size="icon"
                      className="-ml-2 size-12 shrink-0"
                      aria-label={t('back')}
                      onClick={() => {
                        if (router.history.canGoBack()) router.history.back()
                        else void router.navigate({ to: '/settings', replace: true })
                      }}
                    >
                      <ArrowLeft />
                    </Button>
                  )}
                  <h1 className="truncate text-lg font-semibold">{title}</h1>
                </div>
                <div className="flex shrink-0 items-center">
                  {api.transport.kind !== 'unavailable' && <ModuleEvents />}
                  <div ref={setActionsTarget} className="flex items-center" />
                </div>
              </header>
              <main id="main" className="page-content" tabIndex={-1}>
                <Outlet />
              </main>
            </div>
            <nav aria-label={t('mainNavigation')} className="mobile-navigation">
              {routes.map((route) => (
                <Link
                  key={route.to}
                  to={route.to}
                  aria-current={current.to === route.to ? 'page' : undefined}
                  className={cn(
                    'mobile-nav-link',
                    current.to === route.to && 'mobile-nav-selected',
                  )}
                >
                  <span className="nav-icon">
                    <route.icon className="size-5" />
                  </span>
                  <span>{t(route.key)}</span>
                </Link>
              ))}
            </nav>
          </div>
        </HeaderActionsProvider>
      </AndroidImports>
    </Suspense>
  )
}
