import { describe, expect, test } from 'vitest'
import type { Phase, ReasonCode } from '../shared/protocol'
import type { UpdateState } from '../shared/update'
import { noticeText, notificationsFor, shouldShow, watchedOf, type Notice, type Watched } from './notifications'

const N = 'https://github.com/splitlane-app/splitlane/releases/tag/v1.2.0'
const idle: UpdateState = { kind: 'idle' }
const avail = (version: string, hidden = false): UpdateState => ({ kind: 'available', version, notesUrl: N, hidden })

function w(phase: Phase, reason: ReasonCode | null = null, update: UpdateState = idle, notifiedVersion: string | null = null): Watched {
  return { phase, reason, update, notifiedVersion }
}
const kinds = (n: Notice[]): string[] => n.map((x) => x.kind)

describe('notificationsFor: the transition table', () => {
  const entries: [string, Watched, string][] = [
    ['error', w('error', 'ip_forwarding'), 'error'],
    ['error without a reason', w('error'), 'error'],
    ['vpn unreachable', w('degraded', 'vpn_unreachable'), 'vpnDown'],
    ['blocked', w('blocked'), 'blocked'],
    ['login expired', w('degraded', 'login_expired'), 'loginExpired']
  ]
  for (const [name, cond, kind] of entries) {
    test(`${name}: once on entry, not while it lasts, again after it clears`, () => {
      expect(kinds(notificationsFor(w('on'), cond))).toEqual([kind])
      expect(notificationsFor(cond, cond)).toEqual([])
      expect(notificationsFor(cond, w('on'))).toEqual([])
      expect(kinds(notificationsFor(w('on'), cond))).toEqual([kind])
      // the first state seen already in the condition counts as an entry
      expect(kinds(notificationsFor(null, cond))).toEqual([kind])
    })
  }

  test('nothing for starting, stopping, on, off or a plain degraded', () => {
    for (const p of ['starting', 'stopping', 'on', 'off'] as const) {
      expect(notificationsFor(w('degraded', 'vpn_unreachable'), w(p)), p).toEqual([])
      expect(notificationsFor(null, w(p)), p).toEqual([])
    }
    expect(notificationsFor(w('on'), w('degraded', 'restore_pending'))).toEqual([])
    expect(notificationsFor(w('on'), w('degraded'))).toEqual([])
  })

  test('a move from one condition to another notifies for the new one', () => {
    expect(kinds(notificationsFor(w('degraded', 'vpn_unreachable'), w('blocked')))).toEqual(['blocked'])
    expect(kinds(notificationsFor(w('degraded', 'vpn_unreachable'), w('degraded', 'login_expired')))).toEqual(['loginExpired'])
  })

  test('update: once per version, never for a hidden offer or an old notified version', () => {
    expect(notificationsFor(w('on'), w('on', null, avail('1.2.0')))).toEqual([{ kind: 'update', version: '1.2.0' }])
    // the same offer on the next snapshot
    expect(notificationsFor(w('on', null, avail('1.2.0')), w('on', null, avail('1.2.0')))).toEqual([])
    // a version already notified
    expect(notificationsFor(w('on'), w('on', null, avail('1.2.0'), '1.2.0'))).toEqual([])
    // a new version after it
    expect(notificationsFor(w('on', null, avail('1.2.0'), '1.2.0'), w('on', null, avail('1.3.0'), '1.2.0'))).toEqual([
      { kind: 'update', version: '1.3.0' }
    ])
    // «Позже» hides the offer
    expect(notificationsFor(w('on'), w('on', null, avail('1.2.0', true)))).toEqual([])
    // the states around the offer are not offers
    for (const u of [
      { kind: 'checking', manual: false },
      { kind: 'downloading', version: '1.2.0', notesUrl: N, percent: 1 },
      { kind: 'installed', version: '1.2.0' }
    ] satisfies UpdateState[]) {
      expect(notificationsFor(w('on'), w('on', null, u)), u.kind).toEqual([])
    }
  })

  test('two events at once come out together', () => {
    expect(kinds(notificationsFor(w('on'), w('blocked', null, avail('1.2.0'))))).toEqual(['blocked', 'update'])
  })
})

describe('watchedOf', () => {
  test('is null without a state, else the small part of it', () => {
    expect(watchedOf(null, idle, null)).toBeNull()
    const state = { phase: 'degraded', reason: { code: 'login_expired', params: {} } } as never
    expect(watchedOf(state, idle, '1.0.0')).toEqual(w('degraded', 'login_expired', idle, '1.0.0'))
    expect(watchedOf({ phase: 'on', reason: null } as never, idle, null)).toEqual(w('on'))
  })
})

describe('shouldShow', () => {
  const list: Notice[] = [{ kind: 'error' }]
  const on = { enabled: true, windowHidden: true, supported: true }
  test('only while the window is hidden, the setting is on and Windows supports it', () => {
    expect(shouldShow(list, on)).toEqual(list)
    expect(shouldShow(list, { ...on, windowHidden: false })).toEqual([])
    expect(shouldShow(list, { ...on, enabled: false })).toEqual([])
    expect(shouldShow(list, { ...on, supported: false })).toEqual([])
    expect(shouldShow([], on)).toEqual([])
  })
})

describe('noticeText', () => {
  test('texts per language', () => {
    expect(noticeText('ru', { kind: 'error' })).toEqual({
      title: 'SplitLane: деление остановлено',
      body: 'Откройте SplitLane, чтобы узнать причину'
    })
    expect(noticeText('en', { kind: 'error' })).toEqual({
      title: 'SplitLane: split routing stopped',
      body: 'Open SplitLane to see why'
    })
    expect(noticeText('en', { kind: 'vpnDown' }).title).toBe("SplitLane: can't reach the VPN server")
    expect(noticeText('ru', { kind: 'vpnDown' }).title).toBe('SplitLane: нет связи с VPN-сервером')
    expect(noticeText('en', { kind: 'blocked' }).title).toBe('SplitLane: Tailscale turned its exit node back on')
    expect(noticeText('ru', { kind: 'blocked' }).body).toBe('Деление на паузе. Откройте SplitLane, чтобы выбрать, что делать')
    expect(noticeText('en', { kind: 'loginExpired' }).title).toBe('SplitLane: log in to Tailscale again')
    expect(noticeText('ru', { kind: 'loginExpired' }).title).toBe('SplitLane: войдите в Tailscale снова')
    expect(noticeText('ru', { kind: 'update', version: '1.2.0' })).toEqual({
      title: 'SplitLane: доступна версия 1.2.0',
      body: 'Нажмите, чтобы открыть SplitLane и обновить'
    })
    expect(noticeText('en', { kind: 'update', version: '1.2.0' }).title).toBe('SplitLane: version 1.2.0 is available')
  })
})
