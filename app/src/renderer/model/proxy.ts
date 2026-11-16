import type { ProxyOutcome } from '../../shared/ipc'
import {
  PROXY_REJECT_CODES,
  type HelperError,
  type HelperState,
  type ProxyProtocol,
  type ProxyServer,
  type Subscription
} from '../../shared/protocol'
import { isKey, LIMIT_PARAMS, t } from '../i18n/t'
import { errorText } from '../store/appStore'
import { selectedHeld, selectedServer, subscriptionOf } from './connect'

/** The names of the protocols as their own projects write them; the same in every language. */
const PROTOCOL_LABEL: Record<ProxyProtocol, string> = {
  vless: 'VLESS',
  vmess: 'VMess',
  trojan: 'Trojan',
  shadowsocks: 'Shadowsocks',
  hysteria2: 'Hysteria2',
  tuic: 'TUIC',
  wireguard: 'WireGuard'
}

export function protocolLabel(protocol: ProxyProtocol): string {
  return PROTOCOL_LABEL[protocol]
}

/**
 * Where a server came from: «вручную», «файл» or the name of its subscription; null for a server whose subscription
 * is no longer in the list (the helper may keep such an orphan for a moment).
 */
export function sourceText(state: HelperState, server: ProxyServer): string | null {
  if (server.source === 'manual') return t('proxy.source.manual')
  if (server.source === 'file') return t('proxy.source.file')
  return subscriptionOf(state, server)?.name ?? null
}

/** Why a line (or the whole text) was not taken: the text of a known code, a generic one for any other. */
export function rejectionText(code: string): string {
  const key = `error.${code}`
  return (PROXY_REJECT_CODES as readonly string[]).includes(code) && isKey(key)
    ? t(key, LIMIT_PARAMS)
    : t('proxy.unknownLine')
}

/** The line of a rejection: «Строка 3: <причина>», the reason alone for the whole text (line 0). */
export function rejectionLine(r: { line: number; code: string }): string {
  const reason = rejectionText(r.code)
  return r.line === 0 ? reason : t('proxy.line', { n: r.line, reason })
}

/** What an add or a refresh asked for, so that its refusal gets the text that fits. */
export interface ProxyRequest {
  /** A subscription is downloaded: a refresh, or an add of a subscription address. */
  subscription: boolean
  /** The helper may be busy with the phase (routing starting or stopping on that subscription) rather than a download. */
  phaseBusy: boolean
}

/** A single address, as the helper takes a subscription (http too: it refuses that only after the busy check). */
export function isSubscriptionAddress(text: string): boolean {
  return /^https?:\/\/\S+$/i.test(text.trim())
}

const changing = (state: HelperState): boolean => state.phase === 'starting' || state.phase === 'stopping'

/** The add of a pasted text. A new address cannot be told from the one of the selected server's subscription. */
export function addRequest(state: HelperState, text: string): ProxyRequest {
  return { subscription: isSubscriptionAddress(text), phaseBusy: state.connector === 'proxy' && changing(state) }
}

/** The add of a WireGuard file: no download. */
export const FILE_REQUEST: ProxyRequest = { subscription: false, phaseBusy: false }

/** Whether live routing runs on a server of this subscription (the helper then keeps it as it is). */
export function holdsSelected(state: HelperState, sub: Subscription): boolean {
  return selectedHeld(state) && selectedServer(state)?.source === sub.id
}

/** The refresh of a subscription. */
export function refreshRequest(state: HelperState, sub: Subscription): ProxyRequest {
  return { subscription: true, phaseBusy: holdsSelected(state, sub) && changing(state) }
}

/**
 * The text of a failed add or refresh. A refusal of the phase for a subscription means the one of the selected server
 * while routing is live (an add of its address again is a refresh of it); a busy helper means another download, unless
 * nothing was to download or the phase may be the cause: then the general text.
 */
export function proxyErrorText(error: HelperError, req: ProxyRequest): string {
  if (error.code === 'wrong_phase' && req.subscription) return t('proxy.refreshBlocked')
  if (error.code === 'busy' && req.subscription && !req.phaseBusy) return t('proxy.downloading')
  return errorText(error)
}

/**
 * The notice of an answer that came after its screen was left: the numbers only (a line may hold a secret, and the
 * notice has no room for the reasons), or the text of the failure; nothing for a closed file dialog.
 */
export function outcomeNotice(o: ProxyOutcome, req: ProxyRequest): string | null {
  if (o.kind === 'cancelled') return null
  if (o.kind === 'failed') return proxyErrorText(o.error, req)
  const rejected = o.result.rejected.length + o.result.rejectedMore
  const parts = [t('proxy.added', { n: o.result.added.length })]
  if (rejected > 0) parts.push(t('proxy.rejectedCount', { n: rejected }))
  return parts.join(t('sep'))
}

/** The reason of a subscription's last failed refresh; a code this window does not know reads as a failed download. */
export function subscriptionErrorText(code: string): string {
  const key = `error.${code}`
  return isKey(key) ? t(key, LIMIT_PARAMS) : t('error.fetch_failed')
}

/** «Обновлена: …» in the window's language; null before the first good update or for a time that does not parse. */
export function updatedText(updatedAt: string, lang: string): string | null {
  if (updatedAt === '') return null
  const at = new Date(updatedAt)
  if (Number.isNaN(at.getTime())) return null
  const time = new Intl.DateTimeFormat(lang, { dateStyle: 'medium', timeStyle: 'short' }).format(at)
  return t('proxy.subUpdated', { time })
}

/** A thrown bridge call reads as a lost link, as a command's does. */
export async function proxyCall(call: () => Promise<ProxyOutcome>): Promise<ProxyOutcome> {
  try {
    return await call()
  } catch (e) {
    return { kind: 'failed', error: { code: 'internal', message: e instanceof Error ? e.message : String(e) } }
  }
}
