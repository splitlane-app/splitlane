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

/**
 * The toggle of the tray menu: enable in off and error, disable while routing runs. While a server setup of «Свой VPS»
 * runs the enable waits for it (`waitSetup`: greyed out, saying why); the disable never does.
 */
export function trayMenuModel(
  link: LinkStatus,
  state: HelperState | null,
  setupRunning = false
): { toggle: 'enable' | 'disable' | null; waitSetup: boolean } {
  if (link.kind !== 'ready' || !state) return { toggle: null, waitSetup: false }
  switch (state.phase) {
    case 'off':
    case 'error':
      return { toggle: 'enable', waitSetup: setupRunning }
    case 'starting':
    case 'on':
    case 'degraded':
    case 'blocked':
      return { toggle: 'disable', waitSetup: false }
    default:
      return { toggle: null, waitSetup: false }
  }
}

/**
 * The strokes of the app's glyph on a 16×16 grid (x, y from the top left): a road coming up from the bottom that
 * splits into two lanes. Drawn with round ends, HALF_WIDTH to each side of the line.
 */
const STROKES: readonly (readonly [number, number, number, number])[] = [
  [8, 13.4, 8, 9.4],
  [8, 9.4, 4.7, 6.1],
  [4.7, 6.1, 4.7, 2.6],
  [8, 9.4, 11.3, 6.1],
  [11.3, 6.1, 11.3, 2.6]
]
const HALF_WIDTH = 1.35
/** The darker edge around the glyph, in grid units (one pixel at 16 px): it keeps it apart from a light taskbar. */
const OUTLINE = 1
const OUTLINE_SHADE = 0.35
/** Samples per pixel along each axis, for the soft edges. */
const SAMPLES = 4

function distanceToStroke(px: number, py: number, [ax, ay, bx, by]: readonly [number, number, number, number]): number {
  const dx = bx - ax
  const dy = by - ay
  const t = Math.max(0, Math.min(1, ((px - ax) * dx + (py - ay) * dy) / (dx * dx + dy * dy)))
  return Math.hypot(px - (ax + t * dx), py - (ay + t * dy))
}

/**
 * The app's glyph (two lanes out of one road) at size × size pixels in the colour rgb, with a darker outline, on a
 * transparent background: BGRA, straight alpha. The tray shows it in the colour of its state, the window in the VPN
 * colour.
 */
export function laneGlyphBitmap(size: number, rgb: readonly [number, number, number]): Buffer {
  const buf = Buffer.alloc(size * size * 4)
  const unit = 16 / size
  const dark = rgb.map((c) => Math.round(c * OUTLINE_SHADE))
  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) {
      let glyph = 0
      let edge = 0
      for (let sy = 0; sy < SAMPLES; sy++) {
        for (let sx = 0; sx < SAMPLES; sx++) {
          const gx = (x + (sx + 0.5) / SAMPLES) * unit
          const gy = (y + (sy + 0.5) / SAMPLES) * unit
          let d = Infinity
          for (const s of STROKES) d = Math.min(d, distanceToStroke(gx, gy, s))
          if (d <= HALF_WIDTH) glyph++
          if (d <= HALF_WIDTH + OUTLINE) edge++
        }
      }
      if (edge === 0) continue
      const g = glyph / edge // the share of the glyph's colour in the painted part of the pixel
      const i = (y * size + x) * 4
      buf[i] = Math.round(rgb[2] * g + dark[2] * (1 - g))
      buf[i + 1] = Math.round(rgb[1] * g + dark[1] * (1 - g))
      buf[i + 2] = Math.round(rgb[0] * g + dark[0] * (1 - g))
      buf[i + 3] = Math.round((255 * edge) / (SAMPLES * SAMPLES))
    }
  }
  return buf
}
