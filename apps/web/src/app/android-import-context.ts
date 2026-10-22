import { createContext, useContext } from 'react'
import type { DocumentKind } from '@/api/android-input'

export const AndroidImportContext = createContext<{
  available: boolean
  busy: boolean
  pick: (kind: DocumentKind) => void
}>({ available: false, busy: false, pick: () => {} })

export const useAndroidImport = () => useContext(AndroidImportContext)
