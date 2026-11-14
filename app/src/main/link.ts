import {
  LIMITS,
  PROTOCOL_VERSION,
  parseActivity,
  parseState,
  utf8Bytes,
  type Activity,
  type HelloResult,
  type HelperState
} from '../shared/protocol'
import type { LinkStatus } from '../shared/status'
import { DialError, type DialFailure } from './pipe'
import { ConnectionClosedError, HelperCallError, RpcClient, type Conn } from './rpc'

export type { LinkStatus } from '../shared/status'

/** Section 10: up to five runs of the task, each followed by a wait for the pipe of up to 2 s. */
export const RESTART_ATTEMPTS = 5
export const RESTART_WAIT_MS = 2000
export const DIAL_POLL_MS = 200
/** hello, state.get and activity.subscribe are answered at once on a fresh connection. */
export const HANDSHAKE_TIMEOUT_MS = 10_000

export interface LinkDeps {
  dial(): Promise<Conn>
  taskRegistered(): Promise<boolean>
  runTask(): Promise<void>
  sleep(ms: number): Promise<void>
  log(message: string): void
}

interface LinkEvents {
  status: LinkStatus
  state: HelperState
  activity: Activity
}
type Listeners = { [K in keyof LinkEvents]: Set<(x: LinkEvents[K]) => void> }

/** done: a final status is set (ready, helper-older/newer, other-user); stale: a newer start took over. */
type Outcome = 'done' | 'failed' | 'stale'

function errText(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

function dialFailure(e: unknown): DialFailure {
  return e instanceof DialError ? e.failure : 'other'
}

function subscribeParams(detailFor: string | null): { detailFor?: string } {
  return detailFor === null ? {} : { detailFor }
}

function parseHello(x: unknown): HelloResult {
  const o = (typeof x === 'object' && x !== null ? x : {}) as Record<string, unknown>
  if (typeof o.protocol !== 'number' || typeof o.helper !== 'string' || typeof o.core !== 'string') {
    throw new Error('hello: unexpected result')
  }
  return { protocol: o.protocol, helper: o.helper, core: o.core }
}

/**
 * The main process's link to the helper: dial, handshake (hello, state.get, activity.subscribe),
 * events, and the restart of the scheduled task when the pipe is lost (spec 4.3, 5.1, section 10).
 * Every wait goes through deps.sleep; a newer start(), retry() or stop() cancels the running one.
 */
export class HelperLink {
  private readonly deps: LinkDeps
  private readonly listeners: Listeners = { status: new Set(), state: new Set(), activity: new Set() }
  private current: LinkStatus = { kind: 'connecting' }
  private lastState: HelperState | null = null
  private detailFor: string | null = null
  private rpc: RpcClient | null = null
  private generation = 0

  constructor(deps: LinkDeps) {
    this.deps = deps
  }

  start(): void {
    this.begin()
  }

  /** Starts over from any status; an open connection is closed first. */
  retry(): void {
    this.begin()
  }

  /** Cancels a connect or restart in progress and closes the connection; the status is left as it was. */
  stop(): void {
    this.generation++
    this.dropRpc()
  }

  status(): LinkStatus {
    return this.current
  }

  /** The last state from the helper; kept while the link restarts. */
  state(): HelperState | null {
    return this.lastState
  }

  call<T>(method: string, params?: object, timeoutMs?: number): Promise<T> {
    const rpc = this.rpc
    if (rpc === null || this.current.kind !== 'ready') {
      return Promise.reject(new ConnectionClosedError('no link to the helper'))
    }
    return rpc.call<T>(method, params, timeoutMs)
  }

  /** Remembers the program whose detail the activity carries (null: none) and sends it when ready. */
  async setActivityDetail(key: string | null): Promise<void> {
    const detailFor = key === '' ? null : key
    if (detailFor !== null && utf8Bytes(detailFor) > LIMITS.maxPathBytes) {
      throw new Error(`detailFor is longer than ${LIMITS.maxPathBytes} bytes`)
    }
    this.detailFor = detailFor
    const rpc = this.rpc
    if (rpc === null || this.current.kind !== 'ready') return // sent by the next handshake
    try {
      await rpc.call('activity.subscribe', subscribeParams(detailFor))
    } catch (e) {
      if (!(e instanceof ConnectionClosedError)) throw e
      // the link dropped: the key is sent again after the reconnect
    }
  }

  on(event: 'status', cb: (s: LinkStatus) => void): () => void
  on(event: 'state', cb: (s: HelperState) => void): () => void
  on(event: 'activity', cb: (a: Activity) => void): () => void
  on<K extends keyof LinkEvents>(event: K, cb: (x: LinkEvents[K]) => void): () => void {
    const set: Set<(x: LinkEvents[K]) => void> = this.listeners[event]
    set.add(cb)
    return () => {
      set.delete(cb)
    }
  }

  // ---- internals ----

  private begin(): void {
    const gen = ++this.generation
    this.dropRpc()
    this.setStatus({ kind: 'connecting' })
    this.run(this.connect(gen))
  }

  private run(p: Promise<unknown>): void {
    p.catch((e: unknown) => this.deps.log(`link: unexpected error: ${errText(e)}`))
  }

  private async connect(gen: number): Promise<void> {
    let conn: Conn
    try {
      conn = await this.deps.dial()
    } catch (e) {
      if (gen !== this.generation) return
      if (dialFailure(e) === 'denied') {
        this.setStatus({ kind: 'other-user' })
        return
      }
      this.deps.log(`link: cannot connect to the helper: ${errText(e)}`)
      await this.recover(gen)
      return
    }
    if ((await this.handshake(conn, gen)) === 'failed') await this.recover(gen)
  }

  /** The pipe is absent or does not answer: restart the task when it is registered, else setup. */
  private async recover(gen: number): Promise<void> {
    let registered = false
    try {
      registered = await this.deps.taskRegistered()
    } catch (e) {
      this.deps.log(`link: cannot query the task: ${errText(e)}`)
    }
    if (gen !== this.generation) return
    if (!registered) {
      this.setStatus({ kind: 'setup-needed' })
      return
    }
    await this.restartLoop(gen)
  }

  private async restartLoop(gen: number): Promise<void> {
    for (let attempt = 1; attempt <= RESTART_ATTEMPTS; attempt++) {
      this.setStatus({ kind: 'restarting', attempt })
      try {
        await this.deps.runTask()
      } catch (e) {
        // a failed run fails the attempt unless the helper's pipe shows up anyway
        this.deps.log(`link: restart attempt ${attempt}: ${errText(e)}`)
      }
      if (gen !== this.generation) return
      if ((await this.waitForPipe(gen)) !== 'failed') return
    }
    this.setStatus({ kind: 'unresponsive' })
  }

  /** Dials every DIAL_POLL_MS for up to RESTART_WAIT_MS and does the handshake once the pipe is there. */
  private async waitForPipe(gen: number): Promise<Outcome> {
    for (let waited = 0; waited < RESTART_WAIT_MS; waited += DIAL_POLL_MS) {
      await this.deps.sleep(DIAL_POLL_MS)
      if (gen !== this.generation) return 'stale'
      let conn: Conn
      try {
        conn = await this.deps.dial()
      } catch (e) {
        if (gen !== this.generation) return 'stale'
        if (dialFailure(e) === 'denied') {
          this.setStatus({ kind: 'other-user' })
          return 'done'
        }
        continue
      }
      return this.handshake(conn, gen)
    }
    return 'failed'
  }

  private async handshake(conn: Conn, gen: number): Promise<Outcome> {
    if (gen !== this.generation) {
      try {
        conn.end()
      } catch {
        // already gone
      }
      return 'stale'
    }
    this.dropRpc()
    const rpc = new RpcClient(conn)
    this.rpc = rpc
    rpc.onEvent((event, data) => this.onEvent(rpc, event, data))
    rpc.onClose(() => this.onClosed(rpc))
    try {
      const hello = parseHello(await rpc.call('hello', { protocol: PROTOCOL_VERSION }, HANDSHAKE_TIMEOUT_MS))
      if (gen !== this.generation) return 'stale'
      const state = parseState(await rpc.call('state.get', undefined, HANDSHAKE_TIMEOUT_MS))
      if (gen !== this.generation) return 'stale'
      this.setState(state)
      let sent: string | null
      do {
        sent = this.detailFor
        await rpc.call('activity.subscribe', subscribeParams(sent), HANDSHAKE_TIMEOUT_MS)
        if (gen !== this.generation) return 'stale'
      } while (sent !== this.detailFor) // changed while the subscribe was in flight
      if (rpc.closed) throw new ConnectionClosedError() // closed after the last answer
      this.setStatus({ kind: 'ready', hello })
      return 'done'
    } catch (e) {
      if (gen !== this.generation) return 'stale'
      this.dropRpc()
      if (e instanceof HelperCallError && e.error.code === 'protocol_mismatch') {
        const p = typeof e.error.protocol === 'number' ? e.error.protocol : 0
        this.setStatus(
          p > PROTOCOL_VERSION ? { kind: 'helper-newer', helperProtocol: p } : { kind: 'helper-older', helperProtocol: p }
        )
        return 'done'
      }
      this.deps.log(`link: handshake failed: ${errText(e)}`)
      return 'failed'
    }
  }

  /** Called by RpcClient inside its read loop: must not throw. */
  private onEvent(rpc: RpcClient, event: string, data: unknown): void {
    if (this.rpc !== rpc) return
    if (event === 'state') {
      let s: HelperState
      try {
        s = parseState(data)
      } catch (e) {
        this.deps.log(`link: ignored a state event: ${errText(e)}`)
        return
      }
      this.setState(s)
    } else if (event === 'activity') {
      let a: Activity
      try {
        a = parseActivity(data)
      } catch (e) {
        this.deps.log(`link: ignored an activity event: ${errText(e)}`)
        return
      }
      this.emit('activity', a)
    }
  }

  private onClosed(rpc: RpcClient): void {
    if (this.rpc !== rpc) return // replaced or dropped on purpose
    this.rpc = null
    if (this.current.kind !== 'ready') return // a handshake in progress handles its own failure
    this.deps.log('link: the helper pipe closed')
    this.run(this.restartLoop(++this.generation))
  }

  private dropRpc(): void {
    const rpc = this.rpc
    if (rpc === null) return
    this.rpc = null
    try {
      rpc.close()
    } catch (e) {
      this.deps.log(`link: closing the pipe: ${errText(e)}`)
    }
  }

  private setState(s: HelperState): void {
    this.lastState = s
    this.emit('state', s)
  }

  private setStatus(s: LinkStatus): void {
    if (JSON.stringify(s) === JSON.stringify(this.current)) return
    this.current = s
    this.emit('status', s)
  }

  private emit<K extends keyof LinkEvents>(event: K, value: LinkEvents[K]): void {
    const set: Set<(x: LinkEvents[K]) => void> = this.listeners[event]
    for (const cb of [...set]) {
      try {
        cb(value)
      } catch (e) {
        this.deps.log(`link: a ${event} listener failed: ${errText(e)}`)
      }
    }
  }
}
