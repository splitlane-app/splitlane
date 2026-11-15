import { describe, expect, test } from 'vitest'
import {
  ERROR_CODES,
  LIMITS,
  PROTOCOL_VERSION,
  PROXY_REJECT_CODES,
  packageValue,
  parseActivity,
  parseProxyAddResult,
  parseState,
  utf8Bytes
} from './protocol'

test('packageValue matches the helper', () => {
  const ok = String.raw`C:\Program Files\WindowsApps\Microsoft.WindowsTerminal_1.21.3231.0_x64__8wekyb3d8bbwe\WindowsTerminal.exe`
  expect(packageValue(ok)).toBe('Microsoft.WindowsTerminal_8wekyb3d8bbwe/WindowsTerminal.exe')
  expect(packageValue(String.raw`C:\Program Files\windowsapps\Microsoft.WindowsTerminal_1.21.3231.0_x64__8wekyb3d8bbwe\sub\WindowsTerminal.exe`)).toBe(
    'Microsoft.WindowsTerminal_8wekyb3d8bbwe/WindowsTerminal.exe'
  )
  // empty ResourceId is still five fields
  expect(packageValue(String.raw`C:\Program Files\WindowsApps\Contoso.App_1.0.0.0_x64__pub123\App.exe`)).toBe('Contoso.App_pub123/App.exe')
  // four fields
  expect(packageValue(String.raw`C:\Program Files\WindowsApps\Contoso.App_1.0.0.0_x64_pub123\App.exe`)).toBeNull()
  // empty first or fifth field
  expect(packageValue(String.raw`C:\Program Files\WindowsApps\_1.0.0.0_x64__pub123\App.exe`)).toBeNull()
  expect(packageValue(String.raw`C:\Program Files\WindowsApps\Contoso.App_1.0.0.0_x64__\App.exe`)).toBeNull()
  // no file after the folder
  expect(packageValue(String.raw`C:\Program Files\WindowsApps\Microsoft.WindowsTerminal_1.21.3231.0_x64__8wekyb3d8bbwe`)).toBeNull()
  expect(packageValue(String.raw`C:\Tools\a.exe`)).toBeNull()
})

test('utf8Bytes counts bytes', () => {
  expect(utf8Bytes('abc')).toBe(3)
  expect(utf8Bytes('Программы')).toBe(18)
  expect(utf8Bytes('')).toBe(0)
  expect(LIMITS.maxApps).toBe(500)
})

const emptyState = (): Record<string, unknown> => ({
  phase: 'off',
  reason: null,
  allDirect: false,
  defaultLane: 'vpn',
  apps: [],
  addresses: { vpn: null, direct: null },
  connector: 'tailscale',
  tailscale: { backend: 'stopped', authUrl: null, deviceName: '', exitNodes: [], selectedExitNode: null, path: null },
  proxy: { servers: [], selected: null, subscriptions: [] },
  systemTailscale: { installed: false, exitNodeOn: false, restoreOnDisable: false }
})

describe('parseState', () => {
  test("accepts the helper's empty state", () => {
    const s = parseState(JSON.parse(JSON.stringify(emptyState())))
    expect(s.phase).toBe('off')
    expect(s.reason).toBeNull()
    expect(s.apps).toEqual([])
    expect(s.addresses).toEqual({ vpn: null, direct: null })
  })
  test('accepts a full state', () => {
    const s = emptyState()
    s.phase = 'degraded'
    s.reason = { code: 'restore_pending', params: { command: 'cmd', error: 'e' } }
    s.apps = [{ id: 'i', name: 'n', exe: String.raw`C:\a.exe`, matcher: { kind: 'file', value: String.raw`C:\a.exe` }, lane: 'direct' }]
    s.addresses = { vpn: '198.51.100.2', direct: '192.0.2.5' }
    expect(parseState(s).apps[0].lane).toBe('direct')
  })
  test('rejects an unknown phase', () => {
    const s = emptyState()
    s.phase = 'weird'
    expect(() => parseState(s)).toThrow(/phase/)
  })
  test('rejects bad lane, null apps, bad matcher kind', () => {
    expect(() => parseState({ ...emptyState(), defaultLane: 'x' })).toThrow(/defaultLane/)
    expect(() => parseState({ ...emptyState(), apps: null })).toThrow(/apps/)
    const s = emptyState()
    s.apps = [{ id: 'i', name: 'n', exe: 'e', matcher: { kind: 'zzz', value: '' }, lane: 'vpn' }]
    expect(() => parseState(s)).toThrow(/kind/)
    expect(() => parseState(null)).toThrow()
  })
})

test('parseActivity accepts a summary with detail and without it', () => {
  const base = {
    lanes: { vpn: { down: 1, up: 2 }, direct: { down: 3, up: 4 } },
    apps: [{ key: 'k', kind: 'app', name: 'n', exe: 'e', ruleId: null, lane: 'vpn', down: 1, up: 1, connections: 2 }]
  }
  expect(parseActivity(base).detail).toBeUndefined()
  const withDetail = { ...base, detail: { key: 'k', exes: ['e'], connections: [{ host: 'h.example', lane: 'direct', down: 1, up: 2 }] } }
  expect(parseActivity(withDetail).detail?.connections[0].host).toBe('h.example')
  expect(() => parseActivity({ ...base, apps: [{ ...base.apps[0], down: 'x' }] })).toThrow(/down/)
  expect(() => parseActivity({ ...base, apps: null })).toThrow(/apps/)
})

// ---- protocol v2: connector and the proxy block ----

const SERVER_A = '0123456789abcdef'
const SERVER_B = 'fedcba9876543210'
const SUB = 'sub-0123456789abcdef'

const proxyState = (proxy: unknown, over: Record<string, unknown> = {}): Record<string, unknown> => ({
  ...emptyState(),
  connector: 'proxy',
  proxy,
  ...over
})
const server = (id: string, over: Record<string, unknown> = {}): Record<string, unknown> => ({
  id,
  name: 'edge.example.com',
  protocol: 'vless',
  source: 'manual',
  ...over
})
const sub = (over: Record<string, unknown> = {}): Record<string, unknown> => ({
  id: SUB,
  name: 'sub.example.com',
  updatedAt: '2026-11-16T10:00:00Z',
  count: 2,
  error: null,
  ...over
})

describe('parseState v2', () => {
  test('the protocol is 2 and no_server is a reason', () => {
    expect(PROTOCOL_VERSION).toBe(2)
    const s = emptyState()
    s.reason = { code: 'no_server', params: {} }
    expect(parseState(s).reason?.code).toBe('no_server')
  })
  test('accepts both connectors', () => {
    expect(parseState(emptyState()).connector).toBe('tailscale')
    const s = parseState(
      proxyState({
        servers: [server(SERVER_A), server(SERVER_B, { source: SUB, protocol: 'wireguard' }), server('0000000000000001', { source: 'file' })],
        selected: SERVER_B,
        subscriptions: [sub(), sub({ id: 'sub-fedcba9876543210', updatedAt: '', count: 0, error: 'fetch_timeout' })]
      })
    )
    expect(s.connector).toBe('proxy')
    expect(s.proxy.servers).toHaveLength(3)
    expect(s.proxy.selected).toBe(SERVER_B)
    expect(s.proxy.subscriptions[1].error).toBe('fetch_timeout')
  })
  test('ignores unknown extra keys', () => {
    const s = proxyState({ servers: [], selected: null, subscriptions: [], extra: 1 }, { more: true })
    expect(parseState(s).proxy.servers).toEqual([])
  })
  test('rejects a missing proxy block, a missing or unknown connector', () => {
    const noProxy = emptyState() as Record<string, unknown>
    delete noProxy.proxy
    expect(() => parseState(noProxy)).toThrow(/proxy/)
    const noConnector = emptyState() as Record<string, unknown>
    delete noConnector.connector
    expect(() => parseState(noConnector)).toThrow(/connector/)
    expect(() => parseState({ ...emptyState(), connector: 'wireguard' })).toThrow(/connector/)
  })
  test('rejects bad ids, duplicates, an unknown protocol and a bad source', () => {
    const bad = (proxy: unknown, re: RegExp): void => expect(() => parseState(proxyState(proxy))).toThrow(re)
    const empty = { servers: [], selected: null, subscriptions: [] }
    bad({ ...empty, servers: [server('0123456789ABCDEF')] }, /servers\[0\]\.id/)
    bad({ ...empty, servers: [server('0123')] }, /servers\[0\]\.id/)
    bad({ ...empty, servers: [server(SERVER_A), server(SERVER_A)] }, /servers\[1\]\.id repeats/)
    bad({ ...empty, servers: [server(SERVER_A, { protocol: 'openvpn' })] }, /protocol/)
    bad({ ...empty, servers: [server(SERVER_A, { source: 'other' })] }, /source/)
    bad({ ...empty, servers: [server(SERVER_A, { source: 'sub-xyz' })] }, /source/)
    bad({ ...empty, selected: 'nope' }, /selected/)
    bad({ ...empty, selected: SUB }, /selected/)
    bad({ ...empty, subscriptions: [sub({ id: SERVER_A })] }, /subscriptions\[0\]\.id/)
    bad({ ...empty, subscriptions: [sub(), sub()] }, /subscriptions\[1\]\.id repeats/)
    bad({ ...empty, subscriptions: [sub({ count: -1 })] }, /count/)
    bad({ ...empty, subscriptions: [sub({ count: 1.5 })] }, /count/)
    bad({ ...empty, subscriptions: [sub({ error: 3 })] }, /error/)
    bad({ ...empty, subscriptions: [sub({ updatedAt: 'x'.repeat(65) })] }, /updatedAt/)
    bad({ servers: [], selected: null }, /subscriptions/)
  })
  test('a server of a subscription that is gone is kept, not a reason to refuse the state', () => {
    const s = parseState(proxyState({ servers: [server(SERVER_A, { source: SUB })], selected: null, subscriptions: [] }))
    expect(s.proxy.servers[0].source).toBe(SUB)
  })
  test('a name of 256 bytes passes, of 257 throws', () => {
    const empty = { selected: null, subscriptions: [] }
    const named = (name: string): Record<string, unknown> => proxyState({ ...empty, servers: [server(SERVER_A, { name })] })
    expect(parseState(named('я'.repeat(127) + 'ab')).proxy.servers).toHaveLength(1) // 256 bytes
    expect(() => parseState(named('я'.repeat(128) + 'a'))).toThrow(/name/) // 257 bytes
  })
  test('limits: names, 500 servers, 20 subscriptions', () => {
    const empty = { servers: [], selected: null, subscriptions: [] }
    const id = (i: number): string => i.toString(16).padStart(16, '0')
    const many = (n: number): unknown[] => Array.from({ length: n }, (_, i) => server(id(i)))
    expect(parseState(proxyState({ ...empty, servers: many(500) })).proxy.servers).toHaveLength(500)
    expect(() => parseState(proxyState({ ...empty, servers: many(501) }))).toThrow(/more than 500/)
    const subs = (n: number): unknown[] => Array.from({ length: n }, (_, i) => sub({ id: `sub-${id(i)}` }))
    expect(parseState(proxyState({ ...empty, subscriptions: subs(20) })).proxy.subscriptions).toHaveLength(20)
    expect(() => parseState(proxyState({ ...empty, subscriptions: subs(21) }))).toThrow(/more than 20/)
    expect(() => parseState(proxyState({ ...empty, servers: [server(SERVER_A, { name: 'я'.repeat(129) })] }))).toThrow(/name/)
    expect(parseState(proxyState({ ...empty, servers: [server(SERVER_A, { name: 'я'.repeat(128) })] })).proxy.servers).toHaveLength(1)
  })
})

describe('parseProxyAddResult', () => {
  const ok = { added: [SERVER_A], existing: 1, rejected: [{ line: 0, code: 'too_many' }, { line: 3, code: 'bad_link' }], rejectedMore: 4 }
  test('accepts the helper answer', () => {
    expect(parseProxyAddResult(ok)).toEqual(ok)
    expect(parseProxyAddResult({ added: [], existing: 0, rejected: [], rejectedMore: 0 }).added).toEqual([])
  })
  test('keeps a code it does not know', () => {
    const r = parseProxyAddResult({ ...ok, rejected: [{ line: 1, code: 'from_the_future' }] })
    expect(r.rejected).toEqual([{ line: 1, code: 'from_the_future' }])
    expect(() => parseProxyAddResult({ ...ok, rejected: [{ line: 1, code: 'x'.repeat(65) }] })).toThrow(/code/)
  })
  test('is strict about the shape', () => {
    expect(() => parseProxyAddResult(null)).toThrow()
    expect(() => parseProxyAddResult({ ...ok, added: ['xyz'] })).toThrow(/added\[0\]/)
    expect(() => parseProxyAddResult({ ...ok, existing: -1 })).toThrow(/existing/)
    expect(() => parseProxyAddResult({ ...ok, rejectedMore: 'a' })).toThrow(/rejectedMore/)
    expect(() => parseProxyAddResult({ ...ok, rejected: [{ line: -1, code: 'x' }] })).toThrow(/line/)
    expect(() => parseProxyAddResult({ ...ok, rejected: [{ line: 1.5, code: 'x' }] })).toThrow(/line/)
    expect(() => parseProxyAddResult({ ...ok, rejected: [{ line: 1, code: 7 }] })).toThrow(/code/)
    const fifty = Array.from({ length: 50 }, (_, i) => ({ line: i + 1, code: 'bad_link' }))
    expect(parseProxyAddResult({ ...ok, rejected: fifty }).rejected).toHaveLength(50)
    expect(() => parseProxyAddResult({ ...ok, rejected: [...fifty, { line: 51, code: 'bad_link' }] })).toThrow(/more than 50/)
  })
  test('the line codes are the closed list of 17', () => {
    expect(PROXY_REJECT_CODES).toHaveLength(17)
    expect(new Set(PROXY_REJECT_CODES).size).toBe(17)
    for (const c of PROXY_REJECT_CODES) expect(ERROR_CODES.includes(c as never) || c === 'too_many').toBe(true)
  })
})
