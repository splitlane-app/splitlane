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
}

export function watchedOf(state: HelperState | null, update: UpdateState, notifiedVersion: string | null): Watched | null {
  if (state === null) return null
  return { phase: state.phase, reason: state.reason?.code ?? null, update, notifiedVersion }
}

/** The conditions that are in force in a state (an update is not one: it is told per version, see below). */
function conditions(s: Watched): Set<NoticeKind> {
  const c = new Set<NoticeKind>()
  if (s.phase === 'error') c.add('error')
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

/** Notifications are for a hidden window only (a visible one has its banners), when the setting is on. */
export function shouldShow(
  notices: Notice[],
  ctx: { enabled: boolean; windowHidden: boolean; supported: boolean }
): Notice[] {
  return ctx.enabled && ctx.windowHidden && ctx.supported ? notices : []
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
