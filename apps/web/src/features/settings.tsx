import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { z } from 'zod'
import { ArrowUpRight, Code2, FileText, Network, Route, Wifi } from 'lucide-react'
import { useApi, usePreferences } from '@/app/context'
import { moduleQueries } from '@/api/queries'
import { Choice } from '@/components/choice'
import { ErrorState } from '@/components/states'
import { Switch } from '@/components/ui/switch'
import { Skeleton } from '@/components/ui/skeleton'

export function SettingsPage() {
  const api = useApi()
  const cache = useQueryClient()
  const { preferences, update, t } = usePreferences()
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
      return api.request(
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
  const future = [
    { title: t('inbound'), description: t('inboundDescription'), icon: Network },
    { title: t('network'), description: t('networkDescription'), icon: Wifi },
    { title: t('routes'), description: t('routesDescription'), icon: Route },
    { title: t('kernel'), description: t('kernelDescription'), icon: Code2 },
    { title: t('logs'), description: t('logsDescription'), icon: FileText },
  ]
  return (
    <div className="max-w-3xl space-y-9">
      <section aria-labelledby="appearance-title" className="space-y-2">
        <h2 id="appearance-title" className="section-label">
          {t('appearance')}
        </h2>
        <div className="setting-row">
          <p>{t('theme')}</p>
          <Choice
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
      </section>
      <section aria-labelledby="general-title" className="space-y-2">
        <h2 id="general-title" className="section-label">
          {t('general')}
        </h2>
        {api.transport.readOnly ? (
          <p className="py-4 text-sm text-muted-foreground">{t('readOnlyNotice')}</p>
        ) : config.isError ? (
          <ErrorState compact error={config.error} retry={() => void config.refetch()} />
        ) : (
          <div className="setting-row">
            <div className="space-y-1">
              <label htmlFor="auto-start">{t('autoStart')}</label>
              <p className="text-xs text-muted-foreground">{t('autoStartDescription')}</p>
            </div>
            {config.isPending ? (
              <Skeleton className="h-5 w-8" />
            ) : (
              <Switch
                id="auto-start"
                checked={config.data.value}
                disabled={api.transport.readOnly || save.isPending}
                onCheckedChange={(value) => save.mutate(value)}
              />
            )}
          </div>
        )}
        {save.isError && <ErrorState compact error={save.error} />}
      </section>
      <section aria-labelledby="roadmap-title" className="space-y-2">
        <h2 id="roadmap-title" className="section-label">
          {t('roadmap')}
        </h2>
        {future.map((item) => (
          <div key={item.title} className="setting-row">
            <div className="flex min-w-0 gap-3">
              <item.icon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
              <div className="min-w-0 space-y-1">
                <p>{item.title}</p>
                <p className="text-xs text-muted-foreground">{item.description}</p>
              </div>
            </div>
            <span className="max-w-28 text-right text-xs text-muted-foreground">
              {t('nextBatch')}
            </span>
          </div>
        ))}
      </section>
      <section className="space-y-4 border-t pt-6">
        <h2 className="text-sm font-medium">{t('about')}</h2>
        <p className="text-sm text-muted-foreground">
          {t(api.transport.kind === 'native' ? 'nativeReadOnly' : 'aboutDescription')}
        </p>
        <p className="text-xs leading-6 text-muted-foreground">{t('trustNotice')}</p>
        <a
          href="https://github.com/Fanju6/NetProxy-Magisk"
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 text-sm underline underline-offset-4"
        >
          GitHub <ArrowUpRight className="size-3" />
        </a>
      </section>
    </div>
  )
}
