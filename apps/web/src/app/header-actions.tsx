import { createContext, useContext, type ReactNode } from 'react'
import { createPortal } from 'react-dom'

const HeaderTarget = createContext<HTMLElement | null>(null)

export function HeaderActionsProvider({
  target,
  children,
}: {
  target: HTMLElement | null
  children: ReactNode
}) {
  return <HeaderTarget value={target}>{children}</HeaderTarget>
}

export function HeaderActions({ children }: { children: ReactNode }) {
  const target = useContext(HeaderTarget)
  return target ? createPortal(children, target) : null
}
