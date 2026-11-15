import type { ProxyOutcome } from '../../shared/ipc'
import {
  PROXY_REJECT_CODES,
  type HelperError,
  type HelperState,
  type ProxyProtocol,
  type ProxyServer
} from '../../shared/protocol'
import { isKey, t } from '../i18n/t'
import { errorText } from '../store/appStore'
import { subscriptionOf } from './connect'

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
  return (PROXY_REJECT_CODES as readonly string[]).includes(code) && isKey(key) ? t(key) : t('proxy.unknownLine')
}

/** The line of a rejection: «Строка 3: <причина>», the reason alone for the whole text (line 0). */
export function rejectionLine(r: { line: number; code: string }): string {
  const reason = rejectionText(r.code)
  return r.line === 0 ? reason : t('proxy.line', { n: r.line, reason })
}

/**
 * The text of a failed add or refresh. A refusal of the phase means the subscription of the selected server while
 * routing is live (an add of its address again is a refresh of it); a busy helper, another download under way.
 */
export function proxyErrorText(error: HelperError): string {
  if (error.code === 'wrong_phase') return t('proxy.refreshBlocked')
  if (error.code === 'busy') return t('proxy.downloading')
  return errorText(error)
}

/** The reason of a subscription's last failed refresh; a code this window does not know reads as a failed download. */
export function subscriptionErrorText(code: string): string {
  const key = `error.${code}`
  return isKey(key) ? t(key) : t('error.fetch_failed')
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
