import { useState } from 'react'
import { Copy } from 'lucide-react'
import type { GroupSummary } from '@/api/contracts'
import { usePreferences } from '@/app/context'
import { Button } from '@/components/ui/button'

export function ResourceProviderTag({ group }: { group: Pick<GroupSummary, 'provider_tag'> }) {
  const { t, preferences } = usePreferences()
  const [copy, setCopy] = useState<'idle' | 'pending' | 'done' | 'failed'>('idle')
  const tag = group.provider_tag
  return (
    <div className="min-w-0 space-y-2">
      <div className="flex min-w-0 items-center gap-3">
        <div className="min-w-0 flex-1 space-y-1">
          <p className="text-sm font-medium">ProviderTag</p>
          <p className="select-text break-all font-mono text-sm">{tag || t('notProvided')}</p>
        </div>
        <Button
          variant="outline"
          size="icon"
          className="size-11 shrink-0"
          aria-label={preferences.locale === 'zh' ? '复制 ProviderTag' : 'Copy ProviderTag'}
          disabled={!tag || copy === 'pending'}
          onClick={() => {
            if (!tag) return
            setCopy('pending')
            void (async () => {
              try {
                await navigator.clipboard.writeText(tag)
                setCopy('done')
              } catch {
                setCopy('failed')
              }
            })()
          }}
        >
          <Copy />
        </Button>
      </div>
      {copy === 'done' && (
        <p role="status" className="text-sm text-muted-foreground">
          {preferences.locale === 'zh' ? 'ProviderTag 已复制' : 'ProviderTag copied'}
        </p>
      )}
      {copy === 'failed' && (
        <p role="alert" className="text-sm text-destructive">
          {preferences.locale === 'zh'
            ? '复制失败，请手动选择 ProviderTag 复制。'
            : 'Copy failed. Select the ProviderTag and copy it manually.'}
        </p>
      )}
    </div>
  )
}
