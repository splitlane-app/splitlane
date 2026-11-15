import type { HelperState } from './protocol'
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
  /**
   * The last name seen for the selected exit node. The embedded node lists the nodes only while it runs, so with
   * routing off the pill would show the address; the page cannot patch this, the main process keeps it.
   */
  lastExitNode: LastExitNode | null
}

export interface LastExitNode {
  id: string
  name: string
}

export const DEFAULT_SETTINGS: WindowSettings = {
  openAtLogin: false,
  enableOnStart: false,
  connectedOnce: false,
  checkUpdates: true,
  pendingUpdate: null,
  language: 'system',
  lastExitNode: null
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
    language: isLanguageSetting(o.language) ? o.language : DEFAULT_SETTINGS.language,
    lastExitNode: parseLastExitNode(o.lastExitNode)
  }
}

function parseLastExitNode(x: unknown): LastExitNode | null {
  if (typeof x !== 'object' || x === null || Array.isArray(x)) return null
  const { id, name } = x as Record<string, unknown>
  if (typeof id !== 'string' || id === '' || typeof name !== 'string' || name === '') return null
  return { id, name }
}

/**
 * The value of `lastExitNode` after this state: the selected node with a name in the list when it differs from
 * `last`; null when there is nothing new to keep (no selection, the list is empty while routing is off, no name).
 */
export function rememberExitNode(last: LastExitNode | null, state: HelperState): LastExitNode | null {
  const { selectedExitNode, exitNodes } = state.tailscale
  const node = exitNodes.find((n) => n.id === selectedExitNode)
  if (node === undefined || node.name === '') return null
  if (last !== null && last.id === node.id && last.name === node.name) return null
  return { id: node.id, name: node.name }
}
