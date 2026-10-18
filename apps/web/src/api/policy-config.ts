import { z } from 'zod'
import { jsonLanguage } from '@codemirror/lang-json'

export const outboundObjectSchema = z.looseObject({
  tag: z.string().min(1),
  type: z.string().min(1),
})
export const outboundsDocumentSchema = z.looseObject({
  outbounds: z.array(z.looseObject({ tag: z.string().optional(), type: z.string() })).optional(),
})
export type NativeOutbound = z.infer<typeof outboundObjectSchema>
type Node = ReturnType<typeof jsonLanguage.parser.parse>['topNode']

function rootObject(content: string): Node {
  const tree = jsonLanguage.parser.parse(content)
  let invalid = false
  tree.iterate({
    enter: (node) => {
      if (node.type.isError) invalid = true
    },
  })
  const root = tree.topNode.firstChild
  if (invalid || root?.name !== 'Object' || root.nextSibling) throw new Error('Invalid JSON object')
  return root
}
function properties(content: string, object: Node) {
  return object.getChildren('Property').map((property) => {
    const key = property.getChild('PropertyName')!
    return {
      key: JSON.parse(content.slice(key.from, key.to)) as string,
      value: property.lastChild!,
      property,
    }
  })
}
function property(content: string, object: Node, key: string) {
  const matches = properties(content, object).filter((item) => item.key === key)
  if (matches.length > 1) throw new Error('Ambiguous JSON property')
  return matches[0]
}
function identity(content: string, object: Node) {
  const text = (key: string) => {
    const node = property(content, object, key)?.value
    return node?.name === 'String'
      ? (JSON.parse(content.slice(node.from, node.to)) as string)
      : undefined
  }
  return { tag: text('tag'), type: text('type') }
}
function documentArray(content: string) {
  const root = rootObject(content)
  const array = property(content, root, 'outbounds')?.value
  if (array && array.name !== 'Array') throw new Error('Invalid outbounds array')
  const entries: Node[] = []
  if (array) {
    for (let child = array.firstChild; child; child = child.nextSibling) {
      if (['[', ']', ','].includes(child.name)) continue
      if (child.name !== 'Object') throw new Error('Invalid outbound object')
      entries.push(child)
    }
  }
  return { root, array, entries }
}
function replaceRange(content: string, node: Node, replacement: string | null) {
  if (replacement !== null)
    return content.slice(0, node.from) + replacement + content.slice(node.to)
  const after = node.nextSibling
  const before = node.prevSibling
  const from = after?.name === ',' ? node.from : before?.name === ',' ? before.from : node.from
  const to = after?.name === ',' ? after.to : node.to
  return content.slice(0, from) + content.slice(to)
}
export function readPolicyObject(content: string, tag: string) {
  const matches = documentArray(content).entries.filter(
    (entry) => identity(content, entry).tag === tag,
  )
  if (matches.length !== 1) throw new Error('Policy object no longer exists or is ambiguous')
  return content.slice(matches[0]!.from, matches[0]!.to)
}

export function readOutboundsPartition(content: string) {
  const { array } = documentArray(content)
  return array ? '{"outbounds":' + content.slice(array.from, array.to) + '}' : '{}'
}

export function applyOutboundsPartition(content: string, partition: string) {
  const { array } = documentArray(partition)
  if (!array) return setPolicyProperty(content, 'outbounds', undefined)
  const root = rootObject(content)
  const current = property(content, root, 'outbounds')
  const raw = partition.slice(array.from, array.to)
  if (current) return replaceRange(content, current.value, raw)
  const insertion = (properties(content, root).length ? ',' : '') + '\n"outbounds":' + raw
  return content.slice(0, root.to - 1) + insertion + content.slice(root.to - 1)
}

// Only changed values are serialized. Unknown fields, numeric literals and unrelated objects stay raw.
export function setPolicyProperty(content: string, key: string, value: unknown) {
  const root = rootObject(content)
  const existing = property(content, root, key)
  if (value === undefined)
    return existing ? replaceRange(content, existing.property, null) : content
  const raw = JSON.stringify(value)
  if (existing) return replaceRange(content, existing.value, raw)
  const insertion =
    (properties(content, root).length ? ',' : '') + '\n' + JSON.stringify(key) + ': ' + raw
  return content.slice(0, root.to - 1) + insertion + content.slice(root.to - 1)
}
export function replacePolicyObject(
  content: string,
  originalTag: string | null,
  replacement: NativeOutbound | string | null,
) {
  const { root, array, entries } = documentArray(content)
  const raw =
    typeof replacement === 'string'
      ? replacement
      : replacement === null
        ? null
        : JSON.stringify(replacement, null, 2)
  let nextTag: string | undefined
  if (raw !== null) {
    const next = identity(raw, rootObject(raw))
    if (!next.tag || !next.type) throw new Error('Outbound tag and type are required')
    nextTag = next.tag
    if (
      entries.some((entry) => {
        const tag = identity(content, entry).tag
        return tag === nextTag && tag !== originalTag
      })
    )
      throw new Error('Duplicate outbound tag')
  }
  if (originalTag !== null) {
    const matches = entries.filter((entry) => identity(content, entry).tag === originalTag)
    if (matches.length !== 1) throw new Error('Policy object no longer exists or is ambiguous')
    return replaceRange(content, matches[0]!, raw)
  }
  if (raw === null) throw new Error('Missing new outbound')
  if (!array) {
    const insertion = (properties(content, root).length ? ',' : '') + '\n"outbounds": [' + raw + ']'
    return content.slice(0, root.to - 1) + insertion + content.slice(root.to - 1)
  }
  return (
    content.slice(0, array.to - 1) +
    (entries.length ? ',' : '') +
    '\n' +
    raw +
    content.slice(array.to - 1)
  )
}
