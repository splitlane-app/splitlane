// The rules of the VPS address and login: the same as ParseRequest of the helper's server-setup (Ruling 1: people
// type "host:port", the window splits it and sends the bare host and the port apart).

export const VPS_DEFAULT_PORT = 22

const OCTET = String.raw`(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)`
const IPV4 = new RegExp(String.raw`^${OCTET}(\.${OCTET}){3}$`)
const GROUP = /^[0-9a-fA-F]{1,4}$/
const LABEL = /^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$/
const USER = /^[a-z_][a-z0-9_-]{0,31}$/

/** An IPv6 address without brackets and zone: eight groups, one "::" at most, an IPv4 tail allowed. */
function validIPv6(s: string): boolean {
  if (!/^[0-9a-fA-F:.]+$/.test(s) || !s.includes(':')) return false
  let t = s
  if (t.includes('.')) {
    const i = t.lastIndexOf(':')
    if (!IPV4.test(t.slice(i + 1))) return false
    t = `${t.slice(0, i + 1)}0:0` // the IPv4 tail is two groups
  }
  const halves = t.split('::')
  if (halves.length > 2) return false
  const groups = (h: string): string[] => (h === '' ? [] : h.split(':'))
  if (halves.length === 2) {
    const all = [...groups(halves[0]), ...groups(halves[1])]
    return all.length <= 7 && all.every((g) => GROUP.test(g))
  }
  const all = t.split(':')
  return all.length === 8 && all.every((g) => GROUP.test(g))
}

/** RFC 1123 labels, 253 characters at most, no trailing dot; the last label is not all digits (so 198.51.100.300 is no name). */
function validHostName(s: string): boolean {
  if (s === '' || s.length > 253) return false
  const labels = s.split('.')
  if (!labels.every((l) => LABEL.test(l))) return false
  return /[^0-9]/.test(labels[labels.length - 1])
}

function validHost(s: string): boolean {
  return IPV4.test(s) || validIPv6(s) || validHostName(s)
}

function parsePort(s: string): number | null {
  if (!/^\d{1,5}$/.test(s)) return null
  const n = Number(s)
  return n >= 1 && n <= 65535 ? n : null
}

/**
 * What a person typed: IPv4, IPv6 (`[..]:port`, `[..]` or bare), or a name, each with `:port` or without; the port is
 * 1..65535, 22 by default; spaces at the ends are cut. The host comes back bare (IPv6 without brackets).
 */
export function parseVpsAddress(input: string): { host: string; port: number } | null {
  const s = input.trim()
  if (s.startsWith('[')) {
    const m = /^\[([^\]]*)\](?::(.*))?$/.exec(s)
    if (m === null || !validIPv6(m[1])) return null
    const port = m[2] === undefined ? VPS_DEFAULT_PORT : parsePort(m[2])
    return port === null ? null : { host: m[1], port }
  }
  const colons = s.split(':').length - 1
  if (colons >= 2) return validIPv6(s) ? { host: s, port: VPS_DEFAULT_PORT } : null
  if (colons === 1) {
    const [host, p] = s.split(':')
    const port = parsePort(p)
    return port !== null && validHost(host) ? { host, port } : null
  }
  return validHost(s) ? { host: s, port: VPS_DEFAULT_PORT } : null
}

/** 1..32 characters, `[a-z_][a-z0-9_-]*` (the useradd default of Debian and Ubuntu). */
export function validVpsUser(s: string): boolean {
  return USER.test(s)
}
