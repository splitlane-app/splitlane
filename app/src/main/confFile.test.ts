import { describe, expect, test, vi } from 'vitest'
import { LIMITS } from '../shared/protocol'
import { FALLBACK_CONF_NAME, addConfFile, confName, readUpTo, type ConfDeps, type ConfHandle } from './confFile'

const PATH = String.raw`C:\Users\someone\Documents\wg-home.conf`
const RESULT = { added: ['0123456789abcdef'], existing: 0, servers: ['0123456789abcdef'], subscription: null, rejected: [], rejectedMore: 0 }
const MAX = LIMITS.maxProxyTextBytes

/** A file of `bytes` that hands out at most `chunk` bytes per read. */
function handle(bytes: Uint8Array, chunk = 1 << 20): ConfHandle & { closed: boolean; asked: number[] } {
  let pos = 0
  const h = {
    closed: false,
    asked: [] as number[],
    read: async (buf: Uint8Array, length: number) => {
      h.asked.push(length)
      const n = Math.min(length, chunk, bytes.length - pos)
      buf.set(bytes.subarray(pos, pos + n))
      pos += n
      return { bytesRead: n }
    },
    close: async () => {
      h.closed = true
    }
  }
  return h
}

function setup(file: Uint8Array | Error | null, over: Partial<ConfDeps> = {}) {
  const h = file instanceof Uint8Array ? handle(file, 1000) : null
  const deps: ConfDeps = {
    pick: vi.fn(async (_filterName: string) => (file === null ? null : PATH)),
    openFile: vi.fn(async (_path: string) => {
      if (file instanceof Error) throw file
      return h!
    }),
    filterName: 'WireGuard config',
    baseName: (p) => p.slice(p.lastIndexOf('\\') + 1),
    ...over
  }
  const link = { call: vi.fn(async (_m: string, _p?: object, _t?: number): Promise<unknown> => RESULT) }
  return { deps, link, h }
}
const enc = (s: string): Uint8Array => new TextEncoder().encode(s)

describe('addConfFile', () => {
  test('cancel: nothing is read or sent', async () => {
    const t = setup(null)
    expect(await addConfFile(t.deps, t.link)).toEqual({ kind: 'cancelled' })
    expect(t.deps.pick).toHaveBeenCalledWith('WireGuard config')
    expect(t.deps.openFile).not.toHaveBeenCalled()
    expect(t.link.call).not.toHaveBeenCalled()
  })
  test('sends the text and the base name with the long timeout; the path appears nowhere', async () => {
    const t = setup(enc('[Interface]\nPrivateKey = AAAA\n'))
    expect(await addConfFile(t.deps, t.link)).toEqual({ kind: 'done', result: RESULT })
    expect(t.link.call).toHaveBeenCalledTimes(1)
    const [method, params, timeout] = t.link.call.mock.calls[0]
    expect(method).toBe('proxy.add')
    expect(params).toStrictEqual({ text: '[Interface]\nPrivateKey = AAAA\n', name: 'wg-home.conf' })
    expect(timeout).toBe(30_000)
    expect(JSON.stringify(t.link.call.mock.calls)).not.toMatch(/Users|Documents|someone|C:/)
    expect(t.h?.closed).toBe(true)
  })
  test('reads at most 64 KiB + 1 from one handle, in as many reads as it takes', async () => {
    const t = setup(enc('a'.repeat(5000)))
    await addConfFile(t.deps, t.link)
    expect(t.h?.asked[0]).toBe(MAX + 1)
    expect(Math.max(...(t.h?.asked ?? []))).toBe(MAX + 1)
    expect(t.link.call.mock.calls[0][1]).toMatchObject({ text: 'a'.repeat(5000) })
  })
  test('exactly 64 KiB goes; one byte more is a line-0 too_long without a call', async () => {
    const ok = setup(enc('a'.repeat(MAX)))
    expect((await addConfFile(ok.deps, ok.link)).kind).toBe('done')
    expect(ok.link.call).toHaveBeenCalledTimes(1)
    const big = setup(enc('a'.repeat(MAX + 1)))
    expect(await addConfFile(big.deps, big.link)).toEqual({
      kind: 'done',
      result: { added: [], existing: 0, servers: [], subscription: null, rejected: [{ line: 0, code: 'too_long' }], rejectedMore: 0 }
    })
    expect(big.link.call).not.toHaveBeenCalled()
    expect(big.h?.closed).toBe(true)
  })
  test('empty, blank or not UTF-8: a line-0 bad_wireguard without a call', async () => {
    const bad = { kind: 'done', result: { added: [], existing: 0, servers: [], subscription: null, rejected: [{ line: 0, code: 'bad_wireguard' }], rejectedMore: 0 } }
    const files = [new Uint8Array(0), enc('  \r\n'), enc('\ufeff'), Uint8Array.of(0xff, 0xfe, 0x41, 0x00), Uint8Array.of(0x5b, 0xc3, 0x28)]
    for (const bytes of files) {
      const t = setup(bytes)
      expect(await addConfFile(t.deps, t.link)).toEqual(bad)
      expect(t.link.call).not.toHaveBeenCalled()
    }
  })
  test('one leading BOM is stripped', async () => {
    const t = setup(enc('\ufeff[Interface]\n'))
    await addConfFile(t.deps, t.link)
    expect(t.link.call.mock.calls[0][1]).toMatchObject({ text: '[Interface]\n' })
    const two = setup(enc('\ufeff\ufeff[Interface]\n'))
    await addConfFile(two.deps, two.link)
    expect(two.link.call.mock.calls[0][1]).toMatchObject({ text: '\ufeff[Interface]\n' })
  })
  test('a read or open error is failed with no path in the message', async () => {
    const t = setup(new Error(`EACCES: permission denied, open '${PATH}'`))
    const out = await addConfFile(t.deps, t.link)
    expect(out).toEqual({ kind: 'failed', error: { code: 'internal', message: 'file unreadable' } })
    expect(JSON.stringify(out)).not.toContain('Users')
    expect(t.link.call).not.toHaveBeenCalled()
    let closed = false
    const r = setup(enc('x'), {
      openFile: async () => ({
        read: async () => {
          throw new Error(PATH)
        },
        close: async () => {
          closed = true
        }
      })
    })
    expect(await addConfFile(r.deps, r.link)).toEqual({ kind: 'failed', error: { code: 'internal', message: 'file unreadable' } })
    expect(closed).toBe(true)
  })
  test('a failing dialog is a failed outcome too', async () => {
    const t = setup(null, {
      pick: async () => {
        throw new Error('no window')
      }
    })
    expect(await addConfFile(t.deps, t.link)).toMatchObject({ kind: 'failed', error: { code: 'internal' } })
  })
  test('an odd base name is replaced, so the source is still file', async () => {
    for (const base of ['a:b.conf', 'x\u0007.conf', 'я'.repeat(129) + '.conf']) {
      const t = setup(enc('[Interface]'), { baseName: () => base })
      await addConfFile(t.deps, t.link)
      expect(t.link.call.mock.calls[0][1]).toMatchObject({ name: FALLBACK_CONF_NAME })
    }
  })
  test('a helper failure comes back as failed', async () => {
    const t = setup(enc('[Interface]'))
    t.link.call.mockRejectedValueOnce(new Error('boom'))
    expect(await addConfFile(t.deps, t.link)).toMatchObject({ kind: 'failed', error: { code: 'internal', message: 'boom' } })
  })
})

describe('confName', () => {
  test('keeps a fine name, replaces the ones the helper would refuse', () => {
    expect(confName('wg-home.conf')).toBe('wg-home.conf')
    expect(confName('Мой сервер.conf')).toBe('Мой сервер.conf')
    expect(confName('я'.repeat(128))).toBe('я'.repeat(128))
    for (const bad of ['', '.', '..', 'a/b', 'a\\b', 'a:b', 'a\nb', '\u007f', 'я'.repeat(129), 'x\ud800y']) {
      expect(confName(bad), JSON.stringify(bad)).toBe(FALLBACK_CONF_NAME)
    }
  })
})

describe('readUpTo', () => {
  test('stops at the end of the file or at the limit', async () => {
    expect((await readUpTo(handle(enc('abc'), 1), 10)).length).toBe(3)
    expect((await readUpTo(handle(enc('abcdef'), 2), 4)).length).toBe(4)
    expect((await readUpTo(handle(new Uint8Array(0)), 4)).length).toBe(0)
  })
})
