import { runCommand, runProxyAdd, type LinkCalls } from './ipc'
import { readUpTo, type ConfHandle } from './confFile'
import type { VpsEvent, VpsOutcome, VpsStep } from '../shared/ipc'
import { parseVpsAddress, validVpsUser } from '../shared/vps'

/** The key file of the window: at most 16 KiB (the helper's own limit), one byte more is read to see a larger file. */
export const MAX_KEY_BYTES = 16 * 1024
/** After the cancel the process gets this long to leave on its own. */
export const KILL_AFTER_MS = 5000
const MAX_SECRET_CHARS = 1024
/** A line of the subcommand: its longest is the final one with the links (a few KiB); anything above this is noise. */
const MAX_LINE_BYTES = 256 * 1024

const STEPS: readonly VpsStep[] = ['connect', 'system', 'install', 'harden', 'subscription']
const STATES = ['start', 'done', 'skip', 'warn'] as const

/** The part of a child process the setup uses (index.ts adapts node's ChildProcess to it). */
export interface VpsChild {
  stdin: { write(data: Uint8Array): Promise<void>; end(): void }
  onStdout(cb: (chunk: Uint8Array) => void): void
  onClose(cb: (code: number | null) => void): void
  onError(cb: (e: Error) => void): void
  kill(): void
}

export interface VpsDeps {
  /** `splitlane-helper.exe` next to the window's other helper files. */
  helperExe: string
  spawn(file: string, args: string[], options: { stdio: 'pipe'; windowsHide: true }): VpsChild
  /** The phase of the helper's state; null while there is none. */
  phase(): string | null
  link: Pick<LinkCalls, 'call'>
  emit(e: VpsEvent): void
  /** The open dialog for the key; the path picked, or null when cancelled. */
  pick(filterName: string): Promise<string | null>
  openFile(path: string): Promise<ConfHandle>
  filterName(): string
  baseName(path: string): string
  /** Runs `fn` after `ms`; the result cancels it. */
  setTimer(fn: () => void, ms: number): () => void
  log(message: string): void
}

type Obj = Record<string, unknown>

interface Request {
  host: string
  port: number
  user: string
  password?: string
  passphrase?: string
}

/** What the subcommand ended with. */
type Final = { done: { subscription: string | null; links: string; warnings: string[] } } | { failed: { code: string; detail?: string } }

function plainObject(x: unknown): Obj | null {
  if (typeof x !== 'object' || x === null || Array.isArray(x)) return null
  const proto: unknown = Object.getPrototypeOf(x)
  return proto === Object.prototype || proto === null ? (x as Obj) : null
}

function badInput(detail: string): VpsOutcome {
  return { kind: 'failed', code: 'bad_input', detail }
}

function wipe(b: Uint8Array): void {
  b.fill(0)
}

/**
 * The server setup of «Свой VPS»: runs `splitlane-helper.exe server-setup` as an ordinary child, sends it the
 * credentials on stdin only, relays its progress and adds the result to SplitLane. The key (a Buffer) and the request
 * live in this process's memory and nowhere else; nothing of them, of the answers or of the links is logged.
 */
export class VpsSetup {
  private busy = false
  private key: Buffer | null = null
  private child: VpsChild | null = null
  private cancelled = false
  private stopKill: (() => void) | null = null

  constructor(private readonly d: VpsDeps) {}

  // ---- the key ----

  /** The dialog, the reading (at most 16 KiB, one handle) and the memory; the page gets the base name only. */
  async pickKey(): Promise<{ name: string } | { error: 'too_large' | 'not_text' } | null> {
    let path: string | null
    try {
      path = await this.d.pick(this.d.filterName())
    } catch {
      return { error: 'not_text' }
    }
    if (path === null) return null
    let bytes: Uint8Array
    try {
      const h = await this.d.openFile(path)
      try {
        bytes = await readUpTo(h, MAX_KEY_BYTES + 1)
      } finally {
        await h.close().catch(() => {})
      }
    } catch {
      return { error: 'not_text' }
    }
    const copy = Buffer.from(bytes)
    wipe(bytes)
    if (copy.length > MAX_KEY_BYTES) {
      wipe(copy)
      return { error: 'too_large' }
    }
    try {
      if (copy.length === 0) throw new Error('empty')
      new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(copy)
    } catch {
      wipe(copy)
      return { error: 'not_text' }
    }
    this.forgetKey()
    this.key = copy
    return { name: this.d.baseName(path) }
  }

  forgetKey(): void {
    if (this.key !== null) wipe(this.key)
    this.key = null
  }

  /** The window is closing (or gone): the key goes with it. */
  windowClosed(): void {
    this.forgetKey()
  }

  // ---- the setup ----

  async setup(x: unknown): Promise<VpsOutcome> {
    if (this.busy) return { kind: 'failed', code: 'busy' }
    this.busy = true
    this.cancelled = false
    try {
      const out = await this.run(x)
      // the key stays after a failure or a cancel, so that «Попробовать снова» works (ruling 18); the page drops it
      if (out.kind === 'done') this.forgetKey()
      return out
    } finally {
      this.child = null
      this.stopKill?.()
      this.stopKill = null
      this.busy = false
    }
  }

  /** «Отмена»: closing stdin cancels the subcommand; a process that stays is killed after 5 s. */
  async cancel(): Promise<void> {
    const child = this.child
    if (child === null || this.cancelled) return
    this.cancelled = true
    try {
      child.stdin.end()
    } catch {
      // a closed pipe: the process is on its way out
    }
    this.stopKill = this.d.setTimer(() => child.kill(), KILL_AFTER_MS)
  }

  /** The quit of the app: the same as the cancel. */
  shutdown(): void {
    void this.cancel()
  }

  private async run(x: unknown): Promise<VpsOutcome> {
    const p = this.d.phase()
    if (p !== 'off' && p !== 'error') return { kind: 'failed', code: 'routing_on' }
    const o = plainObject(x)
    if (o === null) return badInput('request')
    const body = this.read(o)
    if ('kind' in body) {
      this.scrub(o)
      return body
    }
    const line = this.line(body)
    this.scrub(o)
    this.scrubRequest(body)
    if (line === null) return badInput('request')

    let child: VpsChild
    try {
      child = this.d.spawn(this.d.helperExe, ['server-setup'], { stdio: 'pipe', windowsHide: true })
    } catch {
      wipe(line)
      this.d.log('vps: cannot start the setup process')
      return { kind: 'failed', code: 'kit_failed', detail: 'spawn' }
    }
    this.child = child
    return this.watch(child, line)
  }

  /** The checked request of the page, as the subcommand takes it, or a bad_input. */
  private read(o: Obj): Request | VpsOutcome {
    const allowed = ['address', 'user', 'password', 'useKey', 'keyPassphrase']
    for (const k of Object.keys(o)) if (!allowed.includes(k)) return badInput('request')
    const { address, user, password, useKey, keyPassphrase } = o
    if (typeof address !== 'string' || typeof user !== 'string' || typeof useKey !== 'boolean') return badInput('request')
    if (password !== undefined && typeof password !== 'string') return badInput('password')
    if (keyPassphrase !== undefined && typeof keyPassphrase !== 'string') return badInput('passphrase')
    const where = parseVpsAddress(address)
    if (where === null) return badInput('address')
    if (!validVpsUser(user)) return badInput('user')
    const r: Request = { host: where.host, port: where.port, user }
    if (useKey) {
      if (this.key === null) return badInput('key')
      if (keyPassphrase !== undefined && keyPassphrase !== '') {
        if (keyPassphrase.length > MAX_SECRET_CHARS) return badInput('passphrase')
        r.passphrase = keyPassphrase
      }
    } else {
      if (password === undefined || password === '' || password.length > MAX_SECRET_CHARS || /[\0\r\n]/.test(password)) {
        return badInput('password')
      }
      r.password = password
    }
    return r
  }

  /** The request line with its newline as bytes; the key comes from memory. */
  private line(r: Request): Buffer | null {
    const obj: Obj = { host: r.host, port: r.port, user: r.user }
    if (r.password !== undefined) obj.password = r.password
    if (this.key !== null && r.password === undefined) {
      obj.key = this.key.toString('utf8')
      if (r.passphrase !== undefined) obj.passphrase = r.passphrase
    }
    try {
      return Buffer.from(`${JSON.stringify(obj)}\n`, 'utf8')
    } finally {
      delete obj.password
      delete obj.key
      delete obj.passphrase
    }
  }

  private scrub(o: Obj): void {
    delete o.password
    delete o.keyPassphrase
  }

  private scrubRequest(r: Request): void {
    delete r.password
    delete r.passphrase
  }

  private watch(child: VpsChild, line: Buffer): Promise<VpsOutcome> {
    return new Promise<VpsOutcome>((resolve) => {
      let final: Final | null = null
      let running: VpsStep | null = null
      let lastSeen: VpsStep | null = null
      let pending = ''
      let overflow = false
      const decoder = new TextDecoder('utf-8')

      const handle = (text: string): void => {
        let v: unknown
        try {
          v = JSON.parse(text)
        } catch {
          return // noise
        }
        const m = plainObject(v)
        if (m === null || final !== null) return
        if (typeof m.step === 'string' && (STEPS as readonly string[]).includes(m.step) && (STATES as readonly unknown[]).includes(m.state)) {
          const step = m.step as VpsStep
          const state = m.state as (typeof STATES)[number]
          const e: VpsEvent = typeof m.detail === 'string' ? { step, state, detail: m.detail } : { step, state }
          lastSeen = step
          if (state === 'start') running = step
          else if (running === step) running = null
          this.d.emit(e)
          return
        }
        const done = plainObject(m.done)
        if (done !== null) {
          const warnings = Array.isArray(done.warnings) ? done.warnings.filter((w): w is string => typeof w === 'string') : []
          if (typeof done.links === 'string' && (done.subscription === null || typeof done.subscription === 'string')) {
            final = { done: { subscription: done.subscription, links: done.links, warnings } }
          }
          return
        }
        const failed = plainObject(m.failed)
        if (failed !== null && typeof failed.code === 'string') {
          final = { failed: typeof failed.detail === 'string' ? { code: failed.code, detail: failed.detail } : { code: failed.code } }
        }
      }

      const feed = (text: string): void => {
        pending += text
        let i: number
        while ((i = pending.indexOf('\n')) >= 0) {
          const l = pending.slice(0, i).replace(/\r$/, '')
          pending = pending.slice(i + 1)
          if (overflow) overflow = false
          else handle(l)
        }
        if (pending.length > MAX_LINE_BYTES) {
          pending = ''
          overflow = true
        }
      }

      child.onStdout((chunk) => feed(decoder.decode(chunk, { stream: true })))
      child.onError(() => {
        this.d.log('vps: the setup process reported an error')
      })
      child.onClose(() => {
        this.stopKill?.()
        this.stopKill = null
        feed(decoder.decode())
        if (!overflow && pending.trim() !== '') handle(pending.trim()) // a last line without its newline
        pending = ''
        this.finish(final, running ?? lastSeen)
          .catch((): VpsOutcome => ({ kind: 'failed', code: 'kit_failed' })) // busy must always be released
          .then(resolve)
      })

      // the credentials go in, and the buffer is cleared as soon as the write is over; stdin stays open
      child.stdin.write(line).then(
        () => wipe(line),
        () => wipe(line) // a broken pipe: the close of the process tells the rest
      )
    })
  }

  private async finish(final: Final | null, running: VpsStep | null): Promise<VpsOutcome> {
    if (final === null) {
      if (this.cancelled) return { kind: 'cancelled' }
      this.failStep(running, 'kit_failed')
      return { kind: 'failed', code: 'kit_failed' }
    }
    if ('failed' in final) {
      const { code, detail } = final.failed
      if (code === 'cancelled') return { kind: 'cancelled' }
      this.failStep(running, code)
      return detail === undefined ? { kind: 'failed', code } : { kind: 'failed', code, detail }
    }
    return this.add(final.done)
  }

  private failStep(running: VpsStep | null, code: string): void {
    this.d.emit({ step: running ?? 'connect', state: 'failed', detail: code })
  }

  /** proxy.add with the address of the subscription, or the links; then proxy.select of the first server. */
  private async add(done: { subscription: string | null; links: string; warnings: string[] }): Promise<VpsOutcome> {
    this.d.emit({ step: 'add', state: 'start' })
    const fail = (detail: string): VpsOutcome => {
      this.d.emit({ step: 'add', state: 'failed', detail })
      return { kind: 'failed', code: 'add_failed', detail }
    }
    // the subscription address first; when it is refused or yields no servers, the links of the same setup (ruling 17)
    let added = await runProxyAdd(this.d.link, done.subscription ?? done.links)
    let viaLinks = done.subscription === null
    const none = (o: typeof added): boolean => o.kind !== 'done' || o.result.servers.length === 0
    if (none(added) && !viaLinks) {
      viaLinks = true
      added = await runProxyAdd(this.d.link, done.links)
    }
    if (added.kind === 'failed') return fail(added.error.code)
    if (added.kind === 'cancelled') return fail('cancelled')
    const { servers, subscription } = added.result
    if (servers.length === 0) return fail(added.result.rejected[0]?.code ?? 'no_servers')
    const sel = await runCommand(this.d.link, { method: 'proxy.select', id: servers[0] })
    if (!sel.ok) return fail(sel.error.code)
    const warnings = done.subscription !== null && viaLinks ? [...new Set([...done.warnings, 'sub_unreachable'])] : done.warnings
    this.d.emit({ step: 'add', state: 'done' })
    return { kind: 'done', servers: servers.length, subscriptionId: subscription, warnings }
  }
}
