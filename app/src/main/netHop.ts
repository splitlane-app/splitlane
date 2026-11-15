import type { HopResponse } from './updater'

/** The part of electron's ClientRequest that one hop uses (electron's own object in the app, a fake in tests). */
export interface RequestLike {
  setHeader(name: string, value: string): void
  on(event: 'response', cb: (res: ResponseLike) => void): unknown
  on(event: 'redirect', cb: (statusCode: number, method: string, redirectUrl: string) => void): unknown
  on(event: 'error', cb: (err: Error) => void): unknown
  abort(): void
  end(): void
}

/** The part of electron's IncomingMessage: a readable stream with a status and headers. */
export interface ResponseLike extends AsyncIterable<Uint8Array> {
  statusCode: number
  headers: Record<string, string | string[] | undefined>
  on(event: 'error', cb: (err: Error) => void): unknown
}

function headerNumber(v: string | string[] | undefined): number | null {
  const s = Array.isArray(v) ? v[0] : v
  if (s === undefined || !/^\d{1,15}$/.test(s)) return null
  return Number(s)
}

/**
 * One request without following redirects: a redirect ends the hop with its status and address (the request is
 * aborted, never followed), a response gives its body as a stream, and an abort of the signal aborts the request.
 */
export function hopWith(req: RequestLike, userAgent: string, signal: AbortSignal): Promise<HopResponse> {
  return new Promise<HopResponse>((resolve, reject) => {
    let settled = false
    const fail = (e: Error): void => {
      if (settled) return
      settled = true
      signal.removeEventListener('abort', onAbort)
      reject(e)
    }
    const onAbort = (): void => {
      req.abort()
      fail(new Error('aborted'))
    }
    if (signal.aborted) {
      fail(new Error('aborted'))
      return
    }
    signal.addEventListener('abort', onAbort, { once: true })

    req.setHeader('User-Agent', userAgent)
    req.on('redirect', (statusCode, _method, redirectUrl) => {
      if (settled) return
      settled = true
      signal.removeEventListener('abort', onAbort)
      req.abort()
      resolve({ status: statusCode, location: redirectUrl, body: null, contentLength: null })
    })
    req.on('response', (res) => {
      if (settled) return
      settled = true
      const it = res[Symbol.asyncIterator]()
      let finished = false
      let failure: Error | null = null
      const rejectAbort: { fn: (e: Error) => void } = { fn: () => undefined }
      const aborted = new Promise<never>((_resolve, rej) => {
        rejectAbort.fn = rej
      })
      aborted.catch(() => undefined)
      const onBodyAbort = (): void => rejectAbort.fn(new Error('aborted'))
      signal.addEventListener('abort', onBodyAbort, { once: true })
      // an error of the stream is kept for the next read; after a drop nobody reads it and nothing is thrown
      res.on('error', (e) => {
        failure = e
      })
      /** Ends the body; a body that did not run to its end aborts the request, read or not. */
      const finish = (abort: boolean): void => {
        if (finished) return
        finished = true
        signal.removeEventListener('abort', onAbort)
        signal.removeEventListener('abort', onBodyAbort)
        if (abort) {
          req.abort()
          void Promise.resolve(it.return?.()).catch(() => undefined)
        }
      }
      const iterator: AsyncIterableIterator<Uint8Array> = {
        [Symbol.asyncIterator]: () => iterator,
        async next() {
          if (finished) return { done: true, value: undefined }
          if (failure !== null) {
            const e = failure
            finish(true)
            throw e
          }
          try {
            // a stream of an aborted request may never end by itself: the abort wins the race
            const r = await Promise.race([it.next(), aborted])
            if (r.done) finish(false)
            return r
          } catch (e) {
            finish(true)
            throw e
          }
        },
        return() {
          finish(true)
          return Promise.resolve({ done: true, value: undefined })
        },
        throw(e: unknown) {
          finish(true)
          return Promise.reject(e instanceof Error ? e : new Error(String(e)))
        }
      }
      resolve({
        status: res.statusCode,
        location: null,
        body: iterator,
        contentLength: headerNumber(res.headers['content-length'])
      })
    })
    req.on('error', (e) => {
      if (settled) return
      fail(e)
    })
    req.end()
  })
}
