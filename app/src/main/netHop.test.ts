import { EventEmitter } from 'node:events'
import { Readable } from 'node:stream'
import { expect, test } from 'vitest'
import { hopWith, type RequestLike, type ResponseLike } from './netHop'

class FakeRequest extends EventEmitter {
  headers: Record<string, string> = {}
  aborted = 0
  ended = 0
  setHeader(name: string, value: string): void {
    this.headers[name] = value
  }
  abort(): void {
    this.aborted++
  }
  end(): void {
    this.ended++
  }
}
const asRequest = (r: FakeRequest): RequestLike => r as unknown as RequestLike

function response(chunks: string[], headers: ResponseLike['headers'] = {}, statusCode = 200): ResponseLike {
  const stream = Readable.from(chunks.map((c) => Buffer.from(c)))
  return Object.assign(stream, { statusCode, headers }) as unknown as ResponseLike
}

async function drain(body: AsyncIterable<Uint8Array>): Promise<string> {
  const parts: Buffer[] = []
  for await (const c of body) parts.push(Buffer.from(c))
  return Buffer.concat(parts).toString()
}

test('hopWith sets the User-Agent and sends the request', async () => {
  const req = new FakeRequest()
  const p = hopWith(asRequest(req), 'SplitLane/1.2.3', new AbortController().signal)
  expect(req.headers['User-Agent']).toBe('SplitLane/1.2.3')
  expect(req.ended).toBe(1)
  req.emit('response', response([]))
  await p
})

test('hopWith gives the status, the length and the body of a response', async () => {
  const req = new FakeRequest()
  const p = hopWith(asRequest(req), 'ua', new AbortController().signal)
  req.emit('response', response(['ab', 'cd'], { 'content-length': '4' }))
  const r = await p
  expect(r.status).toBe(200)
  expect(r.location).toBeNull()
  expect(r.contentLength).toBe(4)
  expect(await drain(r.body!)).toBe('abcd')
  expect(req.aborted).toBe(0)
})

test('hopWith reads a bad or missing content-length as unknown', async () => {
  for (const headers of [{}, { 'content-length': 'abc' }, { 'content-length': '-1' }]) {
    const req = new FakeRequest()
    const p = hopWith(asRequest(req), 'ua', new AbortController().signal)
    req.emit('response', response([], headers))
    expect((await p).contentLength).toBeNull()
  }
})

test('hopWith ends a redirect with its status and address and aborts instead of following', async () => {
  const req = new FakeRequest()
  const p = hopWith(asRequest(req), 'ua', new AbortController().signal)
  req.emit('redirect', 302, 'GET', 'https://objects.githubusercontent.com/x')
  const r = await p
  expect(r).toEqual({ status: 302, location: 'https://objects.githubusercontent.com/x', body: null, contentLength: null })
  expect(req.aborted).toBe(1)
  // the abort makes electron report an error afterwards: it is ignored
  req.emit('error', new Error('net::ERR_ABORTED'))
})

test('hopWith rejects on a request error', async () => {
  const req = new FakeRequest()
  const p = hopWith(asRequest(req), 'ua', new AbortController().signal)
  req.emit('error', new Error('net::ERR_NAME_NOT_RESOLVED'))
  await expect(p).rejects.toThrow('ERR_NAME_NOT_RESOLVED')
})

test('hopWith aborts the request when the signal fires before the answer', async () => {
  const req = new FakeRequest()
  const ctl = new AbortController()
  const p = hopWith(asRequest(req), 'ua', ctl.signal)
  ctl.abort()
  await expect(p).rejects.toThrow('aborted')
  expect(req.aborted).toBe(1)
})

test('hopWith does not start with an aborted signal', async () => {
  const req = new FakeRequest()
  const ctl = new AbortController()
  ctl.abort()
  await expect(hopWith(asRequest(req), 'ua', ctl.signal)).rejects.toThrow('aborted')
  expect(req.ended).toBe(0)
})

test('hopWith ends the body with an error when the signal fires while it is read', async () => {
  const req = new FakeRequest()
  const ctl = new AbortController()
  const p = hopWith(asRequest(req), 'ua', ctl.signal)
  const stream = new Readable({ read() {} })
  req.emit('response', Object.assign(stream, { statusCode: 200, headers: {} }))
  const r = await p
  const it = r.body![Symbol.asyncIterator]()
  stream.push(Buffer.from('a'))
  expect((await it.next()).value).toEqual(Buffer.from('a'))
  const next = it.next()
  ctl.abort()
  stream.push(Buffer.from('b'))
  await expect(next).rejects.toThrow('aborted')
  expect(req.aborted).toBeGreaterThanOrEqual(1)
})

test('hopWith aborts the request when the body is dropped early', async () => {
  const req = new FakeRequest()
  const p = hopWith(asRequest(req), 'ua', new AbortController().signal)
  req.emit('response', response(['a', 'b']))
  const r = await p
  const it = r.body![Symbol.asyncIterator]()
  await it.next()
  await it.return?.()
  expect(req.aborted).toBe(1)
})
