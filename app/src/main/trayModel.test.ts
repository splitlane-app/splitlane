import { describe, expect, it } from 'vitest'
import type { HelperState, Phase, ReasonCode } from '../shared/protocol'
import type { LinkStatus } from '../shared/status'
import { circleBitmap, trayIconFor, trayMenuModel, type TrayIcon } from './trayModel'

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

  it('circleBitmap has the size and a transparent corner', () => {
    const b = circleBitmap(16, [0x8b, 0x7c, 0xff])
    expect(b.length).toBe(16 * 16 * 4)
    expect(b[3]).toBe(0)
    const c = (8 * 16 + 8) * 4
    expect([b[c], b[c + 1], b[c + 2], b[c + 3]]).toEqual([0xff, 0x7c, 0x8b, 255])
  })
})
