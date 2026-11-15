import { describe, expect, test, vi } from 'vitest'
import { IPC, type Command } from '../shared/ipc'
import { LIMITS } from '../shared/protocol'
import { PROXY_CALL_TIMEOUT_MS, registerIpc, runCommand, runProxyAdd, runProxyRefresh, validateCommand, type IpcDeps, type IpcEventLike } from './ipc'
import { CallTimeoutError, HelperCallError } from './rpc'

const SERVER = '0123456789abcdef'
const SUB = 'sub-0123456789abcdef'
const RESULT = { added: [SERVER], existing: 0, rejected: [], rejectedMore: 0 }

function link(answer: () => unknown = () => RESULT) {
  return { call: vi.fn(async (_m: string, _p?: object, _t?: number): Promise<unknown> => answer()) }
}

describe('validateCommand: proxy.remove and proxy.select', () => {
  test('remove takes a server or a subscription id', () => {
    expect(validateCommand({ method: 'proxy.remove', id: SERVER })).toEqual({ method: 'proxy.remove', id: SERVER })
    expect(validateCommand({ method: 'proxy.remove', id: SUB })).toEqual({ method: 'proxy.remove', id: SUB })
  })
  test('select takes a server id only', () => {
    expect(validateCommand({ method: 'proxy.select', id: SERVER })).toEqual({ method: 'proxy.select', id: SERVER })
    expect(() => validateCommand({ method: 'proxy.select', id: SUB })).toThrow(/server/)
  })
  test('bad ids and extra keys are refused', () => {
    for (const method of ['proxy.remove', 'proxy.select']) {
      for (const id of ['', 'xyz', SERVER.toUpperCase(), `${SERVER}0`, 'sub-1', 5, null, undefined]) {
        expect(() => validateCommand({ method, id }), `${method} ${String(id)}`).toThrow()
      }
      expect(() => validateCommand({ method, id: SERVER, extra: 1 })).toThrow(/unexpected field/)
      expect(() => validateCommand({ method })).toThrow()
    }
  })
  test('runCommand sends only the id', async () => {
    for (const c of [
      { method: 'proxy.remove', id: SUB },
      { method: 'proxy.select', id: SERVER }
    ] satisfies Command[]) {
      const l = link(() => ({}))
      expect(await runCommand(l, c)).toEqual({ ok: true })
      expect(l.call).toHaveBeenCalledWith(c.method, { id: c.id })
    }
  })
})

describe('runProxyAdd', () => {
  test('sends the text with the 30 s timeout and returns the parsed answer', async () => {
    const l = link()
    expect(await runProxyAdd(l, 'vless://x@example.com:443')).toEqual({ kind: 'done', result: RESULT })
    expect(l.call).toHaveBeenCalledWith('proxy.add', { text: 'vless://x@example.com:443' }, 30_000)
    expect(PROXY_CALL_TIMEOUT_MS).toBe(30_000)
  })
  test('a name goes along for a file', async () => {
    const l = link()
    await runProxyAdd(l, '[Interface]', 'home.conf')
    expect(l.call.mock.calls[0][1]).toStrictEqual({ text: '[Interface]', name: 'home.conf' })
  })
  test('empty or not a string: bad_request, no call', async () => {
    const l = link()
    for (const text of ['', '  \r\n\t ', 5, null, undefined, {}]) {
      expect(await runProxyAdd(l, text)).toEqual({ kind: 'failed', error: { code: 'bad_request', message: 'text is empty' } })
    }
    expect(l.call).not.toHaveBeenCalled()
  })
  test('exactly 64 KiB passes; one byte more is a line-0 too_long without a call', async () => {
    const l = link()
    const max = LIMITS.maxProxyTextBytes
    expect(max).toBe(65536)
    expect((await runProxyAdd(l, 'a'.repeat(max))).kind).toBe('done')
    expect(l.call).toHaveBeenCalledTimes(1)
    const tooLong = { kind: 'done', result: { added: [], existing: 0, rejected: [{ line: 0, code: 'too_long' }], rejectedMore: 0 } }
    expect(await runProxyAdd(l, 'a'.repeat(max + 1))).toEqual(tooLong)
    expect(l.call).toHaveBeenCalledTimes(1)
  })
  test('the limit counts bytes, not characters', async () => {
    const l = link()
    const half = 'я'.repeat(LIMITS.maxProxyTextBytes / 2) // 32768 characters, 65536 bytes
    expect((await runProxyAdd(l, half)).kind).toBe('done')
    expect(l.call).toHaveBeenCalledTimes(1)
    const out = await runProxyAdd(l, `${half}a`)
    expect(out).toMatchObject({ kind: 'done', result: { rejected: [{ line: 0, code: 'too_long' }] } })
    expect(l.call).toHaveBeenCalledTimes(1)
    // 21846 euro signs: fewer characters than the limit, 65538 bytes
    expect(await runProxyAdd(l, '€'.repeat(21846))).toMatchObject({ result: { rejected: [{ code: 'too_long' }] } })
    expect(l.call).toHaveBeenCalledTimes(1)
  })
  test('helper errors, a lost link and a bad answer come back as failed', async () => {
    const err = { code: 'busy', message: 'a download is running' } as const
    const busy = link(() => {
      throw new HelperCallError({ ...err })
    })
    expect(await runProxyAdd(busy, 'x')).toEqual({ kind: 'failed', error: err })
    const slow = link(() => {
      throw new CallTimeoutError('proxy.add', 30000)
    })
    expect(await runProxyAdd(slow, 'x')).toEqual({ kind: 'failed', error: { code: 'internal', message: 'link closed' } })
    expect(await runProxyAdd(link(() => ({ added: 'no' })), 'x')).toMatchObject({ kind: 'failed', error: { code: 'internal' } })
  })
})

describe('runProxyRefresh', () => {
  test('a subscription id, the 30 s timeout', async () => {
    const l = link()
    expect(await runProxyRefresh(l, SUB)).toEqual({ kind: 'done', result: RESULT })
    expect(l.call).toHaveBeenCalledWith('proxy.refresh', { id: SUB }, 30_000)
  })
  test('anything else is bad_request without a call', async () => {
    const l = link()
    for (const id of [SERVER, '', 'sub-', 7, null, undefined]) {
      expect(await runProxyRefresh(l, id)).toMatchObject({ kind: 'failed', error: { code: 'bad_request' } })
    }
    expect(l.call).not.toHaveBeenCalled()
  })
  test('a failed download keeps the helper code', async () => {
    const e = new HelperCallError({ code: 'fetch_timeout', message: 'timeout' })
    const l = link(() => {
      throw e
    })
    expect(await runProxyRefresh(l, SUB)).toEqual({ kind: 'failed', error: { code: 'fetch_timeout', message: 'timeout' } })
  })
})

describe('the proxy channels', () => {
  test('checked sender, checked arguments, file dialog without arguments', async () => {
    const handlers = new Map<string, (event: IpcEventLike, ...args: unknown[]) => unknown>()
    const page = { id: 'page' }
    const l = link()
    const proxyAddFile = vi.fn(async () => ({ kind: 'cancelled' as const }))
    registerIpc({
      ipc: { handle: (c: string, fn: (event: IpcEventLike, ...args: unknown[]) => unknown) => void handlers.set(c, fn) },
      accept: (e: IpcEventLike) => e.sender === page,
      link: { ...l, setActivityDetail: async () => {}, retry: () => {} },
      proxyAddFile,
      log: () => {}
    } as unknown as IpcDeps)
    const call = (channel: string, ...args: unknown[]): unknown => handlers.get(channel)?.({ sender: page }, ...args)
    expect(await call(IPC.proxyAdd, 'vless://x')).toMatchObject({ kind: 'done' })
    expect(await call(IPC.proxyAdd, 42)).toMatchObject({ kind: 'failed' })
    expect(await call(IPC.proxyRefresh, SUB)).toMatchObject({ kind: 'done' })
    expect(await call(IPC.proxyRefresh, SERVER)).toMatchObject({ kind: 'failed' })
    expect(await call(IPC.proxyAddFile, '/some/path')).toEqual({ kind: 'cancelled' })
    expect(proxyAddFile).toHaveBeenCalledWith()
    expect(() => handlers.get(IPC.proxyAdd)?.({ sender: {} }, 'x')).toThrow(/unexpected sender/)
    expect(l.call).toHaveBeenCalledTimes(2)
  })
})
