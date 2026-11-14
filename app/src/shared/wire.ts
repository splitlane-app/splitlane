import type { HelperError } from './protocol'

export function encodeRequest(id: number, method: string, params?: object): string {
  const req: { id: number; method: string; params?: object } = { id, method }
  if (params !== undefined) req.params = params
  return JSON.stringify(req) + '\n'
}

export type Incoming =
  | { kind: 'result'; id: number; result: unknown }
  | { kind: 'error'; id: number | null; error: HelperError }
  | { kind: 'event'; event: string; data: unknown }
  | { kind: 'invalid'; line: string }

export function parseLine(line: string): Incoming {
  const invalid: Incoming = { kind: 'invalid', line }
  let v: unknown
  try {
    v = JSON.parse(line)
  } catch {
    return invalid
  }
  if (typeof v !== 'object' || v === null || Array.isArray(v)) return invalid
  const o = v as Record<string, unknown>
  if (typeof o.event === 'string') return { kind: 'event', event: o.event, data: o.data }
  if ('error' in o && typeof o.error === 'object' && o.error !== null) {
    const id = typeof o.id === 'number' ? o.id : null
    return { kind: 'error', id, error: o.error as HelperError }
  }
  if (typeof o.id === 'number' && 'result' in o) return { kind: 'result', id: o.id, result: o.result }
  return invalid
}

/** Splits a byte stream into lines of any length; multibyte characters may be cut by chunk borders. */
export class LineDecoder {
  private readonly decoder = new TextDecoder('utf-8', { fatal: false })
  private pending: string[] = []

  push(chunk: Uint8Array | string): string[] {
    const text = typeof chunk === 'string' ? chunk : this.decoder.decode(chunk, { stream: true })
    const lines: string[] = []
    let start = 0
    for (;;) {
      const nl = text.indexOf('\n', start)
      if (nl < 0) break
      this.pending.push(text.slice(start, nl))
      let line = this.pending.join('')
      this.pending = []
      if (line.endsWith('\r')) line = line.slice(0, -1)
      if (line !== '') lines.push(line)
      start = nl + 1
    }
    if (start < text.length) this.pending.push(text.slice(start))
    return lines
  }
}
