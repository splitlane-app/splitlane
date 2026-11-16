import { describe, expect, test, vi } from 'vitest'
import type { VpsEvent } from '../shared/ipc'
import { HelperCallError } from './rpc'
import type { ConfHandle } from './confFile'
import { VpsSetup, type VpsChild, type VpsDeps } from './vpsSetup'

const SRV_A = '0123456789abcdef'
const SRV_B = 'fedcba9876543210'
const SUB = 'sub-0123456789abcdef'
const PASSWORD = 'Pw-t3st\'x"y z'
const SUB_URL = 'https://198-51-100-7.sslip.io:8443/tok3n/sub'
const LINKS = 'vless://aaa@198.51.100.7:443#one\nvless://bbb@198.51.100.7:8443#two'
const EXE = String.raw`C:\Program Files\SplitLane\helper\splitlane-helper.exe`

function fakeChild() {
  const c = {
    writes: [] as string[],
    ended: false,
    killed: false,
    out: (_b: Uint8Array): void => {},
    close: (_code: number | null): void => {},
    fail: (_e: Error): void => {},
    child: undefined as unknown as VpsChild,
    line(o: unknown, end = '\n') {
      c.out(new TextEncoder().encode(JSON.stringify(o) + end))
    }
  }
  c.child = {
    stdin: {
      write: async (b) => {
        c.writes.push(new TextDecoder().decode(b))
      },
      end: () => {
        c.ended = true
      }
    },
    onStdout: (cb) => {
      c.out = cb
    },
    onClose: (cb) => {
      c.close = cb
    },
    onError: (cb) => {
      c.fail = cb
    },
    kill: () => {
      c.killed = true
    }
  }
  return c
}

const result = (over: object = {}) => ({ added: [SRV_A], existing: 0, servers: [SRV_A], subscription: null, rejected: [], rejectedMore: 0, ...over })

function keyHandle(bytes: Uint8Array): ConfHandle {
  let pos = 0
  return {
    read: async (buf, length) => {
      const n = Math.min(length, 700, bytes.length - pos)
      buf.set(bytes.subarray(pos, pos + n))
      pos += n
      return { bytesRead: n }
    },
    close: async () => {}
  }
}

function setup(over: Partial<VpsDeps> = {}, file: Uint8Array | null = null) {
  const c = fakeChild()
  const events: VpsEvent[] = []
  const timers: { fn: () => void; ms: number; cancelled: boolean }[] = []
  const calls: [string, object | undefined][] = []
  const link = {
    call: vi.fn(async (m: string, p?: object, _t?: number): Promise<unknown> => {
      calls.push([m, p])
      return m === 'proxy.add' ? result() : {}
    })
  }
  const logs: string[] = []
  const deps: VpsDeps = {
    helperExe: EXE,
    spawn: vi.fn((_f: string, _a: string[], _o: { stdio: 'pipe'; windowsHide: true }) => c.child),
    phase: () => 'off',
    link,
    emit: (e) => events.push(e),
    pick: vi.fn(async (_n: string) => (file === null ? null : String.raw`C:\Users\someone\.ssh\id_ed25519`)),
    openFile: vi.fn(async (_p: string) => keyHandle(file ?? new Uint8Array())),
    filterName: () => 'SSH key',
    baseName: (p) => p.slice(p.lastIndexOf('\\') + 1),
    setTimer: (fn, ms) => {
      const t = { fn, ms, cancelled: false }
      timers.push(t)
      return () => {
        t.cancelled = true
      }
    },
    log: (m) => logs.push(m),
    ...over
  }
  const vs = new VpsSetup(deps)
  const started = async (p: Promise<unknown>) => {
    void p
    await vi.waitFor(() => expect(c.writes.length).toBe(1))
  }
  return { vs, c, deps, events, timers, calls, link, logs, started }
}

const req = { address: ' 198.51.100.7:2222 ', user: 'root', password: PASSWORD, useKey: false }
const DONE = { done: { subscription: SUB_URL, links: LINKS, warnings: ['sub_unreachable'] } }

describe('a successful setup', () => {
  test('writes one JSON line of host, port, user, password; no secret in spawn arguments', async () => {
    const t = setup()
    const p = t.vs.setup({ ...req })
    await t.started(p)
    expect(t.deps.spawn).toHaveBeenCalledWith(EXE, ['server-setup'], { stdio: 'pipe', windowsHide: true })
    const spawned = JSON.stringify((t.deps.spawn as ReturnType<typeof vi.fn>).mock.calls)
    expect(spawned).not.toContain('Pw-t3st')
    expect(spawned).not.toContain('198.51.100.7')
    expect(t.c.writes[0].endsWith('\n')).toBe(true)
    expect(t.c.writes[0].indexOf('\n')).toBe(t.c.writes[0].length - 1)
    expect(JSON.parse(t.c.writes[0])).toStrictEqual({ host: '198.51.100.7', port: 2222, user: 'root', password: PASSWORD })
    expect(t.c.ended).toBe(false) // stdin stays open
    t.c.line(DONE)
    t.c.close(0)
    await p
  })

  test('events relay in order; done adds the subscription address, then selects the first server', async () => {
    const t = setup()
    t.link.call.mockImplementation(async (m: string, p?: object) => {
      t.calls.push([m, p])
      return m === 'proxy.add' ? result({ added: [SRV_A], servers: [SRV_A, SRV_B], subscription: SUB }) : {}
    })
    const p = t.vs.setup({ ...req })
    await t.started(p)
    t.c.line({ step: 'connect', state: 'start' })
    t.c.line({ step: 'connect', state: 'done', detail: 'ok' })
    t.c.line({ step: 'install', state: 'start' })
    t.c.line({ step: 'install', state: 'done' })
    t.c.line({ step: 'harden', state: 'warn', detail: 'harden_failed' })
    t.c.line({ step: 'subscription', state: 'skip' })
    t.c.line(DONE)
    t.c.close(0)
    expect(await p).toEqual({ kind: 'done', servers: 2, subscriptionId: SUB, warnings: ['sub_unreachable'] })
    expect(t.events).toEqual([
      { step: 'connect', state: 'start' },
      { step: 'connect', state: 'done', detail: 'ok' },
      { step: 'install', state: 'start' },
      { step: 'install', state: 'done' },
      { step: 'harden', state: 'warn', detail: 'harden_failed' },
      { step: 'subscription', state: 'skip' },
      { step: 'add', state: 'start' },
      { step: 'add', state: 'done' }
    ])
    expect(t.calls).toEqual([
      ['proxy.add', { text: SUB_URL }],
      ['proxy.select', { id: SRV_A }]
    ])
  })

  test('without a subscription the links go to proxy.add', async () => {
    const t = setup()
    const p = t.vs.setup({ ...req })
    await t.started(p)
    t.c.line({ done: { subscription: null, links: LINKS, warnings: [] } })
    t.c.close(0)
    expect(await p).toEqual({ kind: 'done', servers: 1, subscriptionId: null, warnings: [] })
    expect(t.calls[0]).toEqual(['proxy.add', { text: LINKS }])
  })

  test('every server was there already (added: [], servers: [a, b]): a is selected', async () => {
    const t = setup()
    t.link.call.mockImplementation(async (m: string, p?: object) => {
      t.calls.push([m, p])
      return m === 'proxy.add' ? result({ added: [], existing: 2, servers: [SRV_A, SRV_B] }) : {}
    })
    const p = t.vs.setup({ ...req })
    await t.started(p)
    t.c.line({ done: { subscription: null, links: LINKS, warnings: [] } })
    t.c.close(0)
    expect(await p).toMatchObject({ kind: 'done', servers: 2 })
    expect(t.calls[1]).toEqual(['proxy.select', { id: SRV_A }])
  })

  test('a final line without a newline at the end still counts', async () => {
    const t = setup()
    const p = t.vs.setup({ ...req })
    await t.started(p)
    t.c.line({ done: { subscription: null, links: LINKS, warnings: [] } }, '')
    t.c.close(0)
    expect((await p).kind).toBe('done')
  })

  test('a line split across chunks is joined', async () => {
    const t = setup()
    const p = t.vs.setup({ ...req })
    await t.started(p)
    const raw = new TextEncoder().encode(JSON.stringify({ step: 'system', state: 'start' }) + '\n')
    t.c.out(raw.subarray(0, 7))
    t.c.out(raw.subarray(7))
    t.c.line({ done: { subscription: null, links: LINKS, warnings: [] } })
    t.c.close(0)
    await p
    expect(t.events[0]).toEqual({ step: 'system', state: 'start' })
  })
})

describe('add failures', () => {
  test('proxy.add refused as a whole (servers empty): add_failed, an add failed event', async () => {
    const t = setup()
    t.link.call.mockResolvedValue(result({ added: [], servers: [] }))
    const p = t.vs.setup({ ...req })
    await t.started(p)
    t.c.line({ done: { subscription: SUB_URL, links: LINKS, warnings: [] } })
    t.c.close(0)
    expect(await p).toMatchObject({ kind: 'failed', code: 'add_failed' })
    expect(t.events.at(-1)).toMatchObject({ step: 'add', state: 'failed' })
    expect(t.link.call).toHaveBeenCalledTimes(1) // no select
  })

  test('proxy.add error: the code of the channel in detail', async () => {
    const t = setup()
    t.link.call.mockRejectedValue(new HelperCallError({ code: 'fetch_timeout', message: 'x' }))
    const p = t.vs.setup({ ...req })
    await t.started(p)
    t.c.line({ done: { subscription: SUB_URL, links: LINKS, warnings: [] } })
    t.c.close(0)
    expect(await p).toEqual({ kind: 'failed', code: 'add_failed', detail: 'fetch_timeout' })
  })

  test('proxy.select error is add_failed too', async () => {
    const t = setup()
    t.link.call.mockImplementation(async (m: string) => {
      if (m === 'proxy.add') return result()
      throw new HelperCallError({ code: 'wrong_phase', message: 'x' })
    })
    const p = t.vs.setup({ ...req })
    await t.started(p)
    t.c.line({ done: { subscription: null, links: LINKS, warnings: [] } })
    t.c.close(0)
    expect(await p).toEqual({ kind: 'failed', code: 'add_failed', detail: 'wrong_phase' })
    expect(t.events.at(-1)).toMatchObject({ step: 'add', state: 'failed' })
  })
})

describe('a failed subprocess', () => {
  test('failed: the code and detail, and a failed event for the step that was running', async () => {
    const t = setup()
    const p = t.vs.setup({ ...req })
    await t.started(p)
    t.c.line({ step: 'connect', state: 'start' })
    t.c.line({ step: 'connect', state: 'done' })
    t.c.line({ step: 'install', state: 'start' })
    t.c.line({ failed: { code: 'ports_busy', detail: '443' } })
    t.c.close(1)
    expect(await p).toEqual({ kind: 'failed', code: 'ports_busy', detail: '443' })
    expect(t.events.at(-1)).toEqual({ step: 'install', state: 'failed', detail: 'ports_busy' })
    expect(t.link.call).not.toHaveBeenCalled()
  })

  test('failed before any step is a failed connect', async () => {
    const t = setup()
    const p = t.vs.setup({ ...req })
    await t.started(p)
    t.c.line({ failed: { code: 'bad_input', detail: 'host' } })
    t.c.close(1)
    expect(await p).toEqual({ kind: 'failed', code: 'bad_input', detail: 'host' })
    expect(t.events).toEqual([{ step: 'connect', state: 'failed', detail: 'bad_input' }])
  })

  test('the process ends without a final line: kit_failed', async () => {
    const t = setup()
    const p = t.vs.setup({ ...req })
    await t.started(p)
    t.c.line({ step: 'system', state: 'start' })
    t.c.out(new TextEncoder().encode('{"done":{"subs')) // cut
    t.c.close(3)
    expect(await p).toMatchObject({ kind: 'failed', code: 'kit_failed' })
    expect(t.events.at(-1)).toMatchObject({ step: 'system', state: 'failed' })
  })

  test('a process that cannot start: kit_failed, no write, not busy afterwards', async () => {
    const t = setup({
      spawn: () => {
        throw new Error('ENOENT')
      }
    })
    expect(await t.vs.setup({ ...req })).toMatchObject({ kind: 'failed', code: 'kit_failed' })
    expect(t.c.writes).toEqual([])
    expect(await t.vs.setup({ ...req })).toMatchObject({ code: 'kit_failed' })
  })

  test('an error event of the child: kit_failed', async () => {
    const t = setup()
    const p = t.vs.setup({ ...req })
    await t.started(p)
    t.c.fail(new Error('spawn EPERM'))
    t.c.close(null)
    expect(await p).toMatchObject({ kind: 'failed', code: 'kit_failed' })
  })

  test('code cancelled from the subcommand is a cancelled outcome without a failed event', async () => {
    const t = setup()
    const p = t.vs.setup({ ...req })
    await t.started(p)
    t.c.line({ step: 'install', state: 'start' })
    t.c.line({ failed: { code: 'cancelled' } })
    t.c.close(1)
    expect(await p).toEqual({ kind: 'cancelled' })
    expect(t.events).toEqual([{ step: 'install', state: 'start' }])
  })
})

describe('cancel, busy, phase', () => {
  test('vpsCancel closes stdin; after 5 s the process is killed', async () => {
    const t = setup()
    const p = t.vs.setup({ ...req })
    await t.started(p)
    await t.vs.cancel()
    expect(t.c.ended).toBe(true)
    expect(t.c.killed).toBe(false)
    expect(t.timers).toHaveLength(1)
    expect(t.timers[0].ms).toBe(5000)
    t.timers[0].fn()
    expect(t.c.killed).toBe(true)
    t.c.close(null)
    expect(await p).toEqual({ kind: 'cancelled' })
  })

  test('the process leaving on its own after the cancel clears the timer', async () => {
    const t = setup()
    const p = t.vs.setup({ ...req })
    await t.started(p)
    await t.vs.cancel()
    t.c.line({ failed: { code: 'cancelled' } })
    t.c.close(1)
    expect(await p).toEqual({ kind: 'cancelled' })
    expect(t.timers[0].cancelled).toBe(true)
  })

  test('the quit does the same as the cancel; nothing running is nothing to do', async () => {
    const idle = setup()
    await idle.vs.cancel()
    idle.vs.shutdown()
    expect(idle.timers).toEqual([])
    const t = setup()
    const p = t.vs.setup({ ...req })
    await t.started(p)
    t.vs.shutdown()
    expect(t.c.ended).toBe(true)
    expect(t.timers[0].ms).toBe(5000)
    t.c.close(null)
    await p
  })

  test('a second setup while one runs is busy; afterwards it can run again', async () => {
    const t = setup()
    const p = t.vs.setup({ ...req })
    await t.started(p)
    expect(await t.vs.setup({ ...req })).toEqual({ kind: 'failed', code: 'busy' })
    expect(t.deps.spawn).toHaveBeenCalledTimes(1)
    t.c.line({ failed: { code: 'ssh_auth' } })
    t.c.close(1)
    await p
    expect(await t.vs.setup({ ...req, address: 'bad address' })).toMatchObject({ code: 'bad_input' })
  })

  test('only off and error start a setup; others are routing_on without a process', async () => {
    for (const phase of ['on', 'starting', 'degraded', 'blocked', 'stopping', null]) {
      const t = setup({ phase: () => phase })
      expect(await t.vs.setup({ ...req })).toEqual({ kind: 'failed', code: 'routing_on' })
      expect(t.deps.spawn).not.toHaveBeenCalled()
    }
    for (const phase of ['off', 'error']) {
      const t = setup({ phase: () => phase })
      const p = t.vs.setup({ ...req })
      await t.started(p)
      t.c.line({ failed: { code: 'ssh_auth' } })
      t.c.close(1)
      await p
    }
  })
})

describe('checks before the process', () => {
  test.each([
    ['address', { ...req, address: 'vps.example.com:99999' }],
    ['empty address', { ...req, address: '' }],
    ['user', { ...req, user: 'Root' }],
    ['no password', { ...req, password: undefined }],
    ['empty password', { ...req, password: '' }],
    ['key wanted, none picked', { ...req, password: undefined, useKey: true }],
    ['extra field', { ...req, extra: 1 }],
    ['not an object', 'x'],
    ['wrong type', { ...req, useKey: 'no' }],
    ['password with a newline', { ...req, password: 'a\nb' }],
    ['a long password', { ...req, password: 'a'.repeat(1025) }]
  ])('%s is bad_input and no process', async (_n, r) => {
    const t = setup()
    expect(await t.vs.setup(r)).toMatchObject({ kind: 'failed', code: 'bad_input' })
    expect(t.deps.spawn).not.toHaveBeenCalled()
  })

  test('IPv6 goes as a bare host', async () => {
    const t = setup()
    const p = t.vs.setup({ ...req, address: '[2001:db8::7]:2200' })
    await t.started(p)
    expect(JSON.parse(t.c.writes[0])).toMatchObject({ host: '2001:db8::7', port: 2200 })
    t.c.close(1)
    await p
  })

  test('the request object is cleared after the write', async () => {
    const t = setup()
    const r = { ...req }
    const p = t.vs.setup(r)
    await t.started(p)
    expect(r.password).toBeUndefined()
    t.c.close(1)
    await p
  })
})

describe('the key', () => {
  const KEY = '-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\n-----END OPENSSH PRIVATE KEY-----\n'
  const enc = (s: string): Uint8Array => new TextEncoder().encode(s)
  const keyed = { address: 'vps.example.com', user: 'root', useKey: true }

  test('picked: only the base name returns; the key goes to the subprocess with the passphrase; forgotten after', async () => {
    const t = setup({}, enc(KEY))
    expect(await t.vs.pickKey()).toEqual({ name: 'id_ed25519' })
    expect(t.deps.pick).toHaveBeenCalledWith('SSH key')
    const p = t.vs.setup({ ...keyed, keyPassphrase: 'ph-r4se' })
    await t.started(p)
    expect(JSON.parse(t.c.writes[0])).toStrictEqual({ host: 'vps.example.com', port: 22, user: 'root', key: KEY, passphrase: 'ph-r4se' })
    t.c.line({ failed: { code: 'ssh_key' } })
    t.c.close(1)
    await p
    expect(await t.vs.setup({ ...keyed })).toMatchObject({ code: 'bad_input' })
  })

  test('exactly 16 KiB is taken; one byte more is too_large', async () => {
    const ok = setup({}, enc('a'.repeat(16384)))
    expect(await ok.vs.pickKey()).toEqual({ name: 'id_ed25519' })
    const big = setup({}, enc('a'.repeat(16385)))
    expect(await big.vs.pickKey()).toEqual({ error: 'too_large' })
  })

  test('not UTF-8 or empty is not_text', async () => {
    const bin = setup({}, new Uint8Array([0x80, 0xff, 0x00, 0x41]))
    expect(await bin.vs.pickKey()).toEqual({ error: 'not_text' })
    const empty = setup({}, new Uint8Array(0))
    expect(await empty.vs.pickKey()).toEqual({ error: 'not_text' })
  })

  test('a cancelled dialog is null; an unreadable file is not_text', async () => {
    expect(await setup().vs.pickKey()).toBeNull()
    const t = setup(
      {
        openFile: async () => {
          throw new Error('EACCES')
        }
      },
      enc(KEY)
    )
    expect(await t.vs.pickKey()).toEqual({ error: 'not_text' })
  })

  test('forgetKey and windowClosed drop it', async () => {
    for (const drop of ['forgetKey', 'windowClosed'] as const) {
      const t = setup({}, enc(KEY))
      await t.vs.pickKey()
      t.vs[drop]()
      expect(await t.vs.setup({ ...keyed })).toMatchObject({ code: 'bad_input' })
    }
  })

  test('a cancelled second pick keeps the first key', async () => {
    const t = setup({}, enc(KEY))
    await t.vs.pickKey()
    ;(t.deps.pick as ReturnType<typeof vi.fn>).mockResolvedValueOnce(null)
    expect(await t.vs.pickKey()).toBeNull()
    const p = t.vs.setup({ ...keyed })
    await t.started(p)
    expect(JSON.parse(t.c.writes[0]).key).toBe(KEY)
    t.c.close(1)
    await p
  })
})

describe('no secrets in the logs', () => {
  test('nothing of the request, the password, the links or the subscription is logged', async () => {
    const t = setup()
    t.link.call.mockRejectedValue(new HelperCallError({ code: 'fetch_failed', message: SUB_URL }))
    const p = t.vs.setup({ ...req })
    await t.started(p)
    t.c.line({ step: 'connect', state: 'done', detail: 'x' })
    t.c.line(DONE)
    t.c.close(0)
    await p
    const all = t.logs.join('\n')
    for (const s of ['Pw-t3st', '198.51.100.7', 'sslip.io', 'tok3n', 'vless://', 'root']) expect(all).not.toContain(s)
  })
})
