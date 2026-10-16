import { Link, Outlet, useLocation } from '@tanstack/react-router'
import {
  LayoutDashboard,
  Network,
  Rss,
  Settings,
  Moon,
  Sun,
  Languages,
  FlaskConical,
  ArrowUpRight,
} from 'lucide-react'
import { useApi, usePreferences } from './context'
import { ModuleEvents } from '@/api/events'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

const routes = [
  { to: '/', key: 'dashboard', description: 'overviewDescription', icon: LayoutDashboard },
  { to: '/nodes', key: 'nodes', description: 'nodesDescription', icon: Network },
  {
    to: '/subscriptions',
    key: 'subscriptions',
    description: 'subscriptionsDescription',
    icon: Rss,
  },
  { to: '/settings', key: 'settings', description: 'settingsDescription', icon: Settings },
] as const

export function AppShell() {
  const { t, preferences, update } = usePreferences()
  const api = useApi()
  const pathname = useLocation({ select: (location) => location.pathname })
  const current = routes.find((route) => route.to === pathname) ?? routes[0]
  return (
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
          <img src={`${import.meta.env.BASE_URL}netproxy.svg`} width="28" height="28" alt="" />
          <span className="font-semibold">NetProxy</span>
          <span className="ml-auto text-xs text-muted-foreground">V9</span>
        </Link>
        <nav aria-label={t('mainNavigation')} className="mt-10 space-y-1">
          {routes.map((route) => (
            <Link
              key={route.to}
              to={route.to}
              aria-current={pathname === route.to ? 'page' : undefined}
              className={cn('nav-link', pathname === route.to && 'bg-accent text-foreground')}
            >
              <route.icon className="size-4" />
              {t(route.key)}
            </Link>
          ))}
        </nav>
        <div className="mt-auto space-y-5 px-3 pb-2">
          <p className="max-w-36 text-sm leading-6 text-muted-foreground">{t('intro')}</p>
          <a
            href="https://github.com/Fanju6/NetProxy-Magisk"
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-2 text-xs text-muted-foreground"
          >
            GitHub <ArrowUpRight className="size-3" />
          </a>
        </div>
      </aside>
      <div className="min-w-0 flex-1">
        <header className="app-header">
          <div className="flex items-center gap-2 text-sm">
            <span className="font-medium md:font-normal md:text-muted-foreground">NetProxy</span>
            <span className="text-muted-foreground/40">/</span>
            <span>{t(current.key)}</span>
          </div>
          <div className="flex items-center gap-1">
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={t('theme')}
                    onClick={() =>
                      update({
                        theme: document.documentElement.classList.contains('dark')
                          ? 'light'
                          : 'dark',
                      })
                    }
                  />
                }
              >
                {preferences.theme === 'dark' ? (
                  <Sun className="size-4" />
                ) : (
                  <Moon className="size-4" />
                )}
              </TooltipTrigger>
              <TooltipContent className="hidden md:block">{t('theme')}</TooltipContent>
            </Tooltip>
            <Button
              variant="ghost"
              size="icon"
              aria-label={t('language')}
              onClick={() => update({ locale: preferences.locale === 'zh' ? 'en' : 'zh' })}
            >
              <Languages className="size-4" />
            </Button>
          </div>
        </header>
        {api.transport.kind === 'mock' && (
          <div className="demo-notice" role="note">
            <FlaskConical className="size-3.5 shrink-0" />
            <strong className="font-medium">{t('demo')}</strong>
            <span className="text-muted-foreground">{t('demoNotice')}</span>
          </div>
        )}
        {api.transport.kind === 'native' && (
          <div className="demo-notice" role="note">
            <strong className="font-medium">{t('nativeReadOnly')}</strong>
            <span className="text-muted-foreground">{t('readOnlyNotice')}</span>
          </div>
        )}
        {api.transport.kind !== 'unavailable' && <ModuleEvents />}
        <main id="main" className="page-content" tabIndex={-1}>
          <div className="mb-8 space-y-2">
            <h1 className="text-2xl font-semibold sm:text-3xl">{t(current.key)}</h1>
            <p className="text-sm text-muted-foreground">{t(current.description)}</p>
          </div>
          <Outlet />
        </main>
      </div>
      <nav aria-label={t('mainNavigation')} className="mobile-navigation">
        {routes.map((route) => (
          <Link
            key={route.to}
            to={route.to}
            aria-current={pathname === route.to ? 'page' : undefined}
            className={cn(
              'flex min-h-14 flex-1 flex-col items-center justify-center gap-1 text-[11px] text-muted-foreground',
              pathname === route.to && 'text-foreground',
            )}
          >
            <route.icon className="size-5" />
            {t(route.key)}
          </Link>
        ))}
      </nav>
    </div>
  )
}
