import { useEffect, useState } from 'react'
import { z } from 'zod'
import { Copy } from 'lucide-react'
import { useApi, usePreferences } from '@/app/context'
import { useManagementAction } from '@/api/management'
import type { NodeSnapshot } from '@/api/contracts'
import { Details, EditorPanel } from '@/components/details'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { ErrorState, LoadingState } from '@/components/states'
import { JsonEditor } from '@/components/json-editor'

export type NodeManagerTarget =
  | { operation: 'add' | 'import' }
  | { operation: 'edit' | 'export' | 'remove'; groupId: string; tag: string }

export function NodeManager({
  snapshot,
  target,
  onClose,
}: {
  snapshot: NodeSnapshot
  target: NodeManagerTarget
  onClose: () => void
}) {
  const api = useApi()
  const { preferences } = usePreferences()
  const text = (zh: string, en: string) => (preferences.locale === 'zh' ? zh : en)
  const action = useManagementAction()
  const { operation } = target
  const groups = snapshot.groups.filter((item) => item.group.type === 'local')
  const [localGroupId, setLocalGroupId] = useState(groups[0]?.group.id ?? 'default')
  const groupId = 'groupId' in target ? target.groupId : localGroupId
  const tag = 'tag' in target ? target.tag : ''
  const group = snapshot.groups.find((item) => item.group.id === groupId)
  const [content, setContent] = useState('')
  const [original, setOriginal] = useState('')
  const [error, setError] = useState<unknown>(null)
  const [notice, setNotice] = useState('')
  const [insecure, setInsecure] = useState(false)
  const [loaded, setLoaded] = useState(operation !== 'edit' && operation !== 'export')
  const [copyState, setCopyState] = useState<'idle' | 'copying' | 'copied' | 'failed'>('idle')
  const requiresTarget = 'tag' in target
  const validTarget =
    operation === 'import' ||
    (!!group &&
      (operation === 'export' || group.group.type === 'local') &&
      (!requiresTarget || group.nodes.some((node) => node.tag === tag)))
  useEffect(() => {
    if (operation !== 'edit' && operation !== 'export') return
    const controller = new AbortController()
    void api
      .request(
        {
          method: 'GET',
          path: operation === 'edit' ? '/nodes/get' : '/nodes/export',
          query: { group_id: groupId, node_tag: tag },
        },
        z.unknown(),
        controller.signal,
      )
      .then((value) => {
        if (controller.signal.aborted) return
        const result =
          operation === 'export'
            ? z.object({ link: z.string() }).parse(value).link
            : JSON.stringify(value, null, 2)
        setContent(result)
        setOriginal(result)
        setLoaded(true)
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted) setError(failure)
      })
    return () => controller.abort()
  }, [api, groupId, tag, operation])
  const save = async () => {
    if (action.pending || !validTarget || !loaded || notice) return
    setError(null)
    try {
      const request =
        operation === 'remove'
          ? {
              method: 'DELETE' as const,
              path: '/nodes' as const,
              query: { group_id: groupId, node_tag: tag },
            }
          : {
              method: operation === 'edit' ? ('PUT' as const) : ('POST' as const),
              path: operation === 'import' ? ('/nodes/import' as const) : ('/nodes' as const),
              body: {
                ...(operation === 'import' ? {} : { group_id: groupId }),
                ...(operation === 'edit' ? { node_tag: tag } : {}),
                content,
                allow_insecure: insecure,
              },
            }
      await action.run(request, z.record(z.string(), z.unknown()))
      setContent('')
      setOriginal('')
      setInsecure(false)
      setNotice(text('操作完成，已重新读取节点。', 'Operation completed. Nodes refreshed.'))
    } catch (failure) {
      setError(failure)
    }
  }
  if (!api.supports(operation === 'export' ? 'nodes.export' : `nodes.${operation}`)) return null
  const title = {
    add: text('手动添加节点', 'Add node manually'),
    import: text('导入文本', 'Import text'),
    edit: text('编辑节点', 'Edit node'),
    export: text('分享链接', 'Share link'),
    remove: text('删除节点', 'Delete node'),
  }[operation]
  const targetChanged = !validTarget && !notice && (
    <p role="alert" className="text-sm text-destructive">
      {text(
        '节点或本地分组已变更，请关闭后重新选择。',
        'The node or local group changed. Close and select it again.',
      )}
    </p>
  )
  if (operation === 'remove') {
    return (
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open) onClose()
        }}
      >
        <DialogContent className="rounded-lg">
          <DialogHeader className="pr-8">
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>
              {text('删除后无法恢复。', 'This cannot be undone.')}
            </DialogDescription>
          </DialogHeader>
          <div className="min-w-0 space-y-3">
            <p className="break-all font-medium">{tag}</p>
            <p className="break-all text-sm text-muted-foreground">{group?.group.name}</p>
            {targetChanged}
            {error != null && <ErrorState compact error={error} />}
            {notice && (
              <p role="status" className="text-sm">
                {notice}
              </p>
            )}
          </div>
          {!notice && (
            <DialogFooter>
              <Button variant="outline" onClick={onClose}>
                {text('取消', 'Cancel')}
              </Button>
              <Button
                variant="destructive"
                disabled={action.pending || !validTarget || !loaded}
                onClick={() => void save()}
              >
                {text('确认删除', 'Confirm delete')}
              </Button>
            </DialogFooter>
          )}
        </DialogContent>
      </Dialog>
    )
  }
  const raw = operation === 'add' || operation === 'edit'
  const Panel = raw ? EditorPanel : Details
  return (
    <Panel
      open
      dirty={!notice && operation !== 'export' && (content !== original || insecure)}
      onOpenChange={(value) => {
        if (!value) onClose()
      }}
      title={title}
      description={text(
        '节点原文和分享链接可能包含凭据，仅在当前页面内存中保留。',
        'Node content and share links may contain credentials and stay only in this page memory.',
      )}
      footer={
        operation === 'export' ? (
          <Button
            variant="outline"
            disabled={!loaded || !content || copyState === 'copying'}
            onClick={() => {
              setCopyState('copying')
              void (async () => {
                try {
                  await navigator.clipboard.writeText(content)
                  setCopyState('copied')
                } catch {
                  setCopyState('failed')
                }
              })()
            }}
          >
            <Copy />
            {text('复制链接', 'Copy link')}
          </Button>
        ) : !notice ? (
          <Button
            disabled={action.pending || !validTarget || !loaded || !content.trim()}
            onClick={() => void save()}
          >
            {text('保存', 'Save')}
          </Button>
        ) : undefined
      }
    >
      <div className={raw ? 'flex min-h-0 flex-1 flex-col gap-3' : 'min-w-0 space-y-4'}>
        {requiresTarget && (
          <div className="min-w-0 space-y-1">
            <p className="break-all font-medium">{tag}</p>
            <p className="break-all text-sm text-muted-foreground">{group?.group.name}</p>
          </div>
        )}
        {targetChanged}
        {operation === 'add' && groups.length > 1 && (
          <Select
            value={localGroupId}
            onValueChange={(value) => value && setLocalGroupId(value)}
            disabled={action.pending}
          >
            <SelectTrigger className="min-h-12 w-full" aria-label={text('本地分组', 'Local group')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {groups.map((item) => (
                <SelectItem key={item.group.id} value={item.group.id} className="min-h-12">
                  {item.group.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
        {!loaded && error == null && <LoadingState />}
        {loaded &&
          !notice &&
          (operation === 'add' || operation === 'edit' ? (
            <JsonEditor
              value={content}
              onChange={setContent}
              readOnly={action.pending}
              label={text('节点内容', 'Node content')}
            />
          ) : (
            <label className="block space-y-2 text-sm">
              <span>{text('节点内容', 'Node content')}</span>
              <textarea
                value={content}
                onChange={(event) => setContent(event.target.value)}
                readOnly={operation === 'export' || action.pending}
                autoComplete="off"
                autoCapitalize="none"
                spellCheck={false}
                rows={8}
                className="w-full resize-y rounded-md border bg-transparent p-3 font-mono text-base"
              />
            </label>
          ))}
        {['add', 'edit', 'import'].includes(operation) && !notice && (
          <label className="flex min-h-12 items-center justify-between gap-4 text-sm">
            <span>{text('允许不安全 TLS', 'Allow insecure TLS')}</span>
            <Switch checked={insecure} onCheckedChange={setInsecure} disabled={action.pending} />
          </label>
        )}
        {copyState === 'copied' && (
          <p role="status" className="text-sm">
            {text('链接已复制', 'Link copied')}
          </p>
        )}
        {copyState === 'failed' && (
          <p role="alert" className="text-sm text-destructive">
            {text(
              '复制失败，请手动选择链接复制。',
              'Copy failed. Select the link and copy it manually.',
            )}
          </p>
        )}
        {error != null && <ErrorState compact error={error} />}
        {notice && (
          <p role="status" className="text-sm">
            {notice}
          </p>
        )}
      </div>
    </Panel>
  )
}
