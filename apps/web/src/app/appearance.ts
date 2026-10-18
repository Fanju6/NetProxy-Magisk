export type Appearance = 'light' | 'dark'

export async function syncNativeAppearance(theme: Appearance): Promise<void> {
  if (import.meta.env.MODE !== 'native') return
  try {
    const { invoke, isTauri } = await import('@tauri-apps/api/core')
    if (!isTauri()) return
    await invoke('plugin:module-api|set_appearance', { theme })
  } catch {
    // Appearance failure must not interrupt the page or expose native errors.
  }
}
