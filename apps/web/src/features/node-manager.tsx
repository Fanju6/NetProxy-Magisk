import { useEffect, useState } from 'react'
import { z } from 'zod'
import { Plus } from 'lucide-react'
import { useApi, usePreferences } from '@/app/context'
import { useManagementAction } from '@/api/management'
import type { NodeSnapshot } from '@/api/contracts'
import { Details } from '@/components/details'
import { Choice } from '@/components/choice'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { ErrorState } from '@/components/states'
import { JsonEditor } from '@/components/json-editor'

export function NodeManager({ snapshot }: { snapshot: NodeSnapshot }) {
  const api = useApi()
  const { preferences } = usePreferences()
  const text = (zh: string, en: string) => (preferences.locale === 'zh' ? zh : en)
  const action = useManagementAction()
  const [open, setOpen] = useState(false)
  const [operation, setOperation] = useState('add')
  const groups = snapshot.groups.filter(
    (item) => operation === 'export' || item.group.type === 'local',
  )
  const [groupId, setGroupId] = useState(groups[0]?.group.id ?? 'default')
  const group = groups.find((item) => item.group.id === groupId)
  const [tag, setTag] = useState('')
  const [content, setContent] = useState('')
  const [error, setError] = useState<unknown>(null)
  const [notice, setNotice] = useState('')
  const [insecure, setInsecure] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const requiresTarget = ['edit', 'export', 'remove'].includes(operation)
  const reset = () => {
    setTag('')
    setContent('')
    setDeleting(false)
    setError(null)
    setNotice('')
  }
  useEffect(() => {
    if (!open || !tag || (operation !== 'edit' && operation !== 'export')) return
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
        if (!controller.signal.aborted)
          setContent(
            operation === 'export'
              ? z.object({ link: z.string() }).parse(value).link
              : JSON.stringify(value, null, 2),
          )
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted) setError(failure)
      })
    return () => controller.abort()
  }, [api, groupId, tag, operation, open])
  const close = () => {
    setOpen(false)
    setContent('')
    setTag('')
    setError(null)
    setNotice('')
    setDeleting(false)
    setInsecure(false)
  }
  const save = async () => {
    setError(null)
    setNotice('')
    if (operation === 'remove' && !deleting) {
      setDeleting(true)
      return
    }
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
      setTag('')
      setDeleting(false)
      setNotice(text('操作完成，已重新读取节点。', 'Operation completed. Nodes refreshed.'))
    } catch (failure) {
      setError(failure)
    }
  }
  if (!api.supports('nodes.add')) return null
  return (
    <>
      <Button
        variant="outline"
        onClick={() => {
          setOperation('add')
          setOpen(true)
        }}
      >
        <Plus />
        {text('管理本地节点', 'Manage local nodes')}
      </Button>
      <Details
        open={open}
        onOpenChange={(value) => {
          if (!value) close()
        }}
        title={text('本地节点', 'Local nodes')}
        description={text(
          '仅修改本地分组。打开原始配置或分享链接时，其中的凭据会进入当前页面内存。',
          'Only local groups are editable. Opening raw configuration or a share link exposes its credentials to this trusted page memory.',
        )}
      >
        <div className="space-y-4">
          <Choice
            label={text('操作', 'Operation')}
            value={operation}
            items={[
              { value: 'add', label: text('添加节点', 'Add node') },
              { value: 'edit', label: text('编辑节点', 'Edit node') },
              { value: 'import', label: text('导入文本', 'Import text') },
              { value: 'export', label: text('分享链接', 'Share link') },
              { value: 'remove', label: text('删除节点', 'Delete node') },
            ]}
            onChange={(value) => {
              reset()
              setOperation(value)
            }}
          />
          {operation !== 'import' && (
            <Choice
              label={text('本地分组', 'Local group')}
              value={groupId}
              items={groups.map((item) => ({ value: item.group.id, label: item.group.name }))}
              onChange={(value) => {
                reset()
                setGroupId(value)
              }}
            />
          )}
          {requiresTarget && (
            <Choice
              label={text('节点', 'Node')}
              value={tag}
              items={[
                { value: '', label: text('选择节点', 'Select a node') },
                ...(group?.nodes.map((node) => ({ value: node.tag, label: node.tag })) ?? []),
              ]}
              onChange={(value) => {
                reset()
                setTag(value)
              }}
            />
          )}
          {operation !== 'remove' && (
            <JsonEditor
              value={content}
              onChange={setContent}
              readOnly={operation === 'export'}
              label={text('节点内容', 'Node content')}
            />
          )}
          {['add', 'edit', 'import'].includes(operation) && (
            <label className="setting-row">
              <span>{text('允许不安全 TLS', 'Allow insecure TLS')}</span>
              <Switch checked={insecure} onCheckedChange={setInsecure} />
            </label>
          )}
          {deleting && (
            <p role="alert" className="text-sm text-destructive">
              {text('再次确认将永久删除此节点。', 'Confirm again to permanently delete this node.')}
            </p>
          )}
          {error != null && <ErrorState compact error={error} />}
          {notice && (
            <p role="status" className="text-sm">
              {notice}
            </p>
          )}
          {operation !== 'export' && (
            <Button
              variant={deleting ? 'destructive' : 'default'}
              disabled={
                action.pending ||
                (requiresTarget && !tag) ||
                (operation !== 'remove' && !content.trim())
              }
              onClick={() => void save()}
            >
              {deleting ? text('确认删除', 'Confirm delete') : text('执行', 'Apply')}
            </Button>
          )}
        </div>
      </Details>
    </>
  )
}
