/** The window's own settings, kept in %AppData%\SplitLane\settings.json (spec 9.6). */
export interface WindowSettings {
  openAtLogin: boolean
  enableOnStart: boolean
  /** Set after the first move to the `on` phase; the "connect the VPN" screen stops opening by itself. */
  connectedOnce: boolean
}

export const DEFAULT_SETTINGS: WindowSettings = {
  openAtLogin: false,
  enableOnStart: false,
  connectedOnce: false
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
  const pick = (k: keyof WindowSettings): boolean => (typeof o[k] === 'boolean' ? (o[k] as boolean) : DEFAULT_SETTINGS[k])
  return { openAtLogin: pick('openAtLogin'), enableOnStart: pick('enableOnStart'), connectedOnce: pick('connectedOnce') }
}
