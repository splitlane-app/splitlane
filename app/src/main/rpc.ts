import type { HelperError } from '../shared/protocol'
import { LineDecoder, encodeRequest, parseLine } from '../shared/wire'

export interface Conn {
  write(text: string): void
  end(): void
  onData(cb: (chunk: Buffer) => void): void
  onClose(cb: (err?: Error) => void): void
}

export class HelperCallError extends Error {
  readonly error: HelperError
  constructor(error: HelperError) {
    super(`${error.code}: ${error.message}`)
    this.name = 'HelperCallError'
    this.error = error
  }
}

export class ConnectionClosedError extends Error {
  constructor(message = 'connection closed') {
    super(message)
    this.name = 'ConnectionClosedError'
  }
}

export class CallTimeoutError extends Error {
  constructor(method: string, timeoutMs: number) {
    super(`call ${method} timed out after ${timeoutMs} ms`)
    this.name = 'CallTimeoutError'
  }
}

interface Pending {
  resolve(v: unknown): void
  reject(e: Error): void
  timer: ReturnType<typeof setTimeout>
}

export class RpcClient {
  private readonly conn: Conn
  private readonly defaultTimeout: number
  private readonly decoder = new LineDecoder()
  private readonly pending = new Map<number, Pending>()
  private readonly eventListeners = new Set<(event: string, data: unknown) => void>()
  private readonly closeListeners = new Set<() => void>()
  private nextId = 1
  private isClosed = false

  constructor(conn: Conn, opts?: { timeoutMs?: number }) {
    this.conn = conn
    this.defaultTimeout = opts?.timeoutMs ?? 30_000
    conn.onData((chunk) => this.onData(chunk))
    conn.onClose(() => this.finish())
  }

  get closed(): boolean {
    return this.isClosed
  }

  call<T>(method: string, params?: object, timeoutMs?: number): Promise<T> {
    if (this.isClosed) return Promise.reject(new ConnectionClosedError())
    const id = this.nextId++
    const ms = timeoutMs ?? this.defaultTimeout
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id)
        reject(new CallTimeoutError(method, ms))
      }, ms)
      this.pending.set(id, { resolve: resolve as (v: unknown) => void, reject, timer })
      try {
        this.conn.write(encodeRequest(id, method, params))
      } catch (e) {
        clearTimeout(timer)
        this.pending.delete(id)
        reject(e instanceof Error ? e : new Error(String(e)))
      }
    })
  }

  onEvent(cb: (event: string, data: unknown) => void): () => void {
    this.eventListeners.add(cb)
    return () => this.eventListeners.delete(cb)
  }

  onClose(cb: () => void): () => void {
    this.closeListeners.add(cb)
    return () => this.closeListeners.delete(cb)
  }

  close(): void {
    if (this.isClosed) return
    try {
      this.conn.end()
    } finally {
      this.finish()
    }
  }

  private onData(chunk: Buffer): void {
    for (const line of this.decoder.push(chunk)) {
      const m = parseLine(line)
      if (m.kind === 'event') {
        for (const cb of [...this.eventListeners]) cb(m.event, m.data)
      } else if (m.kind === 'result' || (m.kind === 'error' && m.id !== null)) {
        const id = m.kind === 'result' ? m.id : (m.id as number)
        const p = this.pending.get(id)
        if (!p) continue // late or unknown
        this.pending.delete(id)
        clearTimeout(p.timer)
        if (m.kind === 'result') p.resolve(m.result)
        else p.reject(new HelperCallError(m.error))
      }
    }
  }

  private finish(): void {
    if (this.isClosed) return
    this.isClosed = true
    const pend = [...this.pending.values()]
    this.pending.clear()
    for (const p of pend) {
      clearTimeout(p.timer)
      p.reject(new ConnectionClosedError())
    }
    for (const cb of [...this.closeListeners]) cb()
  }
}
