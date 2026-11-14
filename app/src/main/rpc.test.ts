import { afterEach, expect, test, vi } from 'vitest'
import { CallTimeoutError, ConnectionClosedError, HelperCallError, RpcClient, type Conn } from './rpc'

class FakeConn implements Conn {
  written: string[] = []
  ended = false
  private data: ((c: Buffer) => void)[] = []
  private close: ((e?: Error) => void)[] = []
  write(t: string): void {
    this.written.push(t)
  }
  end(): void {
    this.ended = true
  }
  onData(cb: (c: Buffer) => void): void {
    this.data.push(cb)
  }
  onClose(cb: (e?: Error) => void): void {
    this.close.push(cb)
  }
  feed(s: string): void {
    for (const cb of this.data) cb(Buffer.from(s))
  }
  drop(): void {
    for (const cb of this.close) cb()
  }
  req(i: number): { id: number; method: string; params?: unknown } {
    return JSON.parse(this.written[i])
  }
}

afterEach(() => vi.useRealTimers())

test('matches results to calls by id', async () => {
  const c = new FakeConn()
  const r = new RpcClient(c)
  const a = r.call<string>('a')
  const b = r.call<string>('b', { x: 1 })
  expect(c.req(0)).toEqual({ id: 1, method: 'a' })
  expect(c.req(1)).toEqual({ id: 2, method: 'b', params: { x: 1 } })
  c.feed('{"id":2,"result":"B"}\n{"id":1,"result":"A"}\n')
  expect(await a).toBe('A')
  expect(await b).toBe('B')
})

test('rejects with HelperCallError on an error response', async () => {
  const c = new FakeConn()
  const r = new RpcClient(c)
  const p = r.call('lanes.set', {})
  c.feed('{"id":1,"error":{"code":"rule_rejected","message":"no","rules":[{"id":"u","reason":"too_broad"}]}}\n')
  const e = await p.catch((x: unknown) => x)
  expect(e).toBeInstanceOf(HelperCallError)
  expect((e as HelperCallError).error.rules).toEqual([{ id: 'u', reason: 'too_broad' }])
  expect((e as HelperCallError).error.code).toBe('rule_rejected')
})

test('delivers events in order between results', async () => {
  const c = new FakeConn()
  const r = new RpcClient(c)
  const log: string[] = []
  const l2: string[] = []
  r.onEvent((e, d) => log.push(`${e}:${JSON.stringify(d)}`))
  r.onEvent((e) => l2.push(e))
  const p = r.call('x').then(() => log.push('result'))
  const tick = (): Promise<void> => new Promise((res) => setTimeout(res, 0))
  c.feed('{"event":"state","data":1}\n{"id":1,"result":{}}\n')
  await tick()
  c.feed('{"event":"activity","data":2}\n')
  await p
  expect(log).toEqual(['state:1', 'result', 'activity:2'])
  expect(l2).toEqual(['state', 'activity'])
})

test('fails pending calls when the pipe closes', async () => {
  const c = new FakeConn()
  const r = new RpcClient(c)
  const closed = vi.fn()
  r.onClose(closed)
  const p = r.call('x')
  c.drop()
  await expect(p).rejects.toBeInstanceOf(ConnectionClosedError)
  expect(r.closed).toBe(true)
  expect(closed).toHaveBeenCalledTimes(1)
})

test('times out a call and ignores its late result', async () => {
  vi.useFakeTimers()
  const c = new FakeConn()
  const r = new RpcClient(c, { timeoutMs: 1000 })
  const p = r.call('slow')
  const assertion = expect(p).rejects.toBeInstanceOf(CallTimeoutError)
  await vi.advanceTimersByTimeAsync(1001)
  await assertion
  c.feed('{"id":1,"result":{}}\n')
  const q = r.call('next', undefined, 50)
  const qa = expect(q).rejects.toBeInstanceOf(CallTimeoutError)
  await vi.advanceTimersByTimeAsync(51)
  await qa
})

test('refuses calls after close', async () => {
  const c = new FakeConn()
  const r = new RpcClient(c)
  const p = r.call('x')
  r.close()
  expect(c.ended).toBe(true)
  await expect(p).rejects.toBeInstanceOf(ConnectionClosedError)
  await expect(r.call('y')).rejects.toBeInstanceOf(ConnectionClosedError)
  expect(c.written).toHaveLength(1)
})
