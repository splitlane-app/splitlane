import {
  IPC,
  LINK_CLOSED,
  MAX_DROPPED,
  type AppInfo,
  type Command,
  type CommandResult,
  type ProxyOutcome,
  type Snapshot
} from '../shared/ipc'
import { isLanguageSetting, type Lang } from '../shared/i18n'
import type { Dropped, Program } from '../shared/programs'
import {
  LIMITS,
  SERVER_ID,
  SUBSCRIPTION_ID,
  parseProxyAddResult,
  parseProxyRevealResult,
  utf8Bytes,
  type AppRule,
  type HelperError,
  type Lane,
  type MatcherKind
} from '../shared/protocol'
import type { WindowSettings } from '../shared/settings'
import { notesUrlFor, parseVersion, type UpdateState } from '../shared/update'
import { CallTimeoutError, ConnectionClosedError, HelperCallError } from './rpc'

// ---- checks of what the page sends (spec 11: the page is checked like any other client) ----

type Obj = Record<string, unknown>

function plain(x: unknown, path: string): Obj {
  if (typeof x !== 'object' || x === null || Array.isArray(x)) throw new Error(`${path}: expected an object`)
  const proto: unknown = Object.getPrototypeOf(x)
  if (proto !== Object.prototype && proto !== null) throw new Error(`${path}: expected a plain object`)
  return x as Obj
}

function onlyKeys(o: Obj, allowed: readonly string[], path: string): void {
  for (const k of Object.keys(o)) {
    if (!allowed.includes(k)) throw new Error(`${path}: unexpected field ${JSON.stringify(k)}`)
  }
}

function str(x: unknown, path: string): string {
  if (typeof x !== 'string') throw new Error(`${path}: expected a string`)
  return x
}

function bool(x: unknown, path: string): boolean {
  if (typeof x !== 'boolean') throw new Error(`${path}: expected a boolean`)
  return x
}

function upTo(s: string, maxBytes: number, path: string): string {
  if (utf8Bytes(s) > maxBytes) throw new Error(`${path} is longer than ${maxBytes} bytes`)
  return s
}

function oneOf<T extends string>(x: unknown, allowed: readonly T[], path: string): T {
  if (typeof x !== 'string' || !(allowed as readonly string[]).includes(x)) throw new Error(`${path}: unknown value`)
  return x as T
}

const LANES: readonly Lane[] = ['vpn', 'direct']
const KINDS: readonly MatcherKind[] = ['folder', 'file', 'package', 'system']
/** The helper's regexp (protocol.go uuidRe): rules it accepted come back unchanged. */
const UUID = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/
const OCTET = '(25[0-5]|2[0-4]\\d|1\\d\\d|[1-9]?\\d)'
const IPV4 = new RegExp(`^${OCTET}(\\.${OCTET}){3}$`)
const DRIVE_PATH = /^[A-Za-z]:\\/
const MAX_URL_BYTES = 2048

function rule(x: unknown, path: string): AppRule {
  const o = plain(x, path)
  onlyKeys(o, ['id', 'name', 'exe', 'matcher', 'lane'], path)
  const id = str(o.id, `${path}.id`)
  if (!UUID.test(id)) throw new Error(`${path}.id is not a UUID`)
  const m = plain(o.matcher, `${path}.matcher`)
  onlyKeys(m, ['kind', 'value'], `${path}.matcher`)
  return {
    id,
    name: upTo(str(o.name, `${path}.name`), LIMITS.maxNameBytes, `${path}.name`),
    exe: upTo(str(o.exe, `${path}.exe`), LIMITS.maxPathBytes, `${path}.exe`),
    matcher: {
      kind: oneOf(m.kind, KINDS, `${path}.matcher.kind`),
      value: upTo(str(m.value, `${path}.matcher.value`), LIMITS.maxPathBytes, `${path}.matcher.value`)
    },
    lane: oneOf(o.lane, LANES, `${path}.lane`)
  }
}

function rules(x: unknown): AppRule[] {
  if (!Array.isArray(x)) throw new Error('apps: expected an array')
  if (x.length > LIMITS.maxApps) throw new Error(`apps: more than ${LIMITS.maxApps} rules`)
  const seen = new Set<string>()
  return x.map((r: unknown, i) => {
    const a = rule(r, `apps[${i}]`)
    if (seen.has(a.id)) throw new Error(`apps[${i}].id repeats`)
    seen.add(a.id)
    return a
  })
}

/** A command of the page with exactly the fields of its method; anything else throws. */
export function validateCommand(x: unknown): Command {
  const o = plain(x, 'command')
  const method = o.method
  switch (method) {
    case 'routing.enable':
    case 'tailscale.login':
    case 'tailscale.logout':
    case 'systemTailscale.clearExitNode':
      onlyKeys(o, ['method'], method)
      return { method }
    case 'routing.disable':
      onlyKeys(o, ['method', 'skipRestore'], method)
      return o.skipRestore === undefined ? { method } : { method, skipRestore: bool(o.skipRestore, 'skipRestore') }
    case 'routing.allDirect':
      onlyKeys(o, ['method', 'on'], method)
      return { method, on: bool(o.on, 'on') }
    case 'lanes.set':
      onlyKeys(o, ['method', 'apps', 'defaultLane'], method)
      return { method, apps: rules(o.apps), defaultLane: oneOf(o.defaultLane, LANES, 'defaultLane') }
    case 'tailscale.setExitNode': {
      onlyKeys(o, ['method', 'id'], method)
      const id = str(o.id, 'id')
      if (!IPV4.test(id)) throw new Error('id must be an IPv4 address')
      return { method, id }
    }
    case 'proxy.remove': {
      onlyKeys(o, ['method', 'id'], method)
      const id = str(o.id, 'id')
      if (!SERVER_ID.test(id) && !SUBSCRIPTION_ID.test(id)) throw new Error('id must be the id of a server or a subscription')
      return { method, id }
    }
    case 'proxy.select': {
      onlyKeys(o, ['method', 'id'], method)
      const id = str(o.id, 'id')
      if (!SERVER_ID.test(id)) throw new Error('id must be the id of a server')
      return { method, id }
    }
    default:
      throw new Error(`unknown method ${JSON.stringify(method)}`)
  }
}

/** The login link of the embedded node (`https://login.tailscale.com/...`), normalized; anything else is null. */
export function validateLoginUrl(x: unknown): string | null {
  if (typeof x !== 'string' || x === '' || utf8Bytes(x) > MAX_URL_BYTES) return null
  let u: URL
  try {
    u = new URL(x)
  } catch {
    return null
  }
  if (u.protocol !== 'https:' || u.hostname !== 'login.tailscale.com' || u.port !== '') return null
  if (u.username !== '' || u.password !== '') return null
  return u.href
}

/** A program path for an icon: absolute with a drive letter, at most 1024 bytes. */
export function validateExePath(x: unknown): string {
  const p = upTo(str(x, 'exe'), LIMITS.maxPathBytes, 'exe')
  if (!DRIVE_PATH.test(p)) throw new Error('exe: expected an absolute path with a drive letter')
  return p
}

/** Paths of an Explorer drop: at most 100 strings of at most 1024 bytes (resolveDropped skips the odd ones). */
export function validatePathList(x: unknown): string[] {
  if (!Array.isArray(x)) throw new Error('paths: expected an array')
  if (x.length > MAX_DROPPED) throw new Error(`paths: more than ${MAX_DROPPED} files`)
  return x.map((p: unknown, i) => upTo(str(p, `paths[${i}]`), LIMITS.maxPathBytes, `paths[${i}]`))
}

/** The activity detail key: null or a string of at most 1024 bytes (the helper's detailFor limit). */
export function validateDetailKey(x: unknown): string | null {
  if (x === null) return null
  return upTo(str(x, 'key'), LIMITS.maxPathBytes, 'key')
}

// pendingUpdate, lastExitNode and notifiedVersion belong to the main process: the page cannot set them
const BOOL_SETTING_KEYS = ['openAtLogin', 'enableOnStart', 'connectedOnce', 'checkUpdates', 'notifications'] as const satisfies readonly (keyof WindowSettings)[]
const SETTING_KEYS = [...BOOL_SETTING_KEYS, 'language'] as const

export function validateSettingsPatch(x: unknown): Partial<WindowSettings> {
  const o = plain(x, 'settings')
  onlyKeys(o, SETTING_KEYS, 'settings')
  const out: Partial<WindowSettings> = {}
  for (const k of BOOL_SETTING_KEYS) {
    if (o[k] !== undefined) out[k] = bool(o[k], `settings.${k}`)
  }
  if (o.language !== undefined) {
    if (!isLanguageSetting(o.language)) throw new Error('settings.language: expected "system", "ru" or "en"')
    out.language = o.language
  }
  return out
}

const NOTES_PREFIX = notesUrlFor('') // https://github.com/splitlane-app/splitlane/releases/tag/v

/** The release page of the state, or null: exactly https://github.com/splitlane-app/splitlane/releases/tag/v<x.y.z>. */
export function notesUrlOf(s: UpdateState): string | null {
  if (!('notesUrl' in s)) return null
  const url = s.notesUrl
  if (!url.startsWith(NOTES_PREFIX)) return null
  const version = url.slice(NOTES_PREFIX.length)
  return parseVersion(version) !== null && url === notesUrlFor(version) ? url : null
}

// ---- the command path ----

function errText(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

/** A failed call as the page sees it: the helper's own error, or `link closed` when the link is gone. */
export function toHelperError(e: unknown): HelperError {
  if (e instanceof HelperCallError) return e.error
  if (e instanceof ConnectionClosedError || e instanceof CallTimeoutError) return { ...LINK_CLOSED }
  return { code: 'internal', message: errText(e) }
}

/** Method and params of a command: exactly the protocol fields (the helper decodes strictly). */
function request(c: Command): { method: string; params?: object } {
  switch (c.method) {
    case 'routing.disable':
      return c.skipRestore === true ? { method: c.method, params: { skipRestore: true } } : { method: c.method }
    case 'routing.allDirect':
      return { method: c.method, params: { on: c.on } }
    case 'lanes.set':
      return { method: c.method, params: { apps: c.apps, defaultLane: c.defaultLane } }
    case 'tailscale.setExitNode':
    case 'proxy.remove':
    case 'proxy.select':
      return { method: c.method, params: { id: c.id } }
    default:
      return { method: c.method }
  }
}

/** The part of HelperLink the page reaches. */
export interface LinkCalls {
  call(method: string, params?: object, timeoutMs?: number): Promise<unknown>
  setActivityDetail(key: string | null): Promise<void>
  retry(): void
}

/** Sends a command to the helper; never throws. */
export async function runCommand(link: Pick<LinkCalls, 'call'>, c: Command): Promise<CommandResult> {
  const { method, params } = request(c)
  try {
    await link.call(method, params)
    return { ok: true }
  } catch (e) {
    return { ok: false, error: toHelperError(e) }
  }
}

// ---- proxy.add and proxy.refresh ----

/** A download inside proxy.add or proxy.refresh takes up to 20 s on the helper's side. */
export const PROXY_CALL_TIMEOUT_MS = 30_000

export function proxyRejected(code: string): ProxyOutcome {
  return { kind: 'done', result: { added: [], existing: 0, servers: [], subscription: null, rejected: [{ line: 0, code }], rejectedMore: 0 } }
}

async function proxyCall(link: Pick<LinkCalls, 'call'>, method: string, params: object): Promise<ProxyOutcome> {
  try {
    return { kind: 'done', result: parseProxyAddResult(await link.call(method, params, PROXY_CALL_TIMEOUT_MS)) }
  } catch (e) {
    return { kind: 'failed', error: toHelperError(e) }
  }
}

/**
 * proxy.add: a text of at most 64 KiB in UTF-8, not empty. A text too long is answered here as the whole-text
 * rejection `too_long`, without a call; `name` is the base name of a `.conf` read from a file. Never throws.
 */
export async function runProxyAdd(link: Pick<LinkCalls, 'call'>, text: unknown, name?: string): Promise<ProxyOutcome> {
  if (typeof text !== 'string') return { kind: 'failed', error: { code: 'bad_request', message: 'text is empty' } }
  // UTF-8 bytes are never fewer than UTF-16 units: a long string is refused before it is trimmed or encoded
  if (text.length > LIMITS.maxProxyTextBytes) return proxyRejected('too_long')
  if (text.trim() === '') return { kind: 'failed', error: { code: 'bad_request', message: 'text is empty' } }
  if (utf8Bytes(text) > LIMITS.maxProxyTextBytes) return proxyRejected('too_long')
  return proxyCall(link, 'proxy.add', name === undefined ? { text } : { text, name })
}

/** proxy.refresh: `id` is the id of a subscription. Never throws. */
export async function runProxyRefresh(link: Pick<LinkCalls, 'call'>, id: unknown): Promise<ProxyOutcome> {
  if (typeof id !== 'string' || !SUBSCRIPTION_ID.test(id)) {
    return { kind: 'failed', error: { code: 'bad_request', message: 'id must be the id of a subscription' } }
  }
  return proxyCall(link, 'proxy.refresh', { id })
}

/** proxy.reveal: `id` is the id of a subscription. The address is returned, never logged. Never throws. */
export async function runProxyReveal(link: Pick<LinkCalls, 'call'>, id: unknown): Promise<{ address: string } | { error: HelperError }> {
  if (typeof id !== 'string' || !SUBSCRIPTION_ID.test(id)) {
    return { error: { code: 'bad_request', message: 'id must be the id of a subscription' } }
  }
  try {
    return parseProxyRevealResult(await link.call('proxy.reveal', { id }))
  } catch (e) {
    return { error: toHelperError(e) }
  }
}

/** The archive path in the answer of diagnostics.export. */
export function exportPath(result: unknown): string {
  const p = typeof result === 'object' && result !== null ? (result as Obj).path : undefined
  if (typeof p !== 'string' || !DRIVE_PATH.test(p) || !p.toLowerCase().endsWith('.zip')) {
    throw new Error('diagnostics.export: unexpected result')
  }
  return p
}

/** Start-menu programs, Store apps and games in one list: one entry per exe (the first wins), in the order of the language. */
export function mergePrograms(lang: Lang, ...lists: Program[][]): Program[] {
  const seen = new Set<string>()
  const out: Program[] = []
  for (const list of lists) {
    for (const p of list) {
      const key = p.exe.toLowerCase()
      if (seen.has(key)) continue
      seen.add(key)
      out.push(p)
    }
  }
  return out.sort((a, b) => a.name.localeCompare(b.name, lang))
}

// ---- registration ----

export interface IpcEventLike {
  sender: unknown
}

/** The part of ipcMain that registerIpc uses. */
export interface IpcHandle {
  handle(channel: string, listener: (event: IpcEventLike, ...args: unknown[]) => unknown): void
}

export interface IpcDeps {
  ipc: IpcHandle
  /** True for the window's own page. */
  accept(event: IpcEventLike): boolean
  link: LinkCalls
  snapshot(): Snapshot
  /** A routing command came from the user: the auto-enable of this run stops. */
  userCommand(): void
  installHelper(): Promise<void>
  /** The file dialog for a `.conf`, its reading and proxy.add (confFile.ts). */
  proxyAddFile(): Promise<ProxyOutcome>
  appInfo(): Promise<AppInfo>
  setSettings(patch: Partial<WindowSettings>): Promise<WindowSettings>
  installedPrograms(): Promise<Program[]>
  pickProgram(): Promise<Dropped | null>
  resolveDropped(paths: string[]): Promise<Dropped[]>
  icon(exe: string): Promise<string | null>
  openExternal(url: string): Promise<void>
  saveDiagnostics(): Promise<{ saved: boolean; error?: HelperError }>
  quit(): Promise<void>
  updateState(): UpdateState
  checkUpdates(): Promise<void>
  installUpdate(): Promise<void>
  cancelUpdate(): void
  updateLater(): void
  dismissUpdate(): void
  log(message: string): void
}

/** The request channels of SplitLaneBridge; every argument from the page is checked here. */
export function registerIpc(d: IpcDeps): void {
  const on = (channel: string, fn: (...args: unknown[]) => unknown): void => {
    d.ipc.handle(channel, (event, ...args) => {
      if (!d.accept(event)) throw new Error(`${channel}: unexpected sender`)
      return fn(...args)
    })
  }

  on(IPC.snapshot, () => d.snapshot())
  on(IPC.command, async (x) => {
    let c: Command
    try {
      c = validateCommand(x)
    } catch (e) {
      d.log(`ipc: refused a command: ${errText(e)}`)
      return { ok: false, error: { code: 'bad_request', message: errText(e) } } satisfies CommandResult
    }
    if (c.method.startsWith('routing.')) d.userCommand()
    return runCommand(d.link, c)
  })
  on(IPC.setActivityDetail, async (x) => {
    const key = validateDetailKey(x)
    try {
      await d.link.setActivityDetail(key)
    } catch (e) {
      d.log(`ipc: activity detail: ${errText(e)}`)
    }
  })
  on(IPC.installHelper, () => d.installHelper())
  on(IPC.proxyAdd, (x) => runProxyAdd(d.link, x))
  on(IPC.proxyAddFile, () => d.proxyAddFile())
  on(IPC.proxyRefresh, (x) => runProxyRefresh(d.link, x))
  on(IPC.proxyReveal, (x) => runProxyReveal(d.link, x))
  on(IPC.appInfo, () => d.appInfo())
  on(IPC.retryLink, () => {
    d.link.retry()
  })
  on(IPC.setSettings, (x) => d.setSettings(validateSettingsPatch(x)))
  on(IPC.installedPrograms, () => d.installedPrograms())
  on(IPC.pickProgram, () => d.pickProgram())
  on(IPC.resolveDropped, (x) => d.resolveDropped(validatePathList(x)))
  on(IPC.icon, async (x) => {
    let exe: string
    try {
      exe = validateExePath(x)
    } catch {
      return null
    }
    try {
      return await d.icon(exe)
    } catch {
      return null
    }
  })
  on(IPC.openLogin, async (x) => {
    const url = validateLoginUrl(x)
    if (url === null) return false
    try {
      await d.openExternal(url)
      return true
    } catch {
      d.log('ipc: cannot open the login link') // the error text may carry the link: not logged
      return false
    }
  })
  on(IPC.saveDiagnostics, () => d.saveDiagnostics())
  on(IPC.quit, () => d.quit())
  // no argument of the page is read: the buttons carry none
  // a check or a download runs on its own: the answer comes at once, so that «Отменить» stays pressable
  const detached = (name: string, op: () => Promise<void>): void => {
    op().catch((e: unknown) => d.log(`ipc: ${name}: ${errText(e)}`))
  }
  on(IPC.checkUpdates, () => detached('checkUpdates', () => d.checkUpdates()))
  on(IPC.installUpdate, () => detached('installUpdate', () => d.installUpdate()))
  on(IPC.cancelUpdate, () => {
    d.cancelUpdate()
  })
  on(IPC.updateLater, () => {
    d.updateLater()
  })
  on(IPC.dismissUpdate, () => {
    d.dismissUpdate()
  })
  on(IPC.openUpdateNotes, async () => {
    const url = notesUrlOf(d.updateState())
    if (url === null) return false
    try {
      await d.openExternal(url)
      return true
    } catch {
      d.log('ipc: cannot open the release page')
      return false
    }
  })
}
