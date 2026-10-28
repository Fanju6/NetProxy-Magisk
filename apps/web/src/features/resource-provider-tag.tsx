import { useState } from 'react'
import { Copy } from 'lucide-react'
import type { GroupSummary } from '@/api/contracts'
import { usePreferences } from '@/app/context'
import { Button } from '@/components/ui/button'
import { useResultToast } from '@/components/ui/toast'

export function ResourceProviderTag({ group }: { group: Pick<GroupSummary, 'provider_tag'> }) {
  const { t, preferences } = usePreferences()
  const [copy, setCopy] = useState<'idle' | 'pending' | 'done' | 'failed'>('idle')
  useResultToast(
    'provider-copy',
    copy === 'done' || copy === 'failed' ? copy : null,
    copy === 'done'
      ? preferences.locale === 'zh'
        ? 'Provider Tag 已复制'
        : 'Provider Tag copied'
      : preferences.locale === 'zh'
        ? '复制失败，请手动选择 Provider Tag 复制。'
        : 'Copy failed. Select the Provider Tag and copy it manually.',
  )
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
    </div>
  )
}
