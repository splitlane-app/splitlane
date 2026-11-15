export const PROTOCOL_VERSION = 2
export const PIPE_NAME = '\\\\.\\pipe\\SplitLane.Helper'
export const TASK_NAME = 'SplitLane Helper'

export type Lane = 'vpn' | 'direct'
export type Phase = 'off' | 'starting' | 'on' | 'degraded' | 'blocked' | 'stopping' | 'error'

export const REASON_CODES = [
  'no_default_route',
  'ip_forwarding',
  'other_core',
  'not_logged_in',
  'no_exit_node',
  'system_dns_override',
  'config_invalid',
  'core_start_timeout',
  'tunnel_not_proven',
  'same_address',
  'switch_check_failed',
  'leak_detected',
  'core_crashed',
  'core_unresponsive',
  'vpn_unreachable',
  'login_expired',
  'restore_pending',
  'adapter_left',
  'no_server'
] as const
export type ReasonCode = (typeof REASON_CODES)[number]
export interface Reason {
  code: ReasonCode
  params: Record<string, string>
}

export type MatcherKind = 'folder' | 'file' | 'package' | 'system'
export interface AppRule {
  id: string
  name: string
  exe: string
  matcher: { kind: MatcherKind; value: string }
  lane: Lane
}

export type Backend = 'stopped' | 'needs_login' | 'starting' | 'running' | 'needs_approval' | 'expired'
export interface ExitNode {
  id: string
  name: string
  online: boolean
}

/** The way out of the VPN lane: the embedded Tailscale node or a server of the proxy list. */
export type Connector = 'tailscale' | 'proxy'

export const PROXY_PROTOCOLS = ['vless', 'vmess', 'trojan', 'shadowsocks', 'hysteria2', 'tuic', 'wireguard'] as const
export type ProxyProtocol = (typeof PROXY_PROTOCOLS)[number]

/** A server of the list; its secrets never leave the helper. `source` is `manual`, `file` or a subscription id. */
export interface ProxyServer {
  id: string
  name: string
  protocol: ProxyProtocol
  source: string
}

/** A subscription address the helper keeps; `updatedAt` is RFC 3339, empty before the first good update. */
export interface Subscription {
  id: string
  name: string
  updatedAt: string
  count: number
  error: string | null
}

export interface HelperState {
  phase: Phase
  reason: Reason | null
  allDirect: boolean
  defaultLane: Lane
  apps: AppRule[]
  addresses: { vpn: string | null; direct: string | null }
  connector: Connector
  tailscale: {
    backend: Backend
    authUrl: string | null
    deviceName: string
    exitNodes: ExitNode[]
    selectedExitNode: string | null
    path: 'direct' | 'relay' | null
  }
  proxy: { servers: ProxyServer[]; selected: string | null; subscriptions: Subscription[] }
  systemTailscale: { installed: boolean; exitNodeOn: boolean; restoreOnDisable: boolean }
}

export const ERROR_CODES = [
  'bad_request',
  'unknown_method',
  'protocol_mismatch',
  'wrong_phase',
  'busy',
  'rule_rejected',
  'apply_failed',
  'unknown_exit_node',
  'internal',
  'unknown_server',
  'too_many',
  'empty_subscription',
  // a refresh may fail with any of these: a failed download, or the code of the first rejected line
  'fetch_blocked_address',
  'fetch_not_https',
  'fetch_too_many_redirects',
  'fetch_too_large',
  'fetch_timeout',
  'fetch_http_status',
  'fetch_failed',
  'unsupported_protocol',
  'unsupported_transport',
  'bad_link',
  'missing_field',
  'bad_wireguard',
  'amneziawg',
  'plugin',
  'too_long'
] as const
export type ErrorCode = (typeof ERROR_CODES)[number]
export type RejectReason = 'too_broad' | 'protected' | 'duplicate'
export interface RuleRejection {
  id: string
  reason: RejectReason
}
export interface HelperError {
  code: ErrorCode
  message: string
  protocol?: number
  rules?: RuleRejection[]
}

/** Why a line of the text (or the whole text, line 0) was not taken; the window has a text for each. */
export const PROXY_REJECT_CODES = [
  'unsupported_protocol',
  'unsupported_transport',
  'bad_link',
  'missing_field',
  'bad_wireguard',
  'amneziawg',
  'plugin',
  'too_long',
  'too_many',
  'empty_subscription',
  'fetch_blocked_address',
  'fetch_not_https',
  'fetch_too_many_redirects',
  'fetch_too_large',
  'fetch_timeout',
  'fetch_http_status',
  'fetch_failed'
] as const
export type ProxyRejectCode = (typeof PROXY_REJECT_CODES)[number]

/** `line` is 1-based, 0 is the whole text. `code` is any string: one this list lacks gets a generic text. */
export interface ProxyRejection {
  line: number
  code: string
}

/** The answer of proxy.add and proxy.refresh. */
export interface ProxyAddResult {
  added: string[]
  existing: number
  rejected: ProxyRejection[]
  rejectedMore: number
}

export interface HelloResult {
  protocol: number
  helper: string
  core: string
}

export interface Traffic {
  down: number
  up: number
}
export interface ActivityApp {
  key: string
  kind: 'app' | 'system' | 'unattributed'
  name: string
  exe: string
  ruleId: string | null
  lane: Lane
  down: number
  up: number
  connections: number
}
export interface ActivityDetail {
  key: string
  exes: string[]
  connections: { host: string; lane: Lane; down: number; up: number }[]
}
export interface Activity {
  lanes: { vpn: Traffic; direct: Traffic }
  apps: ActivityApp[]
  detail?: ActivityDetail
}

export const LIMITS = {
  maxApps: 500,
  maxNameBytes: 256,
  maxPathBytes: 1024,
  maxProxyTextBytes: 64 * 1024,
  maxServers: 500,
  maxSubscriptions: 20,
  maxRejections: 50
} as const

/** The helper's patterns (protocol.go serverIDRe, subscriptionIDRe). */
export const SERVER_ID = /^[0-9a-f]{16}$/
export const SUBSCRIPTION_ID = /^sub-[0-9a-f]{16}$/

const encoder = new TextEncoder()
export function utf8Bytes(s: string): number {
  return encoder.encode(s).length
}

/** Same algorithm as helper/internal/rules/matcher.go: packageValue. */
export function packageValue(exe: string): string | null {
  const parts = exe.split('\\')
  for (let i = 0; i + 2 < parts.length; i++) {
    if (parts[i].toLowerCase() !== 'windowsapps') continue
    const fields = parts[i + 1].split('_')
    if (fields.length !== 5 || fields[0] === '' || fields[4] === '') return null
    return `${fields[0]}_${fields[4]}/${parts[parts.length - 1]}`
  }
  return null
}

// ---- validation ----

type Obj = Record<string, unknown>

function obj(x: unknown, path: string): Obj {
  if (typeof x !== 'object' || x === null || Array.isArray(x)) throw new Error(`${path}: expected an object`)
  return x as Obj
}
function arr(x: unknown, path: string): unknown[] {
  if (!Array.isArray(x)) throw new Error(`${path}: expected an array`)
  return x
}
function str(x: unknown, path: string): string {
  if (typeof x !== 'string') throw new Error(`${path}: expected a string`)
  return x
}
function strOrNull(x: unknown, path: string): string | null {
  return x === null ? null : str(x, path)
}
function bool(x: unknown, path: string): boolean {
  if (typeof x !== 'boolean') throw new Error(`${path}: expected a boolean`)
  return x
}
function num(x: unknown, path: string): number {
  if (typeof x !== 'number' || !Number.isFinite(x)) throw new Error(`${path}: expected a number`)
  return x
}
function oneOf<T extends string>(x: unknown, allowed: readonly T[], path: string): T {
  if (typeof x !== 'string' || !(allowed as readonly string[]).includes(x)) throw new Error(`${path}: unknown value`)
  return x as T
}

const LANES = ['vpn', 'direct'] as const
const PHASES = ['off', 'starting', 'on', 'degraded', 'blocked', 'stopping', 'error'] as const
const KINDS = ['folder', 'file', 'package', 'system'] as const
const BACKENDS = ['stopped', 'needs_login', 'starting', 'running', 'needs_approval', 'expired'] as const
const PATHS = ['direct', 'relay'] as const
const CONNECTORS = ['tailscale', 'proxy'] as const
const ACTIVITY_KINDS = ['app', 'system', 'unattributed'] as const

function parseRule(x: unknown, path: string): AppRule {
  const o = obj(x, path)
  const m = obj(o.matcher, `${path}.matcher`)
  return {
    id: str(o.id, `${path}.id`),
    name: str(o.name, `${path}.name`),
    exe: str(o.exe, `${path}.exe`),
    matcher: { kind: oneOf(m.kind, KINDS, `${path}.matcher.kind`), value: str(m.value, `${path}.matcher.value`) },
    lane: oneOf(o.lane, LANES, `${path}.lane`)
  }
}

function int(x: unknown, path: string): number {
  const n = num(x, path)
  if (!Number.isInteger(n) || n < 0) throw new Error(`${path}: expected an integer of 0 or more`)
  return n
}

function upTo(x: unknown, maxBytes: number, path: string): string {
  const v = str(x, path)
  if (utf8Bytes(v) > maxBytes) throw new Error(`${path}: longer than ${maxBytes} bytes`)
  return v
}

function matching(x: unknown, re: RegExp, path: string): string {
  const v = str(x, path)
  if (!re.test(v)) throw new Error(`${path}: not a valid id`)
  return v
}

function parseProxy(x: unknown): HelperState['proxy'] {
  const o = obj(x, 'state.proxy')
  const rawServers = arr(o.servers, 'state.proxy.servers')
  if (rawServers.length > LIMITS.maxServers) throw new Error(`state.proxy.servers: more than ${LIMITS.maxServers}`)
  const rawSubs = arr(o.subscriptions, 'state.proxy.subscriptions')
  if (rawSubs.length > LIMITS.maxSubscriptions) throw new Error(`state.proxy.subscriptions: more than ${LIMITS.maxSubscriptions}`)
  const subscriptions = rawSubs.map((v, i): Subscription => {
    const p = `state.proxy.subscriptions[${i}]`
    const e = obj(v, p)
    return {
      id: matching(e.id, SUBSCRIPTION_ID, `${p}.id`),
      name: upTo(e.name, LIMITS.maxNameBytes, `${p}.name`),
      updatedAt: upTo(e.updatedAt, 64, `${p}.updatedAt`),
      count: int(e.count, `${p}.count`),
      error: e.error === null ? null : upTo(e.error, 64, `${p}.error`)
    }
  })
  const subIds = new Set<string>()
  for (const [i, s] of subscriptions.entries()) {
    if (subIds.has(s.id)) throw new Error(`state.proxy.subscriptions[${i}].id repeats`)
    subIds.add(s.id)
  }
  const ids = new Set<string>()
  const servers = rawServers.map((v, i): ProxyServer => {
    const p = `state.proxy.servers[${i}]`
    const e = obj(v, p)
    const id = matching(e.id, SERVER_ID, `${p}.id`)
    if (ids.has(id)) throw new Error(`${p}.id repeats`)
    ids.add(id)
    const source = str(e.source, `${p}.source`)
    if (source !== 'manual' && source !== 'file' && !subIds.has(source)) throw new Error(`${p}.source: unknown value`)
    return {
      id,
      name: upTo(e.name, LIMITS.maxNameBytes, `${p}.name`),
      protocol: oneOf(e.protocol, PROXY_PROTOCOLS, `${p}.protocol`),
      source
    }
  })
  return {
    servers,
    selected: o.selected === null ? null : matching(o.selected, SERVER_ID, 'state.proxy.selected'),
    subscriptions
  }
}

/** The answer of proxy.add and proxy.refresh; a code this window does not know is kept, not refused. */
export function parseProxyAddResult(x: unknown): ProxyAddResult {
  const o = obj(x, 'result')
  const rejected = arr(o.rejected, 'result.rejected')
  if (rejected.length > LIMITS.maxRejections) throw new Error(`result.rejected: more than ${LIMITS.maxRejections}`)
  return {
    added: arr(o.added, 'result.added').map((v, i) => matching(v, SERVER_ID, `result.added[${i}]`)),
    existing: int(o.existing, 'result.existing'),
    rejected: rejected.map((v, i) => {
      const p = `result.rejected[${i}]`
      const e = obj(v, p)
      return { line: int(e.line, `${p}.line`), code: upTo(e.code, 64, `${p}.code`) }
    }),
    rejectedMore: int(o.rejectedMore, 'result.rejectedMore')
  }
}

export function parseState(x: unknown): HelperState {
  const o = obj(x, 'state')
  let reason: Reason | null = null
  if (o.reason !== null) {
    const r = obj(o.reason, 'state.reason')
    const p = obj(r.params, 'state.reason.params')
    const params: Record<string, string> = {}
    for (const k of Object.keys(p)) params[k] = str(p[k], `state.reason.params.${k}`)
    reason = { code: oneOf(r.code, REASON_CODES, 'state.reason.code'), params }
  }
  const a = obj(o.addresses, 'state.addresses')
  const t = obj(o.tailscale, 'state.tailscale')
  const s = obj(o.systemTailscale, 'state.systemTailscale')
  return {
    phase: oneOf(o.phase, PHASES, 'state.phase'),
    reason,
    allDirect: bool(o.allDirect, 'state.allDirect'),
    defaultLane: oneOf(o.defaultLane, LANES, 'state.defaultLane'),
    connector: oneOf(o.connector, CONNECTORS, 'state.connector'),
    apps: arr(o.apps, 'state.apps').map((r, i) => parseRule(r, `state.apps[${i}]`)),
    addresses: { vpn: strOrNull(a.vpn, 'state.addresses.vpn'), direct: strOrNull(a.direct, 'state.addresses.direct') },
    tailscale: {
      backend: oneOf(t.backend, BACKENDS, 'state.tailscale.backend'),
      authUrl: strOrNull(t.authUrl, 'state.tailscale.authUrl'),
      deviceName: str(t.deviceName, 'state.tailscale.deviceName'),
      exitNodes: arr(t.exitNodes, 'state.tailscale.exitNodes').map((n, i) => {
        const e = obj(n, `state.tailscale.exitNodes[${i}]`)
        return {
          id: str(e.id, `state.tailscale.exitNodes[${i}].id`),
          name: str(e.name, `state.tailscale.exitNodes[${i}].name`),
          online: bool(e.online, `state.tailscale.exitNodes[${i}].online`)
        }
      }),
      selectedExitNode: strOrNull(t.selectedExitNode, 'state.tailscale.selectedExitNode'),
      path: t.path === null ? null : oneOf(t.path, PATHS, 'state.tailscale.path')
    },
    proxy: parseProxy(o.proxy),
    systemTailscale: {
      installed: bool(s.installed, 'state.systemTailscale.installed'),
      exitNodeOn: bool(s.exitNodeOn, 'state.systemTailscale.exitNodeOn'),
      restoreOnDisable: bool(s.restoreOnDisable, 'state.systemTailscale.restoreOnDisable')
    }
  }
}

function parseTraffic(x: unknown, path: string): Traffic {
  const o = obj(x, path)
  return { down: num(o.down, `${path}.down`), up: num(o.up, `${path}.up`) }
}

export function parseActivity(x: unknown): Activity {
  const o = obj(x, 'activity')
  const l = obj(o.lanes, 'activity.lanes')
  const out: Activity = {
    lanes: { vpn: parseTraffic(l.vpn, 'activity.lanes.vpn'), direct: parseTraffic(l.direct, 'activity.lanes.direct') },
    apps: arr(o.apps, 'activity.apps').map((v, i) => {
      const p = `activity.apps[${i}]`
      const a = obj(v, p)
      return {
        key: str(a.key, `${p}.key`),
        kind: oneOf(a.kind, ACTIVITY_KINDS, `${p}.kind`),
        name: str(a.name, `${p}.name`),
        exe: str(a.exe, `${p}.exe`),
        ruleId: strOrNull(a.ruleId, `${p}.ruleId`),
        lane: oneOf(a.lane, LANES, `${p}.lane`),
        down: num(a.down, `${p}.down`),
        up: num(a.up, `${p}.up`),
        connections: num(a.connections, `${p}.connections`)
      }
    })
  }
  if (o.detail !== undefined && o.detail !== null) {
    const d = obj(o.detail, 'activity.detail')
    out.detail = {
      key: str(d.key, 'activity.detail.key'),
      exes: arr(d.exes, 'activity.detail.exes').map((e, i) => str(e, `activity.detail.exes[${i}]`)),
      connections: arr(d.connections, 'activity.detail.connections').map((v, i) => {
        const p = `activity.detail.connections[${i}]`
        const c = obj(v, p)
        return {
          host: str(c.host, `${p}.host`),
          lane: oneOf(c.lane, LANES, `${p}.lane`),
          down: num(c.down, `${p}.down`),
          up: num(c.up, `${p}.up`)
        }
      })
    }
  }
  return out
}
