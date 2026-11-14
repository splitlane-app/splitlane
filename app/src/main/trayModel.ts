import type { HelperState } from '../shared/protocol'
import type { LinkStatus } from '../shared/status'

export type TrayIcon = 'working' | 'off' | 'action' | 'vpn-down'

/** Tray icon by the table of spec 9.6. */
export function trayIconFor(link: LinkStatus, state: HelperState | null, previous: TrayIcon): TrayIcon {
  if (link.kind === 'connecting' || link.kind === 'restarting') return previous
  if (link.kind !== 'ready') return 'action'
  if (!state) return previous
  switch (state.phase) {
    case 'starting':
    case 'stopping':
      return state.reason?.code === 'restore_pending' ? 'action' : previous
    case 'error':
    case 'blocked':
      return 'action'
    case 'degraded':
      // the all-direct mode is «нужно действие» (9.6) before the VPN without link
      return state.allDirect || state.reason?.code === 'login_expired' ? 'action' : 'vpn-down'
    case 'on':
      return state.allDirect ? 'action' : 'working'
    case 'off':
      return state.allDirect ? 'action' : 'off'
  }
}

export function trayMenuModel(link: LinkStatus, state: HelperState | null): { toggle: 'enable' | 'disable' | null } {
  if (link.kind !== 'ready' || !state) return { toggle: null }
  switch (state.phase) {
    case 'off':
    case 'error':
      return { toggle: 'enable' }
    case 'starting':
    case 'on':
    case 'degraded':
    case 'blocked':
      return { toggle: 'disable' }
    default:
      return { toggle: null }
  }
}

/** A filled circle with a 1-pixel soft edge, BGRA, transparent corners. */
export function circleBitmap(size: number, rgb: [number, number, number]): Buffer {
  const buf = Buffer.alloc(size * size * 4)
  const c = size / 2
  const r = size / 2 - 0.5
  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) {
      const d = Math.hypot(x + 0.5 - c, y + 0.5 - c)
      const a = Math.max(0, Math.min(1, r + 0.5 - d))
      const i = (y * size + x) * 4
      buf[i] = rgb[2]
      buf[i + 1] = rgb[1]
      buf[i + 2] = rgb[0]
      buf[i + 3] = Math.round(a * 255)
    }
  }
  return buf
}
