import { describe, expect, test } from 'vitest'
import type { Phase, ReasonCode } from '../shared/protocol'
import type { UpdateState } from '../shared/update'
import { noticeText, notificationsFor, quiet, shouldShow, watchedOf, type Notice, type Watched } from './notifications'

const N = 'https://github.com/splitlane-app/splitlane/releases/tag/v1.2.0'
const idle: UpdateState = { kind: 'idle' }
const avail = (version: string, hidden = false): UpdateState => ({ kind: 'available', version, notesUrl: N, hidden })

function w(
  phase: Phase,
  reason: ReasonCode | null = null,
  update: UpdateState = idle,
  notifiedVersion: string | null = null,
  retrying = false
): Watched {
  return { phase, reason, update, notifiedVersion, retrying }
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

describe('the login burst of the start-up retries', () => {
  const retry = (): Watched => w('error', 'no_default_route', idle, null, true)
  test('no toast per failed attempt while AutoEnable retries; one when it gives up', () => {
    expect(notificationsFor(w('off', null, idle, null, true), retry())).toEqual([])
    expect(notificationsFor(retry(), w('starting', null, idle, null, true))).toEqual([])
    expect(notificationsFor(w('starting', null, idle, null, true), retry())).toEqual([])
    // gave up in the same event: retrying is false, the error is in force
    expect(kinds(notificationsFor(retry(), w('error', 'no_default_route')))).toEqual(['error'])
    // and not again while it lasts
    expect(notificationsFor(w('error', 'no_default_route'), w('error', 'no_default_route'))).toEqual([])
  })
  test('only the no_default_route error is held back while retrying', () => {
    expect(kinds(notificationsFor(w('on', null, idle, null, true), w('error', 'ip_forwarding', idle, null, true)))).toEqual(['error'])
  })
})

describe('watchedOf', () => {
  test('is null without a state, else the small part of it', () => {
    expect(watchedOf(null, idle, null, false)).toBeNull()
    const state = { phase: 'degraded', reason: { code: 'login_expired', params: {} } } as never
    expect(watchedOf(state, idle, '1.0.0', false)).toEqual(w('degraded', 'login_expired', idle, '1.0.0'))
    expect(watchedOf({ phase: 'on', reason: null } as never, idle, null, false)).toEqual(w('on'))
    expect(watchedOf({ phase: 'on', reason: null } as never, idle, null, true)).toEqual(w('on', null, idle, null, true))
  })
})

describe('shouldShow', () => {
  const list: Notice[] = [{ kind: 'error' }]
  const on = { enabled: true, windowHidden: true, windowOpening: false, supported: true }
  test('only while the window is hidden, the setting is on and Windows supports it', () => {
    expect(shouldShow(list, on)).toEqual(list)
    expect(shouldShow(list, { ...on, windowHidden: false })).toEqual([])
    expect(shouldShow(list, { ...on, enabled: false })).toEqual([])
    expect(shouldShow(list, { ...on, supported: false })).toEqual([])
    expect(shouldShow([], on)).toEqual([])
  })
  test('a window that is about to be shown counts as visible', () => {
    expect(shouldShow(list, { ...on, windowOpening: true })).toEqual([])
  })
})

describe('quiet', () => {
  const err: Notice[] = [{ kind: 'error' }]
  test('a kind stays quiet for 10 minutes after its toast; updates are exempt', () => {
    expect(quiet(err, {}, 0)).toEqual(err)
    expect(quiet(err, { error: 0 }, 80_000)).toEqual([])
    expect(quiet(err, { error: 0 }, 600_000)).toEqual([])
    expect(quiet(err, { error: 0 }, 601_000)).toEqual(err)
    // another kind is not held by it
    expect(quiet([{ kind: 'blocked' }], { error: 0 }, 1000)).toEqual([{ kind: 'blocked' }])
    const up: Notice[] = [{ kind: 'update', version: '1.2.0' }]
    expect(quiet(up, { update: 0 }, 1000)).toEqual(up)
  })
})

describe('noticeText', () => {
  test('every kind in both languages', () => {
    const table: [Notice, string, string, string, string][] = [
      [{ kind: 'error' }, 'SplitLane: деление остановлено', 'Откройте SplitLane, чтобы узнать причину', 'SplitLane: split routing stopped', 'Open SplitLane to see why'],
      [
        { kind: 'vpnDown' },
        'SplitLane: нет связи с VPN-сервером',
        'Программы дорожки VPN без интернета, пока связь не вернётся',
        "SplitLane: can't reach the VPN server",
        'Programs in the VPN lane have no internet until the connection is back'
      ],
      [
        { kind: 'blocked' },
        'SplitLane: Tailscale снова включил exit node',
        'Деление на паузе. Откройте SplitLane, чтобы выбрать, что делать',
        'SplitLane: Tailscale turned its exit node back on',
        'Split routing is paused. Open SplitLane to choose what to do'
      ],
      [
        { kind: 'loginExpired' },
        'SplitLane: войдите в Tailscale снова',
        'Откройте SplitLane, чтобы войти',
        'SplitLane: log in to Tailscale again',
        'Open SplitLane to log in'
      ],
      [
        { kind: 'update', version: '1.2.0' },
        'SplitLane: доступна версия 1.2.0',
        'Нажмите, чтобы открыть SplitLane и обновить',
        'SplitLane: version 1.2.0 is available',
        'Click to open SplitLane and update'
      ]
    ]
    for (const [n, ruTitle, ruBody, enTitle, enBody] of table) {
      expect(noticeText('ru', n), n.kind).toEqual({ title: ruTitle, body: ruBody })
      expect(noticeText('en', n), n.kind).toEqual({ title: enTitle, body: enBody })
    }
  })
})
