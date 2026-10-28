import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { Link } from '@tanstack/react-router'
import {
  AppWindow,
  ArrowUpRight,
  ChevronRight,
  Code2,
  FileText,
  Network,
  Route,
  Wifi,
} from 'lucide-react'
import { useApi, usePreferences } from '@/app/context'
import { moduleQueries } from '@/api/queries'
import { Choice } from '@/components/choice'
import { ErrorState } from '@/components/states'
import { Switch } from '@/components/ui/switch'
import { Skeleton } from '@/components/ui/skeleton'
import { useManagementAction } from '@/api/management'

export function SettingsPage() {
  const api = useApi()
  const cache = useQueryClient()
  const { preferences, update, t } = usePreferences()
  const management = useManagementAction()
  const info = useQuery({ ...moduleQueries(api).info, enabled: api.supports('info.read') })
  const text = (zh: string, en: string) => (preferences.locale === 'zh' ? zh : en)
  const config = useQuery({
    ...moduleQueries(api).autoStart,
    select: (document) => ({
      ...document,
      value: z.object({ auto_start: z.boolean() }).parse(JSON.parse(document.content)).auto_start,
    }),
  })
  const save = useMutation({
    mutationFn: (value: boolean) => {
      if (!config.data) throw new Error('Configuration is not loaded')
      return management.run(
        {
          method: 'PUT',
          path: '/config',
          query: { target: 'module/auto_start' },
          body: { revision: config.data.revision, content: JSON.stringify({ auto_start: value }) },
        },
        z.object({ target: z.string(), revision: z.string() }),
      )
    },
    onSettled: async () => {
      await cache.invalidateQueries({ queryKey: ['config', 'module/auto_start'] })
    },
  })
  const policies = [
    {
      title: t('inbound'),
      icon: Network,
      section: 'inbound' as const,
    },
    {
      title: text('应用代理', 'Application proxy'),
      icon: AppWindow,
      section: 'apps' as const,
    },
    {
      title: 'Wi-Fi',
      icon: Wifi,
      section: 'wifi' as const,
    },
  ]
  const advanced = [
    {
      title: t('routes'),
      icon: Route,
      section: 'rules' as const,
    },
    {
      title: t('kernel'),
      icon: Code2,
      section: 'core' as const,
    },
  ]
  return (
    <div className="max-w-3xl space-y-5">
      <section aria-labelledby="general-title">
        <h2 id="general-title" className="section-label">
          {t('general')}
        </h2>
        <div className="settings-group">
          {!api.supports('config.read') ? (
            <p className="px-4 py-4 text-sm text-muted-foreground">{t('readOnlyNotice')}</p>
          ) : config.isError ? (
            <ErrorState compact error={config.error} retry={() => void config.refetch()} />
          ) : (
            <div className="setting-row">
              <label htmlFor="auto-start">{t('autoStart')}</label>
              {config.isPending ? (
                <Skeleton className="h-5 w-8" />
              ) : (
                <Switch
                  id="auto-start"
                  checked={config.data.value}
                  disabled={!api.supports('config.write') || save.isPending || management.pending}
                  onCheckedChange={(value) => save.mutate(value)}
                />
              )}
            </div>
          )}
          {save.isError && <ErrorState compact error={save.error} />}
          <div className="setting-row">
            <p>{t('theme')}</p>
            <Choice
              variant="ghost"
              label={t('theme')}
              value={preferences.theme}
              items={(['system', 'light', 'dark'] as const).map((value) => ({
                value,
                label: t(value),
              }))}
              onChange={(value) => {
                if (value === 'system' || value === 'light' || value === 'dark')
                  update({ theme: value })
              }}
            />
          </div>
          <div className="setting-row">
            <p>{t('language')}</p>
            <Choice
              variant="ghost"
              label={t('language')}
              value={preferences.locale}
              items={[
                { value: 'zh', label: t('chinese') },
                { value: 'en', label: t('english') },
              ]}
              onChange={(value) => {
                if (value === 'zh' || value === 'en') update({ locale: value })
              }}
            />
          </div>
        </div>
      </section>
      <section aria-labelledby="policies-title">
        <h2 id="policies-title" className="section-label">
          {text('代理与网络', 'Proxy & network')}
        </h2>
        <div className="settings-group">
          {policies.map((item) => (
            <Link
              to="/settings/policies"
              search={{ section: item.section }}
              key={item.section}
              className="setting-row"
            >
              <span className="flex min-w-0 items-center gap-3">
                <item.icon className="size-4 shrink-0 text-muted-foreground" />
                <span className="break-words">{item.title}</span>
              </span>
              <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
            </Link>
          ))}
        </div>
      </section>
      <section aria-labelledby="advanced-title">
        <h2 id="advanced-title" className="section-label">
          {text('高级', 'Advanced')}
        </h2>
        <div className="settings-group">
          {advanced.map((item) => (
            <Link
              to="/settings/configuration"
              search={{ section: item.section }}
              key={item.section}
              className="setting-row"
            >
              <span className="flex min-w-0 items-center gap-3">
                <item.icon className="size-4 shrink-0 text-muted-foreground" />
                <span className="break-words">{item.title}</span>
              </span>
              <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
            </Link>
          ))}
          <Link to="/settings/logs" className="setting-row">
            <span className="flex min-w-0 items-center gap-3">
              <FileText className="size-4 shrink-0 text-muted-foreground" />
              {t('logs')}
            </span>
            <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
          </Link>
        </div>
      </section>
      <section aria-labelledby="about-title">
        <h2 id="about-title" className="section-label">
          {t('about')}
        </h2>
        <div className="settings-group">
          <div className="setting-row">
            <p className="font-medium">NETPROXY V9</p>
            {info.data && (
              <p className="min-w-0 break-words text-right text-sm text-muted-foreground">
                {t('version')} {info.data.version} ({info.data.version_code})
              </p>
            )}
          </div>
          {info.isError && (
            <ErrorState compact error={info.error} retry={() => void info.refetch()} />
          )}
          <a
            href="https://github.com/Fanju6/NetProxy-Magisk"
            target="_blank"
            rel="noopener noreferrer"
            className="setting-row"
          >
            GitHub <ArrowUpRight className="size-4 shrink-0 text-muted-foreground" />
          </a>
        </div>
      </section>
    </div>
  )
}
