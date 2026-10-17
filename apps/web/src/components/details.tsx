import {
  useEffect,
  useEffectEvent,
  useId,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from 'react'
import { useBlocker, useRouter } from '@tanstack/react-router'
import { ArrowLeft } from 'lucide-react'
import { usePreferences } from '@/app/context'
import { Button } from './ui/button'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
  SheetFooter,
} from './ui/sheet'
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerDescription,
  DrawerFooter,
} from './ui/drawer'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from './ui/dialog'

const query = '(max-width: 767px)'
const subscribe = (notify: () => void) => {
  const media = matchMedia(query)
  media.addEventListener('change', notify)
  return () => media.removeEventListener('change', notify)
}

function DiscardPrompt({
  open,
  discard,
  cancel,
}: {
  open: boolean
  discard: () => void
  cancel: () => void
}) {
  const { t } = usePreferences()
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) cancel()
      }}
    >
      <DialogContent showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>{t('discardTitle')}</DialogTitle>
          <DialogDescription>{t('discardDescription')}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={cancel}>
            {t('keepEditing')}
          </Button>
          <Button variant="destructive" onClick={discard}>
            {t('discardChanges')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** Use the existing router blocker, not a second overlay history stack. */
function PanelNavigation({
  id,
  dirty,
  inline,
  close,
}: {
  id: string
  dirty: boolean
  inline: boolean
  close: () => void
}) {
  const blocker = useBlocker({
    shouldBlockFn: () => {
      const panels = document.querySelectorAll('[data-panel]')
      return panels.item(panels.length - 1)?.id === id && (!inline || dirty)
    },
    enableBeforeUnload: dirty,
    withResolver: true,
  })
  const dismiss = useEffectEvent(() => {
    blocker.reset?.()
    close()
  })
  useEffect(() => {
    if (blocker.status === 'blocked' && !dirty) dismiss()
  }, [blocker.status, dirty])
  return (
    <DiscardPrompt
      open={blocker.status === 'blocked' && dirty}
      cancel={() => blocker.reset?.()}
      discard={() => {
        if (inline) blocker.proceed?.()
        else {
          blocker.reset?.()
          close()
        }
      }}
    />
  )
}

export function Details({
  open,
  onOpenChange,
  title,
  description,
  children,
  footer,
  mode = 'sheet',
  dirty = false,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description?: string | undefined
  children: ReactNode
  footer?: ReactNode
  mode?: 'sheet' | 'fullscreen' | 'inline'
  dirty?: boolean
}) {
  const mobile = useSyncExternalStore(
    subscribe,
    () => matchMedia(query).matches,
    () => false,
  )
  const { t } = usePreferences()
  const router = useRouter({ warn: false })
  const id = useId()
  const [confirm, setConfirm] = useState(false)
  const close = () => {
    if (dirty) setConfirm(true)
    else onOpenChange(false)
  }
  const change = (next: boolean) => {
    if (!next) close()
  }
  const guard = (
    <>
      {open && router && (
        <PanelNavigation
          id={id}
          dirty={dirty}
          inline={mode === 'inline'}
          close={() => onOpenChange(false)}
        />
      )}
      <DiscardPrompt
        open={confirm}
        cancel={() => setConfirm(false)}
        discard={() => {
          setConfirm(false)
          onOpenChange(false)
        }}
      />
    </>
  )
  const actions = (
    <>
      {footer}
      <Button variant="outline" onClick={close}>
        {t('close')}
      </Button>
    </>
  )
  if (mode === 'inline')
    return (
      <>
        <div id={id} data-panel>
          {children}
          {footer && (
            <div className="panel-inline-actions flex gap-2 border-t bg-background py-3">
              {footer}
            </div>
          )}
        </div>
        {guard}
      </>
    )
  if (mobile && mode !== 'fullscreen')
    return (
      <>
        <Drawer open={open} onOpenChange={change} showSwipeHandle>
          <DrawerContent
            id={id}
            data-panel
            className="max-h-[88dvh] pb-[env(safe-area-inset-bottom)]"
          >
            <DrawerHeader className="text-left">
              <DrawerTitle>{title}</DrawerTitle>
              <DrawerDescription className={description ? '' : 'sr-only'}>
                {description ?? title}
              </DrawerDescription>
            </DrawerHeader>
            <div className="min-h-0 overflow-y-auto overscroll-contain px-4 py-4">{children}</div>
            <DrawerFooter className="flex-row flex-wrap gap-2 pt-2">{actions}</DrawerFooter>
          </DrawerContent>
        </Drawer>
        {guard}
      </>
    )
  const full = mode === 'fullscreen'
  return (
    <>
      <Sheet open={open} onOpenChange={change}>
        <SheetContent
          id={id}
          data-panel
          showCloseButton={false}
          className={
            full
              ? 'editor-panel gap-0 data-[side=right]:w-full sm:data-[side=right]:max-w-none md:data-[side=right]:w-[min(56rem,calc(100%-216px))]'
              : 'gap-0 data-[side=right]:w-full sm:data-[side=right]:max-w-xl'
          }
        >
          <SheetHeader className="shrink-0 border-b pb-3 pt-[max(12px,env(safe-area-inset-top))]">
            <div className="flex min-w-0 items-center gap-2">
              <Button
                variant="ghost"
                size="icon"
                className="-ml-2 size-12 shrink-0"
                aria-label={t('back')}
                onClick={close}
              >
                <ArrowLeft />
              </Button>
              <SheetTitle className="min-w-0 break-all text-base">{title}</SheetTitle>
            </div>
            <SheetDescription className={full || !description ? 'sr-only' : ''}>
              {description ?? title}
            </SheetDescription>
          </SheetHeader>
          <div
            className={full ? 'editor-body p-4' : 'min-h-0 overflow-y-auto overscroll-contain p-4'}
          >
            {children}
          </div>
          <SheetFooter className="shrink-0 flex-row flex-wrap border-t pb-[max(12px,env(safe-area-inset-bottom))] pt-3">
            {actions}
          </SheetFooter>
        </SheetContent>
      </Sheet>
      {guard}
    </>
  )
}
