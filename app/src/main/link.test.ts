import { expect, test, vi } from 'vitest'
import { parseState, type Activity, type HelperState } from '../shared/protocol'
import type { LinkStatus } from '../shared/status'
import { HelperLink, type LinkDeps } from './link'
import { DialError } from './pipe'
import { ConnectionClosedError, type Conn } from './rpc'

const HELLO = { protocol: 2, helper: '0.1.0-test', core: '1.14.4' }
const STATE = {
  phase: 'off',
  reason: null,
  allDirect: false,
  defaultLane: 'vpn',
  apps: [],
  addresses: { vpn: null, direct: null },
  connector: 'tailscale',
  tailscale: {
    backend: 'stopped',
    authUrl: null,
    deviceName: 'demo-splitlane',
    exitNodes: [],
    selectedExitNode: null,
    path: null
  },
  proxy: { servers: [], selected: null, subscriptions: [] },
  systemTailscale: { installed: false, exitNodeOn: false, restoreOnDisable: false }
}
const ACTIVITY = { lanes: { vpn: { down: 2048, up: 512 }, direct: { down: 0, up: 0 } }, apps: [] }

type Reply = { result: unknown } | { error: Record<string, unknown> } | null
type Answer = (method: string, params: unknown) => Reply

function helperAnswer(method: string): Reply {
  if (method === 'hello') return { result: HELLO }
  if (method === 'state.get') return { result: STATE }
  if (method === 'activity.subscribe') return { result: {} }
  return null // never answered: the call stays in flight
}

interface Req {
  id: number
  method: string
  params?: unknown
}

/** A helper on the other end of the pipe: answers requests through `answer`. */
class FakeConn implements Conn {
  requests: Req[] = []
  ended = false
  private readonly dataCbs: ((c: Buffer) => void)[] = []
  private readonly closeCbs: ((e?: Error) => void)[] = []
  constructor(private readonly answer: Answer = helperAnswer) {}
  write(text: string): void {
    for (const line of text.split('\n')) {
      if (line === '') continue
      const req = JSON.parse(line) as Req
      this.requests.push(req)
      const reply = this.answer(req.method, req.params)
      if (reply) queueMicrotask(() => this.feed(JSON.stringify({ id: req.id, ...reply }) + '\n'))
    }
  }
  end(): void {
    this.ended = true
  }
  onData(cb: (c: Buffer) => void): void {
    this.dataCbs.push(cb)
  }
  onClose(cb: (e?: Error) => void): void {
    this.closeCbs.push(cb)
  }
  feed(s: string): void {
    for (const cb of this.dataCbs) cb(Buffer.from(s))
  }
  drop(): void {
    for (const cb of this.closeCbs) cb()
  }
  calls(): [string, unknown][] {
    return this.requests.map((r) => [r.method, r.params])
  }
  methods(): string[] {
    return this.requests.map((r) => r.method)
  }
}

const absent = (): DialError => new DialError('absent', 'ENOENT')

function harness() {
  const deps = {
    dial: vi.fn<() => Promise<Conn>>(async () => {
      throw absent()
    }),
    taskRegistered: vi.fn<() => Promise<boolean>>(async () => true),
    runTask: vi.fn<() => Promise<void>>(async () => {}),
    sleep: vi.fn<(ms: number) => Promise<void>>(async () => {}),
    log: vi.fn<(message: string) => void>()
  } satisfies LinkDeps
  const link = new HelperLink(deps)
  const statuses: LinkStatus[] = []
  link.on('status', (s) => statuses.push(s))
  return { deps, link, statuses }
}

/** Lets every pending promise chain run; the link only waits through deps.sleep and the fake pipe. */
async function settle(): Promise<void> {
  for (let i = 0; i < 20; i++) await new Promise<void>((r) => setImmediate(r))
}

test('connects: hello, state.get, subscribe, ready', async () => {
  const t = harness()
  const c = new FakeConn()
  t.deps.dial.mockResolvedValueOnce(c)
  expect(t.link.status()).toEqual({ kind: 'connecting' })
  expect(t.link.state()).toBeNull()
  const states: HelperState[] = []
  t.link.on('state', (s) => states.push(s))

  t.link.start()
  await settle()

  expect(c.calls()).toEqual([
    ['hello', { protocol: 2 }],
    ['state.get', undefined],
    ['activity.subscribe', {}]
  ])
  expect(t.link.status()).toEqual({ kind: 'ready', hello: HELLO })
  expect(t.statuses).toEqual([{ kind: 'ready', hello: HELLO }])
  expect(t.link.state()).toEqual(parseState(STATE))
  expect(states).toEqual([parseState(STATE)])
  expect(t.deps.taskRegistered).not.toHaveBeenCalled()
  expect(t.deps.runTask).not.toHaveBeenCalled()
})

test('setup-needed when there is no pipe and no task', async () => {
  const t = harness()
  t.deps.taskRegistered.mockResolvedValue(false)

  t.link.start()
  await settle()

  expect(t.link.status()).toEqual({ kind: 'setup-needed' })
  expect(t.deps.dial).toHaveBeenCalledTimes(1)
  expect(t.deps.runTask).not.toHaveBeenCalled()
})

test('starts the task when the pipe is absent but the task exists', async () => {
  const t = harness()
  const c = new FakeConn()
  let started = false
  let polls = 0
  t.deps.runTask.mockImplementation(async () => {
    started = true
  })
  t.deps.dial.mockImplementation(async () => {
    if (!started || ++polls < 3) throw absent()
    return c
  })

  t.link.start()
  await settle()

  // a helper that was not running yet is started while the window connects: no «перезапускается»
  expect(t.statuses).toEqual([{ kind: 'ready', hello: HELLO }])
  expect(t.deps.runTask).toHaveBeenCalledTimes(1)
  expect(t.deps.sleep.mock.calls).toEqual([[200], [200], [200]])
  expect(c.methods()).toEqual(['hello', 'state.get', 'activity.subscribe'])
})

test('other-user when the pipe denies access', async () => {
  const t = harness()
  t.deps.dial.mockRejectedValue(new DialError('denied', 'EACCES'))

  t.link.start()
  await settle()

  expect(t.link.status()).toEqual({ kind: 'other-user' })
  expect(t.deps.taskRegistered).not.toHaveBeenCalled()
  expect(t.deps.runTask).not.toHaveBeenCalled()
})

test('helper-older and helper-newer on protocol_mismatch', async () => {
  const cases: [number, LinkStatus][] = [
    [1, { kind: 'helper-older', helperProtocol: 1 }],
    [3, { kind: 'helper-newer', helperProtocol: 3 }]
  ]
  for (const [protocol, expected] of cases) {
    const t = harness()
    const c = new FakeConn((m) =>
      m === 'hello' ? { error: { code: 'protocol_mismatch', message: 'unsupported protocol', protocol } } : null
    )
    t.deps.dial.mockResolvedValue(c)

    t.link.start()
    await settle()

    expect(t.link.status()).toEqual(expected)
    expect(c.calls()).toEqual([['hello', { protocol: 2 }]])
    expect(c.ended).toBe(true)
    expect(t.deps.runTask).not.toHaveBeenCalled()
    await expect(t.link.call('state.get')).rejects.toBeInstanceOf(ConnectionClosedError)
  }
})

test('restarts the task up to five times when the pipe is lost, then unresponsive', async () => {
  const t = harness()
  const c = new FakeConn()
  t.deps.dial.mockResolvedValueOnce(c)
  t.link.start()
  await settle()
  expect(t.link.status().kind).toBe('ready')

  c.drop()
  expect(t.link.status()).toEqual({ kind: 'restarting', attempt: 1 })
  await settle()

  expect(t.statuses.slice(1)).toEqual([
    { kind: 'restarting', attempt: 1 },
    { kind: 'restarting', attempt: 2 },
    { kind: 'restarting', attempt: 3 },
    { kind: 'restarting', attempt: 4 },
    { kind: 'restarting', attempt: 5 },
    { kind: 'unresponsive' }
  ])
  expect(t.deps.runTask).toHaveBeenCalledTimes(5)
  // each attempt waits for the pipe up to 2 s, dialing every 200 ms
  expect(t.deps.sleep).toHaveBeenCalledTimes(50)
  expect(t.deps.sleep.mock.calls.every(([ms]) => ms === 200)).toBe(true)
  expect(t.deps.dial).toHaveBeenCalledTimes(1 + 50)
  // the board keeps the last state under the banner
  expect(t.link.state()).toEqual(parseState(STATE))
})

test('a start without the pipe runs the task while connecting, then unresponsive', async () => {
  const t = harness()
  const held: (() => void)[] = []
  t.deps.sleep.mockImplementation(() => new Promise<void>((r) => held.push(r)))
  t.link.start()
  await settle()
  expect(t.deps.runTask).toHaveBeenCalledTimes(1)
  expect(t.link.status()).toEqual({ kind: 'connecting' })

  t.deps.sleep.mockImplementation(async () => {})
  for (const r of held.splice(0)) r()
  await settle()
  // the same five runs with a wait of up to 2 s each as after a loss of the pipe
  expect(t.deps.runTask).toHaveBeenCalledTimes(5)
  expect(t.deps.sleep).toHaveBeenCalledTimes(50)
  expect(t.statuses).toEqual([{ kind: 'unresponsive' }])

  // «Повторить» connects again the same way
  t.link.retry()
  await settle()
  expect(t.statuses).toEqual([{ kind: 'unresponsive' }, { kind: 'connecting' }, { kind: 'unresponsive' }])
  expect(t.deps.runTask).toHaveBeenCalledTimes(10)
})

test('retry starts over', async () => {
  // from setup-needed (after the install, for example)
  const t = harness()
  t.deps.taskRegistered.mockResolvedValue(false)
  t.link.start()
  await settle()
  expect(t.link.status()).toEqual({ kind: 'setup-needed' })

  const c = new FakeConn()
  t.deps.dial.mockResolvedValueOnce(c)
  t.link.retry()
  expect(t.link.status()).toEqual({ kind: 'connecting' })
  await settle()
  expect(t.statuses).toEqual([{ kind: 'setup-needed' }, { kind: 'connecting' }, { kind: 'ready', hello: HELLO }])

  // a restart loop in progress is cancelled: it neither runs the task again nor touches the status
  const u = harness()
  const c1 = new FakeConn()
  const c2 = new FakeConn()
  const held: (() => void)[] = []
  u.deps.sleep.mockImplementation(() => new Promise<void>((r) => held.push(r)))
  u.deps.dial.mockResolvedValueOnce(c1)
  u.link.start()
  await settle()
  c1.drop()
  await settle()
  expect(u.link.status()).toEqual({ kind: 'restarting', attempt: 1 })
  expect(held).toHaveLength(1)

  u.deps.dial.mockResolvedValueOnce(c2)
  u.link.retry()
  await settle()
  expect(u.link.status()).toEqual({ kind: 'ready', hello: HELLO })
  const dials = u.deps.dial.mock.calls.length

  for (const r of held.splice(0)) r()
  await settle()
  expect(u.link.status()).toEqual({ kind: 'ready', hello: HELLO })
  expect(u.deps.runTask).toHaveBeenCalledTimes(1)
  expect(u.deps.dial).toHaveBeenCalledTimes(dials)
  expect(c2.ended).toBe(false)

  // from ready: the connection is replaced
  const c3 = new FakeConn()
  u.deps.dial.mockResolvedValueOnce(c3)
  u.link.retry()
  expect(c2.ended).toBe(true)
  await settle()
  expect(u.link.status()).toEqual({ kind: 'ready', hello: HELLO })
  expect(c3.methods()).toEqual(['hello', 'state.get', 'activity.subscribe'])
  expect(u.deps.runTask).toHaveBeenCalledTimes(1)
})

test('re-subscribes the activity detail after a reconnect', async () => {
  const t = harness()
  const c1 = new FakeConn()
  const c2 = new FakeConn()
  t.deps.dial.mockResolvedValueOnce(c1).mockResolvedValueOnce(c2)

  // not ready: the key is only remembered
  await t.link.setActivityDetail('C:\\Tools\\Editor\\')
  expect(t.deps.dial).not.toHaveBeenCalled()

  t.link.start()
  await settle()
  expect(c1.calls()[2]).toEqual(['activity.subscribe', { detailFor: 'C:\\Tools\\Editor\\' }])

  await t.link.setActivityDetail(null)
  expect(c1.calls()[3]).toEqual(['activity.subscribe', {}])
  await t.link.setActivityDetail('b6f3c1de-8a41-4e5f-9a7c-2d0e7f1a9b35')
  expect(c1.calls()[4]).toEqual(['activity.subscribe', { detailFor: 'b6f3c1de-8a41-4e5f-9a7c-2d0e7f1a9b35' }])

  c1.drop()
  await settle()
  expect(t.link.status()).toEqual({ kind: 'ready', hello: HELLO })
  expect(c2.calls()).toEqual([
    ['hello', { protocol: 2 }],
    ['state.get', undefined],
    ['activity.subscribe', { detailFor: 'b6f3c1de-8a41-4e5f-9a7c-2d0e7f1a9b35' }]
  ])
})

test('a detail chosen during the handshake is sent before ready', async () => {
  const t = harness()
  let hold = true
  const c = new FakeConn((m) => (m === 'activity.subscribe' && hold ? null : helperAnswer(m)))
  t.deps.dial.mockResolvedValueOnce(c)
  t.link.start()
  await settle()
  expect(c.calls()[2]).toEqual(['activity.subscribe', {}])
  expect(t.link.status()).toEqual({ kind: 'connecting' })

  await t.link.setActivityDetail('C:\\Tools\\Editor\\')
  // over the helper's limit of 1024 bytes: refused, the remembered key stays
  await expect(t.link.setActivityDetail('Ж'.repeat(513))).rejects.toThrow('1024')
  expect(c.requests).toHaveLength(3)

  hold = false
  c.feed(JSON.stringify({ id: c.requests[2].id, result: {} }) + '\n')
  await settle()
  expect(c.calls().slice(3)).toEqual([['activity.subscribe', { detailFor: 'C:\\Tools\\Editor\\' }]])
  expect(t.link.status()).toEqual({ kind: 'ready', hello: HELLO })
})

test('fails pending calls when the pipe closes', async () => {
  const t = harness()
  const c = new FakeConn()
  t.deps.dial.mockResolvedValueOnce(c)
  t.link.start()
  await settle()

  const p = t.link.call('lanes.set', { apps: [], defaultLane: 'direct' })
  expect(c.calls()[3]).toEqual(['lanes.set', { apps: [], defaultLane: 'direct' }])
  c.drop()
  await expect(p).rejects.toBeInstanceOf(ConnectionClosedError)
  expect(t.statuses[1]).toEqual({ kind: 'restarting', attempt: 1 })

  // without a link a call fails at once and nothing is written
  await settle()
  const before = c.requests.length
  await expect(t.link.call('state.get')).rejects.toBeInstanceOf(ConnectionClosedError)
  expect(c.requests).toHaveLength(before)
})

test('forwards state and activity events', async () => {
  const t = harness()
  const c = new FakeConn()
  t.deps.dial.mockResolvedValueOnce(c)
  t.link.start()
  await settle()
  const states: HelperState[] = []
  const acts: Activity[] = []
  t.link.on('state', () => {
    throw new Error('a broken listener')
  })
  t.link.on('state', (s) => states.push(s))
  const off = t.link.on('activity', (a) => acts.push(a))

  const next = { ...STATE, phase: 'starting' }
  c.feed(JSON.stringify({ event: 'state', data: next }) + '\n')
  expect(states).toEqual([parseState(next)])
  expect(t.link.state()).toEqual(parseState(next))

  // a malformed state, an unknown event and a throwing listener do not stop the rest of the chunk
  c.feed(
    '{"event":"state","data":{"phase":"bogus"}}\n' +
      '{"event":"mystery","data":1}\n' +
      JSON.stringify({ event: 'activity', data: ACTIVITY }) +
      '\n'
  )
  expect(acts).toEqual([ACTIVITY])
  expect(states).toHaveLength(1)
  expect(t.link.state()).toEqual(parseState(next))
  expect(t.deps.log.mock.calls.some(([m]) => m.includes('state'))).toBe(true)

  off()
  c.feed(JSON.stringify({ event: 'activity', data: ACTIVITY }) + '\n')
  expect(acts).toHaveLength(1)
})

test('an unusual dial error follows the rules of an absent pipe', async () => {
  const t = harness()
  t.deps.taskRegistered.mockResolvedValue(false)
  t.deps.dial.mockRejectedValue(new DialError('other', 'ETIMEDOUT'))
  t.link.start()
  await settle()
  expect(t.link.status()).toEqual({ kind: 'setup-needed' })

  const u = harness()
  const c = new FakeConn()
  u.deps.dial.mockRejectedValueOnce(new Error('something odd')).mockResolvedValueOnce(c)
  u.link.start()
  await settle()
  expect(u.statuses).toEqual([{ kind: 'ready', hello: HELLO }])
  expect(u.deps.runTask).toHaveBeenCalledTimes(1)
})

test('a failed handshake restarts the task', async () => {
  const t = harness()
  const broken = new FakeConn((m) => (m === 'state.get' ? { result: { phase: 'bogus' } } : helperAnswer(m)))
  const good = new FakeConn()
  t.deps.dial.mockResolvedValueOnce(broken).mockResolvedValueOnce(good)
  t.link.start()
  await settle()
  expect(broken.ended).toBe(true)
  expect(t.statuses).toEqual([{ kind: 'ready', hello: HELLO }])
  expect(t.deps.runTask).toHaveBeenCalledTimes(1)
})

test('a failing runTask counts as a failed attempt', async () => {
  const t = harness()
  t.deps.runTask.mockRejectedValue(new Error('schtasks /run failed with code 1'))
  t.link.start()
  await settle()
  expect(t.link.status()).toEqual({ kind: 'unresponsive' })
  expect(t.statuses).toEqual([{ kind: 'unresponsive' }])
  expect(t.deps.runTask).toHaveBeenCalledTimes(5)
  expect(t.deps.log.mock.calls.filter(([m]) => m.includes('schtasks /run failed')).length).toBe(5)
})

test('shutdown closes without a restart', async () => {
  // the helper answers helper.shutdown, then exits: the pipe closing is not a loss
  const t = harness()
  const c = new FakeConn((m) => (m === 'helper.shutdown' ? { result: {} } : helperAnswer(m)))
  t.deps.dial.mockResolvedValueOnce(c)
  t.link.start()
  await settle()
  expect(t.link.status().kind).toBe('ready')

  await t.link.shutdown()
  expect(c.calls().slice(3)).toEqual([['helper.shutdown', undefined]])
  expect(c.ended).toBe(true)
  c.drop()
  await settle()
  expect(t.deps.taskRegistered).not.toHaveBeenCalled()
  expect(t.deps.runTask).not.toHaveBeenCalled()
  expect(t.deps.dial).toHaveBeenCalledTimes(1)
  expect(t.statuses).toEqual([{ kind: 'ready', hello: HELLO }])
  await expect(t.link.call('state.get')).rejects.toBeInstanceOf(ConnectionClosedError)

  // the helper closes the pipe before its answer arrives: no restart either
  const u = harness()
  const d = new FakeConn((m) => (m === 'helper.shutdown' ? null : helperAnswer(m)))
  u.deps.dial.mockResolvedValueOnce(d)
  u.link.start()
  await settle()
  const p = u.link.shutdown()
  expect(d.calls().slice(3)).toEqual([['helper.shutdown', undefined]])
  d.drop()
  await p
  await settle()
  expect(u.deps.runTask).not.toHaveBeenCalled()
  expect(u.statuses).toEqual([{ kind: 'ready', hello: HELLO }])

  // no answer in time: shutdown resolves anyway and closes the pipe
  const v = harness()
  const e = new FakeConn((m) => (m === 'helper.shutdown' ? null : helperAnswer(m)))
  v.deps.dial.mockResolvedValueOnce(e)
  v.link.start()
  await settle()
  await v.link.shutdown(20)
  expect(e.ended).toBe(true)
  e.drop()
  await settle()
  expect(v.deps.runTask).not.toHaveBeenCalled()
  expect(v.deps.log.mock.calls.some(([m]) => m.includes('helper.shutdown'))).toBe(true)

  // not ready: nothing is sent and a restart loop in progress stops
  const w = harness()
  const held: (() => void)[] = []
  w.deps.sleep.mockImplementation(() => new Promise<void>((r) => held.push(r)))
  w.link.start()
  await settle()
  expect(w.deps.runTask).toHaveBeenCalledTimes(1)
  expect(w.link.status()).toEqual({ kind: 'connecting' })
  await w.link.shutdown()
  for (const r of held.splice(0)) r()
  await settle()
  expect(w.deps.runTask).toHaveBeenCalledTimes(1)
  expect(w.deps.dial).toHaveBeenCalledTimes(1)
  expect(w.link.status()).toEqual({ kind: 'connecting' })
})

test('stop cancels the link and closes the connection', async () => {
  const t = harness()
  const c = new FakeConn()
  t.deps.dial.mockResolvedValueOnce(c)
  t.link.start()
  await settle()
  t.link.stop()
  expect(c.ended).toBe(true)
  c.drop()
  await settle()
  expect(t.deps.runTask).not.toHaveBeenCalled()
  await expect(t.link.call('state.get')).rejects.toBeInstanceOf(ConnectionClosedError)
})
