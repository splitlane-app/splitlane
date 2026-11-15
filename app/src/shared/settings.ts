import { isLanguageSetting, type LanguageSetting } from './i18n'
import { parseVersion } from './update'

/** The window's own settings, kept in %AppData%\SplitLane\settings.json (spec 9.6). */
export interface WindowSettings {
  openAtLogin: boolean
  enableOnStart: boolean
  /** Set after the first move to the `on` phase; the "connect the VPN" screen stops opening by itself. */
  connectedOnce: boolean
  /** Automatic update checks (30 s after start, then every 24 h). */
  checkUpdates: boolean
  /** Version of an installer started by the window and not yet confirmed after the restart; x.y.z or null. */
  pendingUpdate: string | null
  /** The language of the window: one of the two, or the one of Windows. */
  language: LanguageSetting
}

export const DEFAULT_SETTINGS: WindowSettings = {
  openAtLogin: false,
  enableOnStart: false,
  connectedOnce: false,
  checkUpdates: true,
  pendingUpdate: null,
  language: 'system'
}

export function parseSettings(text: string): WindowSettings {
  let raw: unknown
  try {
    raw = JSON.parse(text)
  } catch {
    return { ...DEFAULT_SETTINGS }
  }
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) return { ...DEFAULT_SETTINGS }
  const o = raw as Record<string, unknown>
  const pick = (k: 'openAtLogin' | 'enableOnStart' | 'connectedOnce' | 'checkUpdates'): boolean =>
    typeof o[k] === 'boolean' ? (o[k] as boolean) : DEFAULT_SETTINGS[k]
  const p = o.pendingUpdate
  return {
    openAtLogin: pick('openAtLogin'),
    enableOnStart: pick('enableOnStart'),
    connectedOnce: pick('connectedOnce'),
    checkUpdates: pick('checkUpdates'),
    pendingUpdate: typeof p === 'string' && parseVersion(p) !== null ? p : null,
    language: isLanguageSetting(o.language) ? o.language : DEFAULT_SETTINGS.language
  }
}
