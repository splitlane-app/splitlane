export const PROTOCOL_VERSION = 1
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
  'adapter_left'
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

export interface HelperState {
  phase: Phase
  reason: Reason | null
  allDirect: boolean
  defaultLane: Lane
  apps: AppRule[]
  addresses: { vpn: string | null; direct: string | null }
  tailscale: {
    backend: Backend
    authUrl: string | null
    deviceName: string
    exitNodes: ExitNode[]
    selectedExitNode: string | null
    path: 'direct' | 'relay' | null
  }
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
  'internal'
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

export const LIMITS = { maxApps: 500, maxNameBytes: 256, maxPathBytes: 1024 } as const

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
