import { useEffect, useRef, type ReactNode } from 'react'
import { Toast } from '@base-ui/react/toast'
import { X } from 'lucide-react'
import { usePreferences } from '@/app/context'
import { Button } from './button'

export function ToastProvider({ children }: { children: ReactNode }) {
  return (
    <Toast.Provider limit={3}>
      {children}
      <ToastList />
    </Toast.Provider>
  )
}

function ToastList() {
  const { toasts } = Toast.useToastManager()
  const { t } = usePreferences()
  return (
    <Toast.Portal>
      <Toast.Viewport
        className="pointer-events-none fixed inset-x-4 bottom-[calc(max(80px,var(--keyboard-inset,0px))+env(safe-area-inset-bottom))] z-[70] mx-auto flex max-w-md flex-col gap-2 outline-none md:inset-x-auto md:bottom-5 md:right-5 md:w-96"
        aria-label={t('notifications')}
      >
        {toasts.map((toast) => (
          <Toast.Root
            data-slot="toast"
            key={toast.id}
            toast={toast}
            className="pointer-events-auto rounded-lg border bg-popover p-3 text-popover-foreground data-limited:hidden"
          >
            <Toast.Content className="flex items-start gap-3">
              <div className="min-w-0 flex-1 space-y-1">
                <Toast.Title className="text-sm font-medium" />
                <Toast.Description className="break-words text-sm text-muted-foreground" />
                {toast.actionProps && (
                  <Toast.Action
                    aria-hidden={false}
                    render={<Button variant="outline" size="sm" />}
                    className="mt-2"
                  />
                )}
              </div>
              <Toast.Close
                aria-hidden={false}
                render={<Button variant="ghost" size="icon" />}
                aria-label={t('close')}
              >
                <X />
              </Toast.Close>
            </Toast.Content>
          </Toast.Root>
        ))}
      </Toast.Viewport>
    </Toast.Portal>
  )
}

/** Announce one operation result once; rerenders or manual dismissal do not repeat it. */
export function useResultToast(
  id: string,
  result: unknown,
  title: string,
  action?: { label: string; run: () => void },
) {
  const manager = Toast.useToastManager()
  const previous = useRef<{ result: unknown; title: string }>({ result: null, title: '' })
  useEffect(() => {
    if (previous.current.result === result && previous.current.title === title) return
    previous.current = { result, title }
    if (!result) {
      manager.close(id)
      return
    }
    manager.add({
      id,
      title,
      timeout: action ? 0 : 5000,
      ...(action ? { actionProps: { children: action.label, onClick: action.run } } : {}),
    })
  }, [id, result, title, action, manager])
  const { close } = manager
  useEffect(() => () => close(id), [close, id])
}
