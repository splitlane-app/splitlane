import type { ProxyOutcome } from '../shared/ipc'
import { LIMITS, utf8Bytes } from '../shared/protocol'
import { proxyRejected, runProxyAdd, type LinkCalls } from './ipc'

/** The name the source `file` gets when the base name of the file breaks the helper's rules. */
export const FALLBACK_CONF_NAME = 'wireguard.conf'

/** An opened file: reads at most `length` bytes into `buf` (fewer at the end of the file). */
export interface ConfHandle {
  read(buf: Uint8Array, length: number): Promise<{ bytesRead: number }>
  close(): Promise<void>
}

export interface ConfDeps {
  /** The open dialog of the window (`openFile`, one `.conf` filter); the path picked, or null when cancelled. */
  pick(filterName: string): Promise<string | null>
  openFile(path: string): Promise<ConfHandle>
  /** The text of the filter, in the language in force. */
  filterName: string
  baseName(path: string): string
}

/** Reads from the handle until `max` bytes or the end of the file: one read may return less. */
export async function readUpTo(h: ConfHandle, max: number): Promise<Uint8Array> {
  const buf = new Uint8Array(max)
  let n = 0
  while (n < max) {
    const { bytesRead } = await h.read(buf.subarray(n), max - n)
    if (bytesRead === 0) break
    n += bytesRead
  }
  return buf.subarray(0, n)
}

/** The helper's name rules (ValidateProxyAdd): at most 256 bytes, no `/ \ :`, not `.` or `..`, no control character. */
export function confName(base: string): string {
  const bad =
    base === '' ||
    base === '.' ||
    base === '..' ||
    /[/\\:]/.test(base) ||
    /\p{Cc}/u.test(base) ||
    utf8Bytes(base) > LIMITS.maxNameBytes ||
    !wellFormed(base)
  return bad ? FALLBACK_CONF_NAME : base
}

function wellFormed(s: string): boolean {
  try {
    encodeURIComponent(s) // throws on a lone surrogate
    return true
  } catch {
    return false
  }
}

const UNREADABLE: ProxyOutcome = { kind: 'failed', error: { code: 'internal', message: 'file unreadable' } }

/**
 * The «.conf» file of the proxy screen: the dialog, the reading (at most 64 KiB, one handle, no stat first) and
 * proxy.add with the base name. The path stays in this process: it is neither sent nor put in an error. Never throws.
 */
export async function addConfFile(deps: ConfDeps, link: Pick<LinkCalls, 'call'>): Promise<ProxyOutcome> {
  let path: string | null
  try {
    path = await deps.pick(deps.filterName)
  } catch {
    return UNREADABLE
  }
  if (path === null) return { kind: 'cancelled' }

  let bytes: Uint8Array
  try {
    const h = await deps.openFile(path)
    try {
      bytes = await readUpTo(h, LIMITS.maxProxyTextBytes + 1)
    } finally {
      await h.close().catch(() => {})
    }
  } catch {
    return UNREADABLE
  }
  if (bytes.length > LIMITS.maxProxyTextBytes) return proxyRejected('too_long')
  if (bytes.length === 0) return proxyRejected('bad_wireguard')

  let text: string
  try {
    text = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes)
  } catch {
    return proxyRejected('bad_wireguard')
  }
  if (text.charCodeAt(0) === 0xfeff) text = text.slice(1)
  if (text.trim() === '') return proxyRejected('bad_wireguard')

  return runProxyAdd(link, text, confName(deps.baseName(path)))
}
