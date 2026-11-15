import { createContext, useContext, useEffect, useSyncExternalStore } from 'react'
import { useStore, type StoreApi } from 'zustand'
import type { Lang } from '../shared/i18n'
import type { Command, SplitLaneBridge } from '../shared/ipc'
import type { HelperError } from '../shared/protocol'
import type { IconCache } from './bridge'
import { currentLang, t } from './i18n/t'
import { errorText, type AppState } from './store/appStore'

/** What the components of the page work with: the store, the bridge and the icon cache. */
export interface Services {
  store: StoreApi<AppState>
  bridge: SplitLaneBridge
  icons: IconCache
}

export const ServicesContext = createContext<Services | null>(null)

export function useServices(): Services {
  const services = useContext(ServicesContext)
  if (services === null) throw new Error('useServices: no ServicesContext above this component')
  return services
}

/** A value of the store; the component renders again when it changes (Object.is). */
export function useApp<T>(selector: (s: AppState) => T): T {
  return useStore(useServices().store, selector)
}

/**
 * The language of the texts: the snapshot's, or the one the page started with before the first snapshot. A memo of
 * texts (the board's view, the drag instructions) takes it as a dependency; the root makes it the document's `lang`.
 */
export function useLang(): Lang {
  return useApp((s) => s.snapshot?.language) ?? currentLang()
}

const thrown = (e: unknown): HelperError => ({
  code: 'internal',
  message: e instanceof Error ? e.message : String(e)
})

/**
 * Sends a command of the page; a failure (an error answer or a throw of the bridge) shows its notice. A login
 * marks its link to be opened by the connect screen (a failed one does not); a logout drops that mark.
 */
export async function sendCommand({ bridge, store }: Services, c: Command): Promise<boolean> {
  const login = c.method === 'tailscale.login'
  if (login || c.method === 'tailscale.logout') store.getState().requestLogin(login)
  let error: HelperError | undefined
  try {
    const r = await bridge.command(c)
    if (!r.ok) error = r.error
  } catch (e) {
    error = thrown(e)
  }
  if (error !== undefined) {
    if (login) store.getState().requestLogin(false)
    store.getState().showNotice(errorText(error))
  }
  return error === undefined
}

/** Asks the main process for the diagnostics archive and a place to save it; tells how it went. */
export async function saveDiagnostics({ bridge, store }: Services): Promise<void> {
  let r: Awaited<ReturnType<SplitLaneBridge['saveDiagnostics']>>
  try {
    r = await bridge.saveDiagnostics()
  } catch (e) {
    r = { saved: false, error: thrown(e) }
  }
  if (r.error !== undefined) store.getState().showNotice(errorText(r.error))
  else if (r.saved) store.getState().showNotice(t('notice.logSaved'))
}

/** The icon of a program as a data URL, or null while it loads or when there is none. */
export function useIcon(exe: string): string | null {
  const { icons } = useServices()
  useEffect(() => {
    if (exe !== '') icons.request(exe)
  }, [icons, exe])
  return useSyncExternalStore(icons.subscribe, () => icons.get(exe))
}
