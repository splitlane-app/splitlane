import type { Lang } from '../shared/i18n'
import type { HelperState, Phase, ReasonCode } from '../shared/protocol'
import type { UpdateState } from '../shared/update'
import { mainText } from './mainText'

// The Windows notifications of the window while it sits in the tray. This module has no electron imports: the
// transitions, the «show or not» decision and the texts are plain functions; main/index.ts only shows them.

export type NoticeKind = 'error' | 'vpnDown' | 'blocked' | 'loginExpired' | 'update'
export interface Notice {
  kind: NoticeKind
  /** The version of an `update` notice. */
  version?: string
}

/** The small part of the state the notifications look at. */
export interface Watched {
  phase: Phase
  reason: ReasonCode | null
  update: UpdateState
  /** The last version a notification was shown for (the window's settings). */
  notifiedVersion: string | null
  /** AutoEnable is still retrying the start-up enable: its failed attempts are not news. */
  retrying: boolean
}

export function watchedOf(
  state: HelperState | null,
  update: UpdateState,
  notifiedVersion: string | null,
  retrying: boolean
): Watched | null {
  if (state === null) return null
  return { phase: state.phase, reason: state.reason?.code ?? null, update, notifiedVersion, retrying }
}

/** The conditions that are in force in a state (an update is not one: it is told per version, see below). */
function conditions(s: Watched): Set<NoticeKind> {
  const c = new Set<NoticeKind>()
  // the failed attempts of the start-up retries (no network yet) are not told one by one: only the give-up is
  if (s.phase === 'error' && !(s.retrying && s.reason === 'no_default_route')) c.add('error')
  if (s.phase === 'blocked') c.add('blocked')
  if (s.phase === 'degraded' && s.reason === 'vpn_unreachable') c.add('vpnDown')
  if (s.phase === 'degraded' && s.reason === 'login_expired') c.add('loginExpired')
  return c
}

/** The version of a visible offer; null for any other state of the updater. */
function offered(s: Watched): string | null {
  return s.update.kind === 'available' && !s.update.hidden ? s.update.version : null
}

/**
 * What to tell after the state changed from `prev` (null: the first one seen) to `next`: each condition once when it
 * starts, an update once per version. Nothing repeats while a condition lasts.
 */
export function notificationsFor(prev: Watched | null, next: Watched): Notice[] {
  const before = prev === null ? new Set<NoticeKind>() : conditions(prev)
  const out: Notice[] = [...conditions(next)].filter((k) => !before.has(k)).map((kind) => ({ kind }))
  const v = offered(next)
  if (v !== null && v !== next.notifiedVersion && (prev === null || offered(prev) !== v)) {
    out.push({ kind: 'update', version: v })
  }
  return out
}

/**
 * Notifications are for a hidden window only (a visible one has its banners), when the setting is on. A window that
 * is about to be shown (a normal start, before ready-to-show) counts as visible.
 */
export function shouldShow(
  notices: Notice[],
  ctx: { enabled: boolean; windowHidden: boolean; windowOpening: boolean; supported: boolean }
): Notice[] {
  return ctx.enabled && ctx.windowHidden && !ctx.windowOpening && ctx.supported ? notices : []
}

/** The quiet period of a kind after its toast: a condition that flaps does not flood the tray. */
export const QUIET_MS = 10 * 60_000

/** Drops the notices of a kind shown less than QUIET_MS ago; an update is told once per version already. */
export function quiet(notices: Notice[], lastShown: Partial<Record<NoticeKind, number>>, now: number): Notice[] {
  return notices.filter((n) => {
    const at = lastShown[n.kind]
    return n.kind === 'update' || at === undefined || now - at > QUIET_MS
  })
}

const TITLE: Record<Exclude<NoticeKind, 'update'>, 'notifyErrorTitle' | 'notifyVpnDownTitle' | 'notifyBlockedTitle' | 'notifyLoginTitle'> = {
  error: 'notifyErrorTitle',
  vpnDown: 'notifyVpnDownTitle',
  blocked: 'notifyBlockedTitle',
  loginExpired: 'notifyLoginTitle'
}
const BODY = {
  error: 'notifyErrorBody',
  vpnDown: 'notifyVpnDownBody',
  blocked: 'notifyBlockedBody',
  loginExpired: 'notifyLoginBody'
} as const

export function noticeText(lang: Lang, n: Notice): { title: string; body: string } {
  const m = mainText(lang)
  if (n.kind === 'update') {
    return { title: m.notifyUpdateTitle.replace('{v}', n.version ?? ''), body: m.notifyUpdateBody }
  }
  return { title: m[TITLE[n.kind]], body: m[BODY[n.kind]] }
}
