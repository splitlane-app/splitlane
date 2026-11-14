import net from 'node:net'
import type { Conn } from './rpc'

export type DialFailure = 'absent' | 'denied' | 'other'

export class DialError extends Error {
  readonly failure: DialFailure
  readonly code?: string
  constructor(failure: DialFailure, code?: string, message?: string) {
    super(message ?? `cannot connect to the pipe: ${failure}${code ? ` (${code})` : ''}`)
    this.name = 'DialError'
    this.failure = failure
    this.code = code
  }
}

export function classifyDialError(code: string | undefined): DialFailure {
  if (code === 'ENOENT') return 'absent'
  if (code === 'EACCES' || code === 'EPERM') return 'denied'
  return 'other'
}

export function dialPipe(name: string, timeoutMs = 5000): Promise<Conn> {
  return new Promise<Conn>((resolve, reject) => {
    const sock = net.connect(name)
    let settled = false
    const timer = setTimeout(() => {
      if (settled) return
      settled = true
      sock.destroy()
      reject(new DialError('other', 'ETIMEDOUT', 'timed out connecting to the pipe'))
    }, timeoutMs)

    const closeCbs: ((err?: Error) => void)[] = []
    let closeFired = false
    let lastErr: Error | undefined
    const fireClose = (): void => {
      if (closeFired) return
      closeFired = true
      for (const cb of closeCbs) cb(lastErr)
    }

    sock.once('connect', () => {
      if (settled) return
      settled = true
      clearTimeout(timer)
      const conn: Conn = {
        write: (text) => {
          sock.write(text)
        },
        end: () => {
          sock.end()
        },
        onData: (cb) => {
          sock.on('data', cb)
        },
        onClose: (cb) => {
          if (closeFired) cb(lastErr)
          else closeCbs.push(cb)
        }
      }
      resolve(conn)
    })
    sock.on('error', (err: NodeJS.ErrnoException) => {
      lastErr = err
      if (!settled) {
        settled = true
        clearTimeout(timer)
        reject(new DialError(classifyDialError(err.code), err.code))
      }
    })
    sock.on('close', fireClose)
  })
}
