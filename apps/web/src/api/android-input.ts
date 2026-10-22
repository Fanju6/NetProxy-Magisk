import { z } from 'zod'
import { ApiError } from './contracts'

export type DocumentKind = 'config' | 'nodes'
export type ImportKind = DocumentKind | 'subscription'
export interface AndroidImportPayload {
  id: string
  name: string
  content: string
}
export interface AndroidInputAdapter {
  pickDocument(kind: DocumentKind): Promise<AndroidImportPayload | null>
  takeImport(): Promise<AndroidImportPayload | null>
  completeImport(id: string): Promise<void>
  listen(notify: () => void, visibility: (visible: boolean) => void): Promise<() => void>
}
export interface AndroidInputIpc {
  invoke(command: string, args: Record<string, unknown>): Promise<unknown>
  listen(
    event: 'import',
    callback: (event: unknown) => void,
  ): Promise<{
    unregister(): Promise<void>
  }>
}

const importIdSchema = z
  .string()
  .min(1)
  .max(256)
  .regex(/^[A-Za-z0-9_-]+$/)
const payloadSchema = z.strictObject({
  id: importIdSchema,
  name: z
    .string()
    .max(256)
    .refine(
      (name) =>
        !/[/\\:]/.test(name) &&
        [...name].every(
          (character) => character.charCodeAt(0) >= 32 && character.charCodeAt(0) !== 127,
        ),
    ),
  content: z.string(),
})
const safeCodes = [
  'api.import_unreadable',
  'api.import_invalid',
  'api.body_too_large',
  'api.busy',
] as const
const inputError = (code = 'api.import_invalid') =>
  new ApiError(code, 'Android input unavailable or invalid', 0)

export function androidImportPayload(value: unknown): AndroidImportPayload | null {
  if (value === null) return null
  const result = payloadSchema.safeParse(value)
  if (!result.success) throw inputError()
  const bytes = new TextEncoder().encode(result.data.content)
  if (bytes.length > 1024 * 1024) throw inputError('api.body_too_large')
  if (new TextDecoder('utf-8', { fatal: true }).decode(bytes) !== result.data.content)
    throw inputError()
  return result.data
}

export function createAndroidInputAdapter(ipc: AndroidInputIpc): AndroidInputAdapter {
  async function read(command: string, args: Record<string, unknown>) {
    try {
      return androidImportPayload(await ipc.invoke(command, args))
    } catch (failure) {
      // Native failures may contain provider URIs, paths or user content.
      if (failure instanceof ApiError && safeCodes.some((code) => code === failure.code))
        throw inputError(failure.code)
      const parsed = z.object({ code: z.enum(safeCodes) }).safeParse(failure)
      if (parsed.success) throw inputError(parsed.data.code)
      throw inputError()
    }
  }
  return {
    pickDocument: (kind) => {
      if (kind !== 'config' && kind !== 'nodes') return Promise.reject(inputError())
      return read('plugin:module-api|pick_document', { kind })
    },
    takeImport: () => read('plugin:module-api|take_import', {}),
    async completeImport(id) {
      if (!importIdSchema.safeParse(id).success) throw inputError()
      const result = await read('plugin:module-api|take_import', { completedId: id })
      if (result !== null) throw inputError()
    },
    async listen(notify, visibility) {
      const handles: { unregister(): Promise<void> }[] = []
      let closed = false
      let revision = 0
      const refreshVisibility = async (wake: boolean) => {
        const current = ++revision
        try {
          const value = await ipc.invoke('plugin:module-api|visibility', {})
          if (closed || current !== revision) return
          visibility(z.object({ visible: z.boolean() }).parse(value).visible)
          if (wake) notify()
        } catch {
          if (!closed && current === revision) visibility(false)
        }
      }
      const release = () => {
        closed = true
        for (const handle of handles.splice(0)) {
          try {
            void handle.unregister().catch(() => {})
          } catch {
            /* The document may already be gone. */
          }
        }
      }
      try {
        handles.push(
          await ipc.listen('import', () => {
            if (!closed) void refreshVisibility(true)
          }),
        )
        // ModuleEvents owns the one native visibility listener. Import wakes also signal resume.
        await refreshVisibility(false)
        return release
      } catch {
        release()
        throw inputError()
      }
    },
  }
}

export async function loadAndroidInput(): Promise<AndroidInputAdapter | null> {
  if (import.meta.env.MODE !== 'native') return null
  const { invoke, addPluginListener, isTauri } = await import('@tauri-apps/api/core')
  if (!isTauri()) return null
  return createAndroidInputAdapter({
    invoke,
    listen: (event, callback) => addPluginListener('module-api', event, callback),
  })
}

// Classification is only a routing hint. Go remains the parser and validator.
export function detectImportKind(content: string): ImportKind | null {
  const text = content.trim()
  try {
    const url = new URL(text)
    if (!/\s/.test(text) && ['http:', 'https:'].includes(url.protocol)) return 'subscription'
  } catch {
    /* Not a subscription URL. */
  }
  if (/^(?:ss|ssr|vmess|vless|trojan|hysteria2?|hy2|tuic|socks5?|https?|anytls):\/\//im.test(text))
    return 'nodes'
  if (/^proxies\s*:/m.test(text)) return 'nodes'
  try {
    const value: unknown = JSON.parse(text)
    if (typeof value !== 'object' || value === null || Array.isArray(value)) return null
    const keys = Object.keys(value)
    if (
      keys.some((key) =>
        ['log', 'dns', 'route', 'inbounds', 'experimental', 'services'].includes(key),
      )
    )
      return 'config'
    if (keys.includes('outbounds') || keys.includes('endpoints')) return null
    if (keys.includes('type') || keys.includes('servers')) return 'nodes'
  } catch {
    /* Ambiguous input is chosen by the user. */
  }
  return null
}
