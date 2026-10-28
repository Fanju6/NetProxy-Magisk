import { Link } from '@tanstack/react-router'
import { AppWindow, ChevronRight } from 'lucide-react'
import { z } from 'zod'
import { backendDocumentSchema, parseDocument, rootDocumentSchema } from '@/api/management-data'
import { Choice } from '@/components/choice'
import { EmptyState } from '@/components/states'
import { NativeInboundForm } from './native-inbound-form'
import { useManagementCopy } from './management-copy'

const inboundFormSchema = z.looseObject({
  backend: backendDocumentSchema.shape.backend,
  root_policy: rootDocumentSchema.shape.root_policy,
  ebpf: z.record(z.string(), z.unknown()),
  tun: z.record(z.string(), z.unknown()),
})

export function InboundSettingsForm({
  content,
  change,
}: {
  content: string
  change: (content: string) => void
}) {
  const copy = useManagementCopy()
  const document = parseDocument(content, inboundFormSchema)
  if (!document) return <EmptyState title={copy.invalid} />
  const patch = (values: Partial<Pick<typeof document, 'backend' | 'root_policy'>>) =>
    change(JSON.stringify({ ...document, ...values }, null, 2))
  return (
    <div className="space-y-5">
      <div className="settings-group">
        <div className="setting-row">
          <span className="min-w-0 break-words">{copy.backend}</span>
          <Choice
            variant="ghost"
            label={copy.backend}
            value={document.backend}
            items={(['ebpf', 'tun'] as const).map((value) => ({ value, label: copy[value] }))}
            onChange={(backend) => {
              if (backend === 'ebpf' || backend === 'tun') patch({ backend })
            }}
          />
        </div>
        <div className="setting-row">
          <span className="min-w-0 break-words">{copy.root}</span>
          <Choice
            variant="ghost"
            label={copy.root}
            value={document.root_policy}
            items={(['default', 'include', 'exclude'] as const).map((value) => ({
              value,
              label: copy[value],
            }))}
            onChange={(root_policy) => {
              if (
                root_policy === 'default' ||
                root_policy === 'include' ||
                root_policy === 'exclude'
              )
                patch({ root_policy })
            }}
          />
        </div>
        <Link to="/settings/policies" search={{ section: 'apps' }} className="setting-row">
          <span className="flex min-w-0 items-center gap-3">
            <AppWindow className="size-4 shrink-0 text-muted-foreground" />
            {copy.apps}
          </span>
          <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
        </Link>
      </div>
      <NativeInboundForm
        key={document.backend}
        backend={document.backend}
        content={content}
        change={change}
      />
    </div>
  )
}
