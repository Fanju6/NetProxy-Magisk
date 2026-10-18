import {
  useEffect,
  useEffectEvent,
  useId,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from 'react'
import { useBlocker, useRouter } from '@tanstack/react-router'
import { Dialog as DialogPrimitive } from '@base-ui/react/dialog'
import { ArrowLeft, X } from 'lucide-react'
import { usePreferences } from '@/app/context'
import { Button } from './ui/button'
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription } from './ui/sheet'
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
  DialogPortal,
  DialogOverlay,
} from './ui/dialog'

const query = '(max-width: 767px)'
const subscribe = (notify: () => void) => {
  const media = matchMedia(query)
  media.addEventListener('change', notify)
  return () => media.removeEventListener('change', notify)
}
function useMobile() {
  return useSyncExternalStore(
    subscribe,
    () => matchMedia(query).matches,
    () => false,
  )
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

/** Existing router history remains the single owner of system/browser back. */
function PanelNavigation({
  id,
  dirty,
  inline = false,
  close,
}: {
  id: string
  dirty: boolean
  inline?: boolean
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

export function DraftGuard({ dirty, children }: { dirty: boolean; children: ReactNode }) {
  const id = useId()
  const router = useRouter({ warn: false })
  return (
    <div id={id} data-panel>
      {children}
      {router && <PanelNavigation id={id} dirty={dirty} inline close={() => {}} />}
    </div>
  )
}

interface PanelProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description?: string | undefined
  children: ReactNode
  footer?: ReactNode
  headerActions?: ReactNode
  dirty?: boolean
}
function useDismiss({ open, onOpenChange, dirty = false }: PanelProps) {
  const id = useId()
  const router = useRouter({ warn: false })
  const [confirm, setConfirm] = useState(false)
  const close = () => {
    if (dirty) setConfirm(true)
    else onOpenChange(false)
  }
  return {
    id,
    close,
    change: (next: boolean) => {
      if (!next) close()
    },
    guard: (
      <>
        {open && router && (
          <PanelNavigation id={id} dirty={dirty} close={() => onOpenChange(false)} />
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
    ),
  }
}

/** Short forms and read-only details: explicit business footer, dismissal in the header. */
export function Details(props: PanelProps) {
  const { open, title, description, children, footer, headerActions } = props
  const mobile = useMobile()
  const { t } = usePreferences()
  const { id, close, change, guard } = useDismiss(props)
  const dismiss = (
    <Button variant="ghost" size="icon" aria-label={t('close')} onClick={close}>
      <X />
    </Button>
  )
  return (
    <>
      {mobile ? (
        <Drawer open={open} onOpenChange={change} showSwipeHandle>
          <DrawerContent id={id} data-panel className="editor-drawer">
            <DrawerHeader className="group-data-[swipe-axis=y]/drawer-popup:text-left">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0 space-y-1 py-2">
                  <DrawerTitle>{title}</DrawerTitle>
                  <DrawerDescription className={description ? '' : 'sr-only'}>
                    {description ?? title}
                  </DrawerDescription>
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  {headerActions}
                  {dismiss}
                </div>
              </div>
            </DrawerHeader>
            <div className="panel-body">{children}</div>
            {footer && (
              <DrawerFooter className="flex-row justify-end border-t">{footer}</DrawerFooter>
            )}
          </DrawerContent>
        </Drawer>
      ) : (
        <Dialog open={open} onOpenChange={change}>
          <DialogContent
            id={id}
            data-panel
            showCloseButton={false}
            className="details-dialog flex max-h-[85dvh] flex-col gap-0 p-0 sm:max-w-lg"
          >
            <DialogHeader className="shrink-0 flex-row items-start justify-between gap-3 px-5 py-3">
              <div className="min-w-0 space-y-1 py-2">
                <DialogTitle>{title}</DialogTitle>
                <DialogDescription className={description ? '' : 'sr-only'}>
                  {description ?? title}
                </DialogDescription>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                {headerActions}
                {dismiss}
              </div>
            </DialogHeader>
            <div className="panel-body">{children}</div>
            {footer && (
              <DialogFooter className="shrink-0 flex-row justify-end border-t px-5 py-4">
                {footer}
              </DialogFooter>
            )}
          </DialogContent>
        </Dialog>
      )}
      {guard}
    </>
  )
}

/** A dedicated editing workspace, not an expanded details drawer. */
export function EditorPanel(props: PanelProps) {
  const { open, title, description, children, footer, headerActions } = props
  const mobile = useMobile()
  const { t } = usePreferences()
  const { id, close, change, guard } = useDismiss(props)
  const titleBar = (titleNode: ReactNode, descriptionNode: ReactNode) => (
    <>
      <div className="editor-toolbar">
        <Button variant="ghost" size="icon" aria-label={t('close')} onClick={close}>
          {mobile ? <ArrowLeft /> : <X />}
        </Button>
        <div className="min-w-0 flex-1">
          {titleNode}
          {descriptionNode}
        </div>
        <div className="flex shrink-0 items-center gap-1">
          {headerActions}
          {footer}
        </div>
      </div>
      <div className="editor-body">{children}</div>
    </>
  )
  return (
    <>
      {mobile ? (
        <Dialog open={open} onOpenChange={change}>
          <DialogPortal>
            <DialogOverlay />
            <DialogPrimitive.Popup
              id={id}
              data-panel
              data-slot="editor-workspace"
              className="editor-panel editor-workspace"
            >
              {titleBar(
                <DialogTitle className="truncate text-base">{title}</DialogTitle>,
                <DialogDescription className="sr-only">{description ?? title}</DialogDescription>,
              )}
            </DialogPrimitive.Popup>
          </DialogPortal>
        </Dialog>
      ) : (
        <Sheet open={open} onOpenChange={change}>
          <SheetContent
            id={id}
            data-panel
            showCloseButton={false}
            className="editor-panel gap-0 data-[side=right]:w-[min(56rem,calc(100vw-216px))] sm:data-[side=right]:max-w-none"
          >
            <SheetHeader className="sr-only">
              <SheetTitle>{title}</SheetTitle>
              <SheetDescription>{description ?? title}</SheetDescription>
            </SheetHeader>
            {titleBar(<span className="truncate text-base font-medium">{title}</span>, null)}
          </SheetContent>
        </Sheet>
      )}
      {guard}
    </>
  )
}
