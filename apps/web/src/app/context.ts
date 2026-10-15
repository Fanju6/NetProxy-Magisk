import { createContext, useContext } from 'react'
import type { ApiClient } from '@/api/client'
import type { Translate } from './i18n'

export interface Preferences {
  theme: 'system' | 'light' | 'dark'
  locale: 'zh' | 'en'
}
export const ApiContext = createContext<ApiClient | null>(null)
export const PreferencesContext = createContext<{
  preferences: Preferences
  update: (patch: Partial<Preferences>) => void
  t: Translate
} | null>(null)
export function useApi() {
  const value = useContext(ApiContext)
  if (!value) throw new Error('API provider is missing')
  return value
}
export function usePreferences() {
  const value = useContext(PreferencesContext)
  if (!value) throw new Error('Preferences provider is missing')
  return value
}
