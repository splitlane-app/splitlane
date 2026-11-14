import { describe, expect, test } from 'vitest'
import { LIMITS, packageValue, parseActivity, parseState, utf8Bytes } from './protocol'

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
  tailscale: { backend: 'stopped', authUrl: null, deviceName: '', exitNodes: [], selectedExitNode: null, path: null },
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
