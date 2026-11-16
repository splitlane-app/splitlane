import { describe, expect, it } from 'vitest'
import type { HelperState, Phase, ReasonCode } from '../shared/protocol'
import type { LinkStatus } from '../shared/status'
import { laneGlyphBitmap, trayIconFor, trayMenuModel, type TrayIcon } from './trayModel'

const ready: LinkStatus = { kind: 'ready', hello: { protocol: 1, helper: 'x', core: 'y' } }
function st(phase: Phase, code?: ReasonCode, allDirect = false): HelperState {
  return { phase, reason: code ? { code, params: {} } : null, allDirect } as HelperState
}

describe('trayModel', () => {
  it('trayIconFor follows the table of 9.6', () => {
    const rows: [LinkStatus, HelperState | null, TrayIcon][] = [
      [ready, st('on'), 'working'],
      [ready, st('off'), 'off'],
      [ready, st('degraded', 'vpn_unreachable'), 'vpn-down'],
      [ready, st('degraded', 'login_expired'), 'action'],
      [ready, st('degraded', 'core_crashed'), 'vpn-down'],
      [ready, st('blocked', 'leak_detected'), 'action'],
      [ready, st('error', 'no_default_route'), 'action'],
      [ready, st('stopping', 'restore_pending'), 'action'],
      [ready, st('on', undefined, true), 'action'],
      [ready, st('off', undefined, true), 'action'],
      // the all-direct mode needs an action whatever the phase says (allDirect wins)
      [ready, st('degraded', 'vpn_unreachable', true), 'action'],
      [ready, st('degraded', 'core_crashed', true), 'action'],
      [{ kind: 'setup-needed' }, null, 'action'],
      [{ kind: 'other-user' }, null, 'action'],
      [{ kind: 'helper-older', helperProtocol: 0 }, null, 'action'],
      [{ kind: 'helper-newer', helperProtocol: 2 }, null, 'action'],
      [{ kind: 'unresponsive' }, null, 'action']
    ]
    for (const [link, state, want] of rows) {
      expect(trayIconFor(link, state, 'working')).toBe(want)
    }
    expect(trayIconFor({ kind: 'connecting' }, null, 'off')).toBe('off')
    expect(trayIconFor({ kind: 'restarting', attempt: 1 }, null, 'vpn-down')).toBe('vpn-down')
    expect(trayIconFor(ready, null, 'off')).toBe('off')
  })

  it('keeps the previous icon in starting and stopping', () => {
    for (const p of ['starting', 'stopping'] as const) {
      expect(trayIconFor(ready, st(p), 'vpn-down')).toBe('vpn-down')
      expect(trayIconFor(ready, st(p), 'working')).toBe('working')
    }
    expect(trayIconFor(ready, st('starting', 'core_crashed'), 'off')).toBe('off')
  })

  it('trayMenuModel holds the enable back while a server setup runs; the disable stays', () => {
    expect(trayMenuModel(ready, st('off'), true)).toEqual({ toggle: 'enable', waitSetup: true })
    expect(trayMenuModel(ready, st('error', 'core_crashed'), true)).toEqual({ toggle: 'enable', waitSetup: true })
    expect(trayMenuModel(ready, st('off'), false)).toEqual({ toggle: 'enable', waitSetup: false })
    expect(trayMenuModel(ready, st('on'), true)).toEqual({ toggle: 'disable', waitSetup: false })
    expect(trayMenuModel(ready, st('off'))).toEqual({ toggle: 'enable', waitSetup: false })
  })

  it('trayMenuModel offers enable in off and error, disable in live phases', () => {
    expect(trayMenuModel(ready, st('off')).toggle).toBe('enable')
    expect(trayMenuModel(ready, st('error', 'core_crashed')).toggle).toBe('enable')
    for (const p of ['starting', 'on', 'degraded', 'blocked'] as const) {
      expect(trayMenuModel(ready, st(p)).toggle).toBe('disable')
    }
    expect(trayMenuModel(ready, st('stopping')).toggle).toBeNull()
    expect(trayMenuModel({ kind: 'connecting' }, st('off')).toggle).toBeNull()
    expect(trayMenuModel(ready, null).toggle).toBeNull()
  })

  describe('laneGlyphBitmap', () => {
    const VPN: [number, number, number] = [0x8b, 0x7c, 0xff]
    const WARN: [number, number, number] = [0xff, 0xb4, 0x54]
    const px = (b: Buffer, size: number, x: number, y: number): number[] => {
      const i = (y * size + x) * 4
      return [b[i], b[i + 1], b[i + 2], b[i + 3]]
    }

    it('has the size, BGRA, and transparent corners', () => {
      for (const size of [16, 20, 24, 32, 48]) {
        const b = laneGlyphBitmap(size, VPN)
        expect(b.length, `${size}`).toBe(size * size * 4)
        for (const [x, y] of [
          [0, 0],
          [size - 1, 0],
          [0, size - 1],
          [size - 1, size - 1]
        ]) {
          expect(px(b, size, x, y)[3], `${size}: ${x},${y}`).toBe(0)
        }
      }
    })

    it('paints the glyph in the state colour', () => {
      for (const rgb of [VPN, WARN]) {
        const b = laneGlyphBitmap(16, rgb)
        // the road below the split, and the tops of the two lanes
        for (const [x, y] of [
          [7, 11],
          [8, 11],
          [4, 4],
          [11, 4]
        ]) {
          expect(px(b, 16, x, y), `${x},${y}`).toEqual([rgb[2], rgb[1], rgb[0], 255])
        }
        // between the two lanes nothing is painted
        expect(px(b, 16, 8, 3)[3]).toBeLessThan(64)
      }
      const big = laneGlyphBitmap(32, VPN)
      expect(px(big, 32, 15, 22)).toEqual([VPN[2], VPN[1], VPN[0], 255])
    })

    it('has a darker outline for a light taskbar', () => {
      const b = laneGlyphBitmap(16, VPN)
      // left of the road: solid, darker than the glyph in every channel
      const edge = px(b, 16, 6, 11)
      expect(edge[3]).toBe(255)
      expect(edge[0]).toBeLessThan(VPN[2])
      expect(edge[1]).toBeLessThan(VPN[1])
      expect(edge[2]).toBeLessThan(VPN[0])
      const outside = px(b, 16, 5, 11)
      expect(outside[3]).toBeGreaterThan(0)
      expect(outside[3]).toBeLessThan(255)
      expect(outside.slice(0, 3)).toEqual([VPN[2], VPN[1], VPN[0]].map((c) => Math.round(c * 0.35)))
    })
  })
})
