import { expect, test, vi } from 'vitest'
import { IPC, isSetupRunning, SETUP_RUNNING, type Command, type ProxyOutcome, type Snapshot, type VpsOutcome } from '../shared/ipc'
import type { AppRule } from '../shared/protocol'
import type { UpdateState } from '../shared/update'
import {
  exportPath,
  mergePrograms,
  registerIpc,
  runCommand,
  runEnable,
  validateCommand,
  validateDetailKey,
  validateExePath,
  validateLoginUrl,
  validatePathList,
  validateSettingsPatch,
  type IpcDeps,
  type IpcEventLike
} from './ipc'
import { CallTimeoutError, ConnectionClosedError, HelperCallError } from './rpc'

const ID1 = 'b6f3c1de-8a41-4e5f-9a7c-2d0e7f1a9b35'
const ID2 = '0c9e2a71-5d3b-4f86-b1e4-7a2c9d8e6f10'
const ID3 = '5a1f7c3e-2b9d-4e60-8f14-c3d2b1a09e87'
const ID4 = 'e2d4f6a8-1c3b-4d5e-a7f9-0b1c2d3e4f5a'

function rule(over: Partial<AppRule> = {}): AppRule {
  return {
    id: ID1,
    name: 'Editor',
    exe: 'C:\\Tools\\Editor\\editor.exe',
    matcher: { kind: 'folder', value: '' },
    lane: 'direct',
    ...over
  }
}

function lanesSet(apps: unknown[], defaultLane: unknown = 'vpn'): unknown {
  return { method: 'lanes.set', apps, defaultLane }
}

test('validateCommand accepts every method of the union', () => {
  const apps: AppRule[] = [
    rule(),
    rule({
      id: ID2,
      name: '',
      exe: 'C:\\Program Files\\WindowsApps\\Vendor.Notes_1.2.3.0_x64__abcdefghijk\\App\\notes.exe',
      matcher: { kind: 'package', value: '' },
      lane: 'vpn'
    }),
    rule({ id: ID3, name: 'Общий', exe: 'D:\\Общие\\tool.exe', matcher: { kind: 'file', value: 'D:\\Общие\\tool.exe' } }),
    rule({ id: ID4, name: '', exe: '', matcher: { kind: 'system', value: '' } })
  ]
  const commands: Command[] = [
    { method: 'routing.enable' },
    { method: 'routing.disable' },
    { method: 'routing.disable', skipRestore: true },
    { method: 'routing.disable', skipRestore: false },
    { method: 'routing.allDirect', on: true },
    { method: 'routing.allDirect', on: false },
    { method: 'lanes.set', apps, defaultLane: 'vpn' },
    { method: 'lanes.set', apps: [], defaultLane: 'direct' },
    { method: 'tailscale.login' },
    { method: 'tailscale.logout' },
    { method: 'tailscale.setExitNode', id: '100.64.0.5' },
    { method: 'systemTailscale.clearExitNode' }
  ]
  for (const c of commands) expect(validateCommand(structuredClone(c))).toEqual(c)
  expect(new Set(commands.map((c) => c.method)).size).toBe(8)

  // the result is built anew from the checked fields
  const input = { method: 'lanes.set', apps: [rule()], defaultLane: 'vpn' }
  const out = validateCommand(input)
  expect(out).not.toBe(input)
  expect(out.method === 'lanes.set' && out.apps[0]).not.toBe(input.apps[0])
})

test('validateCommand rejects unknown methods and extra fields', () => {
  const bad: unknown[] = [
    null,
    undefined,
    'routing.enable',
    42,
    [],
    [{ method: 'routing.enable' }],
    {},
    { method: 1 },
    // methods the page never sends: the main process owns them
    { method: 'helper.shutdown' },
    { method: 'diagnostics.export' },
    { method: 'state.get' },
    { method: 'hello' },
    { method: 'activity.subscribe' },
    { method: 'Routing.Enable' },
    { method: 'toString' },
    { method: '__proto__' },
    // extra fields
    { method: 'routing.enable', params: {} },
    { method: 'routing.enable', extra: true },
    { method: 'routing.disable', skipRestore: true, force: true },
    { method: 'tailscale.login', url: 'https://login.tailscale.com/a/0123abcd' },
    { method: 'tailscale.setExitNode', id: '100.64.0.5', name: 'node-a' },
    { method: 'lanes.set', apps: [], defaultLane: 'vpn', extra: 1 },
    lanesSet([{ ...rule(), extra: 1 }]),
    lanesSet([{ ...rule(), matcher: { kind: 'folder', value: '', extra: 1 } }]),
    // missing or mistyped fields
    { method: 'routing.allDirect' },
    { method: 'routing.allDirect', on: 'yes' },
    { method: 'routing.disable', skipRestore: 1 },
    { method: 'lanes.set', apps: [] },
    { method: 'lanes.set', apps: {}, defaultLane: 'vpn' },
    lanesSet([{ id: ID1, name: 'Editor', exe: 'C:\\Tools\\editor.exe', lane: 'vpn' }]),
    lanesSet([{ ...rule(), name: null }]),
    lanesSet([{ ...rule(), matcher: null }]),
    lanesSet([null]),
    { method: 'tailscale.setExitNode' },
    // not plain objects
    new Date(),
    Object.create({ method: 'routing.enable' }) as unknown
  ]
  for (const x of bad) expect(() => validateCommand(x), JSON.stringify(x) ?? String(x)).toThrow()
})

test('validateCommand enforces LIMITS, UUIDs and lanes', () => {
  // at most 500 rules
  const ids = Array.from({ length: 501 }, (_, i) => `00000000-0000-4000-8000-${i.toString(16).padStart(12, '0')}`)
  expect(() => validateCommand(lanesSet(ids.slice(0, 500).map((id) => rule({ id }))))).not.toThrow()
  expect(() => validateCommand(lanesSet(ids.map((id) => rule({ id }))))).toThrow('500')

  // name up to 256 bytes of UTF-8, exe and value up to 1024
  expect(() => validateCommand(lanesSet([rule({ name: 'Ж'.repeat(128) })]))).not.toThrow()
  expect(() => validateCommand(lanesSet([rule({ name: 'Ж'.repeat(128) + 'a' })]))).toThrow('256')
  const path = (bytes: number): string => 'C:\\' + 'a'.repeat(bytes - 3)
  expect(() => validateCommand(lanesSet([rule({ exe: path(1024) })]))).not.toThrow()
  expect(() => validateCommand(lanesSet([rule({ exe: path(1025) })]))).toThrow('1024')
  expect(() => validateCommand(lanesSet([rule({ exe: 'C:\\' + 'Ж'.repeat(511) })]))).toThrow('1024')
  expect(() => validateCommand(lanesSet([rule({ matcher: { kind: 'file', value: path(1024) } })]))).not.toThrow()
  expect(() => validateCommand(lanesSet([rule({ matcher: { kind: 'file', value: path(1025) } })]))).toThrow('1024')

  // ids: the helper's UUID shape (rules it accepted come back unchanged), unique
  expect(() => validateCommand(lanesSet([rule({ id: ID1.toUpperCase() })]))).not.toThrow()
  expect(() => validateCommand(lanesSet([rule({ id: '6ba7b810-9dad-11d1-80b4-00c04fd430c8' })]))).not.toThrow()
  for (const id of [
    '',
    'not-a-uuid',
    `{${ID1}}`,
    ID1.replaceAll('-', ''),
    ID1.slice(0, -1),
    'g' + ID1.slice(1),
    ` ${ID1}`,
    `${ID1}\n`
  ]) {
    expect(() => validateCommand(lanesSet([rule({ id })])), id).toThrow('UUID')
  }
  expect(() => validateCommand(lanesSet([{ ...rule(), id: 42 }]))).toThrow()
  expect(() => validateCommand(lanesSet([rule(), rule({ name: 'Copy' })]))).toThrow('repeats')

  // lanes and kinds
  for (const lane of ['proxy', 'VPN', '', 'both']) {
    expect(() => validateCommand(lanesSet([{ ...rule(), lane }])), lane).toThrow()
    expect(() => validateCommand(lanesSet([], lane)), lane).toThrow()
  }
  expect(() => validateCommand({ method: 'routing.disable' })).not.toThrow()
  for (const kind of ['nested', 'Folder', '']) {
    expect(() => validateCommand(lanesSet([{ ...rule(), matcher: { kind, value: '' } }])), kind).toThrow()
  }

  // exit node: an IPv4 address
  expect(validateCommand({ method: 'tailscale.setExitNode', id: '100.64.0.12' })).toEqual({
    method: 'tailscale.setExitNode',
    id: '100.64.0.12'
  })
  for (const id of ['', 'node-a', '100.64.0', '100.64.0.256', '100.064.0.5', ' 100.64.0.5', '100.64.0.5\n', 'fd7a::1']) {
    expect(() => validateCommand({ method: 'tailscale.setExitNode', id }), id).toThrow()
  }
})

test('validateLoginUrl accepts only https login.tailscale.com', () => {
  expect(validateLoginUrl('https://login.tailscale.com/a/0123abcd')).toBe('https://login.tailscale.com/a/0123abcd')
  // the normalized form is what opens
  expect(validateLoginUrl('https://LOGIN.tailscale.com:443/a/0123abcd')).toBe('https://login.tailscale.com/a/0123abcd')
  for (const bad of [
    'http://login.tailscale.com/a/0123abcd',
    'https://login.tailscale.com.example.net/a/0123abcd',
    'https://example.net/login.tailscale.com/a/0123abcd',
    'https://login.tailscale.com@example.net/a/0123abcd',
    'https://user:secret@login.tailscale.com/a/0123abcd',
    'https://login.tailscale.com:8443/a/0123abcd',
    'https://sub.login.tailscale.com/a/0123abcd',
    'https://tailscale.com/a/0123abcd',
    'file:///C:/Windows/notepad.exe',
    'javascript:alert(1)',
    'login.tailscale.com/a/0123abcd',
    '',
    'https://login.tailscale.com/a/' + 'x'.repeat(4096)
  ]) {
    expect(validateLoginUrl(bad), bad.slice(0, 80)).toBeNull()
  }
  for (const bad of [null, undefined, 42, {}, ['https://login.tailscale.com/a/0123abcd']]) {
    expect(validateLoginUrl(bad)).toBeNull()
  }
})

// ---- beyond the brief: the rest of what the page sends, and the command path ----

test('page arguments for icons, drops, detail and settings are checked', () => {
  expect(validateExePath('C:\\Tools\\Editor\\editor.exe')).toBe('C:\\Tools\\Editor\\editor.exe')
  expect(validateExePath('d:\\Игры\\game.exe')).toBe('d:\\Игры\\game.exe')
  for (const bad of ['', 'editor.exe', '\\\\server\\share\\a.exe', 'C:relative.exe', 'C:\\' + 'a'.repeat(1022), 42, null]) {
    expect(() => validateExePath(bad)).toThrow()
  }

  expect(validatePathList(['C:\\a.exe', 'C:\\b.lnk', 'notes.txt', ''])).toEqual(['C:\\a.exe', 'C:\\b.lnk', 'notes.txt', ''])
  expect(validatePathList(Array.from({ length: 100 }, () => 'C:\\a.exe'))).toHaveLength(100)
  expect(() => validatePathList(Array.from({ length: 101 }, () => 'C:\\a.exe'))).toThrow('100')
  expect(() => validatePathList(['C:\\' + 'a'.repeat(1022)])).toThrow('1024')
  for (const bad of ['C:\\a.exe', [42], [null], null, {}]) expect(() => validatePathList(bad)).toThrow()

  expect(validateDetailKey(null)).toBeNull()
  expect(validateDetailKey(ID1)).toBe(ID1)
  expect(validateDetailKey('C:\\Tools\\Editor\\')).toBe('C:\\Tools\\Editor\\')
  expect(() => validateDetailKey('Ж'.repeat(513))).toThrow('1024')
  for (const bad of [undefined, 42, {}]) expect(() => validateDetailKey(bad)).toThrow()

  expect(validateSettingsPatch({})).toEqual({})
  expect(validateSettingsPatch({ openAtLogin: true, enableOnStart: false, connectedOnce: true })).toEqual({
    openAtLogin: true,
    enableOnStart: false,
    connectedOnce: true
  })
  for (const bad of [{ openAtLogin: 'yes' }, { theme: 'dark' }, { openAtLogin: true, extra: false }, null, [], 'x']) {
    expect(() => validateSettingsPatch(bad)).toThrow()
  }
})

function fakeLink(fail?: Error) {
  return {
    call: vi.fn(async (_method: string, _params?: object, _timeoutMs?: number): Promise<unknown> => {
      if (fail) throw fail
      return {}
    }),
    setActivityDetail: vi.fn(async (_key: string | null): Promise<void> => {}),
    retry: vi.fn((): void => {})
  }
}

test('runCommand sends exactly the protocol params', async () => {
  const apps = [rule()]
  const cases: [Command, string, object | undefined][] = [
    [{ method: 'routing.enable' }, 'routing.enable', undefined],
    [{ method: 'routing.disable' }, 'routing.disable', undefined],
    [{ method: 'routing.disable', skipRestore: false }, 'routing.disable', undefined],
    [{ method: 'routing.disable', skipRestore: true }, 'routing.disable', { skipRestore: true }],
    [{ method: 'routing.allDirect', on: true }, 'routing.allDirect', { on: true }],
    [{ method: 'routing.allDirect', on: false }, 'routing.allDirect', { on: false }],
    [{ method: 'lanes.set', apps, defaultLane: 'direct' }, 'lanes.set', { apps, defaultLane: 'direct' }],
    [{ method: 'tailscale.login' }, 'tailscale.login', undefined],
    [{ method: 'tailscale.logout' }, 'tailscale.logout', undefined],
    [{ method: 'tailscale.setExitNode', id: '100.64.0.5' }, 'tailscale.setExitNode', { id: '100.64.0.5' }],
    [{ method: 'systemTailscale.clearExitNode' }, 'systemTailscale.clearExitNode', undefined]
  ]
  for (const [c, method, params] of cases) {
    const link = fakeLink()
    expect(await runCommand(link, c)).toEqual({ ok: true })
    expect(link.call).toHaveBeenCalledTimes(1)
    const [m, p] = link.call.mock.calls[0]
    expect(m).toBe(method)
    expect(p).toStrictEqual(params)
  }
})

test('runCommand maps helper errors and a closed link without throwing', async () => {
  const rejected = {
    code: 'rule_rejected',
    message: 'rules rejected',
    rules: [{ id: ID1, reason: 'too_broad' }]
  } as const
  const linkClosed = { ok: false, error: { code: 'internal', message: 'link closed' } }
  const cases: [Error, unknown][] = [
    [new HelperCallError({ ...rejected, rules: [...rejected.rules] }), { ok: false, error: rejected }],
    [new HelperCallError({ code: 'wrong_phase', message: 'not possible in phase stopping' }), { ok: false, error: { code: 'wrong_phase', message: 'not possible in phase stopping' } }],
    [new ConnectionClosedError(), linkClosed],
    [new ConnectionClosedError('no link to the helper'), linkClosed],
    [new CallTimeoutError('lanes.set', 30000), linkClosed],
    [new Error('write after end'), { ok: false, error: { code: 'internal', message: 'write after end' } }]
  ]
  for (const [err, expected] of cases) {
    expect(await runCommand(fakeLink(err), { method: 'routing.enable' })).toEqual(expected)
  }
})

function ipcHarness() {
  const handlers = new Map<string, (event: IpcEventLike, ...args: unknown[]) => unknown>()
  const page = { id: 'page' }
  const link = fakeLink()
  const d = {
    ipc: { handle: (channel: string, fn: (event: IpcEventLike, ...args: unknown[]) => unknown) => void handlers.set(channel, fn) },
    accept: (e: IpcEventLike) => e.sender === page,
    link,
    snapshot: vi.fn((): Snapshot => {
      throw new Error('not used')
    }),
    userCommand: vi.fn(),
    installHelper: vi.fn(async () => {}),
    proxyAddFile: vi.fn(async (): Promise<ProxyOutcome> => ({ kind: 'cancelled' })),
    vpsPickKey: vi.fn(async () => null),
    vpsForgetKey: vi.fn(async () => {}),
    vpsSetup: vi.fn(async (_x: unknown): Promise<VpsOutcome> => ({ kind: 'cancelled' })),
    vpsCancel: vi.fn(async () => {}),
    vpsBusy: vi.fn(() => false),
    appInfo: vi.fn(async () => ({ version: '0.2.0', installed: true })),
    setSettings: vi.fn(async (p: object) => ({ openAtLogin: false, enableOnStart: false, connectedOnce: false, checkUpdates: true, pendingUpdate: null, language: 'system' as const, lastExitNode: null, notifications: true, notifiedVersion: null, ...p })),
    installedPrograms: vi.fn(async () => []),
    pickProgram: vi.fn(async () => null),
    resolveDropped: vi.fn(async (paths: string[]) =>
      paths.map((p) => ({ kind: 'skipped' as const, name: p, reason: 'notProgram' as const }))
    ),
    icon: vi.fn(async (_exe: string): Promise<string | null> => 'data:image/png;base64,AA=='),
    openExternal: vi.fn(async (_url: string) => {}),
    saveDiagnostics: vi.fn(async () => ({ saved: false })),
    quit: vi.fn(async () => {}),
    updateState: vi.fn((): UpdateState => ({ kind: 'idle' })),
    checkUpdates: vi.fn(async () => {}),
    installUpdate: vi.fn(async () => {}),
    cancelUpdate: vi.fn(() => {}),
    updateLater: vi.fn(() => {}),
    dismissUpdate: vi.fn(() => {}),
    log: vi.fn()
  } satisfies IpcDeps
  registerIpc(d)
  const invoke = (channel: string, ...args: unknown[]): Promise<unknown> =>
    Promise.resolve().then(() => {
      const h = handlers.get(channel)
      if (!h) throw new Error(`no handler for ${channel}`)
      return h({ sender: page }, ...args)
    })
  const invokeFrom = (sender: unknown, channel: string, ...args: unknown[]): Promise<unknown> =>
    Promise.resolve().then(() => handlers.get(channel)?.({ sender }, ...args))
  return { d, link, handlers, invoke, invokeFrom }
}

test('registerIpc checks the sender and every argument from the page', async () => {
  const h = ipcHarness()
  expect([...h.handlers.keys()].sort()).toEqual(
    Object.values(IPC)
      // the pushes from main to the page have no handler: the page cannot send on them
      .filter((c) => c !== IPC.snapshotChanged && c !== IPC.activity && c !== IPC.vpsEvent && c !== IPC.openSettings)
      .sort()
  )

  // the VPS channels pass the page's argument on untouched: vpsSetup checks it itself, the others take none
  await h.invoke(IPC.vpsSetup, { address: 'x' })
  expect(h.d.vpsSetup).toHaveBeenCalledWith({ address: 'x' })
  await h.invoke(IPC.vpsPickKey, 'ignored')
  expect(h.d.vpsPickKey).toHaveBeenCalledWith()
  await h.invoke(IPC.vpsForgetKey)
  await h.invoke(IPC.vpsCancel)
  expect(h.d.vpsForgetKey).toHaveBeenCalled()
  expect(h.d.vpsCancel).toHaveBeenCalled()

  // another sender gets nothing
  await expect(h.invokeFrom({ id: 'other' }, IPC.command, { method: 'routing.enable' })).rejects.toThrow()
  expect(h.link.call).not.toHaveBeenCalled()

  // commands: a bad one is answered, not thrown; routing commands stop the auto-enable
  expect(await h.invoke(IPC.command, { method: 'helper.shutdown' })).toMatchObject({ ok: false, error: { code: 'bad_request' } })
  expect(h.link.call).not.toHaveBeenCalled()
  expect(await h.invoke(IPC.command, lanesSet([]))).toEqual({ ok: true })
  expect(h.d.userCommand).not.toHaveBeenCalled()
  expect(await h.invoke(IPC.command, { method: 'routing.disable', skipRestore: true })).toEqual({ ok: true })
  expect(h.d.userCommand).toHaveBeenCalledTimes(1)
  expect(h.link.call.mock.calls.map(([m, p]) => [m, p])).toEqual([
    ['lanes.set', { apps: [], defaultLane: 'vpn' }],
    ['routing.disable', { skipRestore: true }]
  ])

  // icons: a bad path or a failing source gives null
  expect(await h.invoke(IPC.icon, 'C:\\Tools\\editor.exe')).toBe('data:image/png;base64,AA==')
  expect(await h.invoke(IPC.icon, 'editor.exe')).toBeNull()
  h.d.icon.mockRejectedValueOnce(new Error('EPERM'))
  expect(await h.invoke(IPC.icon, 'C:\\Tools\\editor.exe')).toBeNull()
  expect(h.d.icon).toHaveBeenCalledTimes(2)

  // the login link opens only for login.tailscale.com
  expect(await h.invoke(IPC.openLogin, 'https://login.tailscale.com/a/0123abcd')).toBe(true)
  expect(await h.invoke(IPC.openLogin, 'https://example.net/a/0123abcd')).toBe(false)
  h.d.openExternal.mockRejectedValueOnce(new Error('no browser'))
  expect(await h.invoke(IPC.openLogin, 'https://login.tailscale.com/a/0123abcd')).toBe(false)
  expect(h.d.openExternal.mock.calls).toEqual([
    ['https://login.tailscale.com/a/0123abcd'],
    ['https://login.tailscale.com/a/0123abcd']
  ])

  // drops, detail and settings
  expect(await h.invoke(IPC.resolveDropped, ['notes.txt'])).toEqual([
    { kind: 'skipped', name: 'notes.txt', reason: 'notProgram' }
  ])
  await expect(h.invoke(IPC.resolveDropped, 'C:\\a.exe')).rejects.toThrow()
  await h.invoke(IPC.setActivityDetail, ID1)
  await h.invoke(IPC.setActivityDetail, null)
  await expect(h.invoke(IPC.setActivityDetail, 42)).rejects.toThrow()
  expect(h.link.setActivityDetail.mock.calls).toEqual([[ID1], [null]])
  h.link.setActivityDetail.mockRejectedValueOnce(new HelperCallError({ code: 'bad_request', message: 'detailFor' }))
  await expect(h.invoke(IPC.setActivityDetail, ID2)).resolves.toBeUndefined()
  expect(await h.invoke(IPC.setSettings, { enableOnStart: true })).toEqual({
    openAtLogin: false,
    enableOnStart: true,
    connectedOnce: false,
    checkUpdates: true,
    pendingUpdate: null,
    language: 'system',
    lastExitNode: null,
    notifications: true,
    notifiedVersion: null
  })
  await expect(h.invoke(IPC.setSettings, { enableOnStart: 1 })).rejects.toThrow()
  expect(h.d.setSettings).toHaveBeenCalledTimes(1)

  await h.invoke(IPC.retryLink)
  expect(h.link.retry).toHaveBeenCalledTimes(1)
})

test('installed programs merge the Start menu and the Store', () => {
  const start = [
    { exe: 'C:\\Program Files\\Editor\\editor.exe', name: 'Редактор' },
    { exe: 'C:\\Tools\\viewer.exe', name: 'Viewer' }
  ]
  const store = [
    { exe: 'C:\\PROGRAM FILES\\EDITOR\\EDITOR.EXE', name: 'Editor (Store)' },
    { exe: 'C:\\Program Files\\WindowsApps\\Vendor.Notes_1.2.3.0_x64__abcdefghijk\\notes.exe', name: 'Заметки' },
    { exe: 'C:\\Program Files\\WindowsApps\\Vendor.Atlas_2.0.0.0_x64__abcdefghijk\\atlas.exe', name: 'Atlas' }
  ]
  // the Start-menu entry wins a duplicate; the Russian order puts Cyrillic first
  expect(mergePrograms('ru', start, store).map((p) => p.name)).toEqual(['Заметки', 'Редактор', 'Atlas', 'Viewer'])
  // the English order puts Latin first
  expect(mergePrograms('en', start, store).map((p) => p.name)).toEqual(['Atlas', 'Viewer', 'Заметки', 'Редактор'])
  expect(mergePrograms('ru', [], [])).toEqual([])
})

test('installed programs take in the Steam and Epic games', () => {
  const start = [
    { exe: 'C:\\Tools\\viewer.exe', name: 'Viewer' },
    // a game shortcut of the Start menu, resolved to the same exe as the Steam list
    { exe: 'D:\\Games\\SteamLibrary\\steamapps\\common\\Starfall\\bin\\starfall.exe', name: 'Звёздная тактика' }
  ]
  const store = [{ exe: 'C:\\Program Files\\WindowsApps\\Vendor.Atlas_2.0.0.0_x64__abcdefghijk\\atlas.exe', name: 'Atlas' }]
  const games = [
    { exe: 'd:\\games\\steamlibrary\\steamapps\\common\\starfall\\bin\\STARFALL.EXE', name: 'Starfall (Steam)' },
    { exe: 'C:\\Program Files (x86)\\Steam\\steamapps\\common\\Nebula Drift\\Nebula.exe', name: 'Nebula Drift' },
    { exe: 'C:\\Program Files\\Epic Games\\LumenRush\\Binaries\\Win64\\LumenRush.exe', name: 'Lumen Rush' },
    { exe: 'C:\\Program Files\\Epic Games\\Aurora\\Aurora.exe', name: 'Аврора' }
  ]
  expect(mergePrograms('ru', start, store, games)).toEqual([
    { exe: 'C:\\Program Files\\Epic Games\\Aurora\\Aurora.exe', name: 'Аврора' },
    { exe: 'D:\\Games\\SteamLibrary\\steamapps\\common\\Starfall\\bin\\starfall.exe', name: 'Звёздная тактика' },
    { exe: 'C:\\Program Files\\WindowsApps\\Vendor.Atlas_2.0.0.0_x64__abcdefghijk\\atlas.exe', name: 'Atlas' },
    { exe: 'C:\\Program Files\\Epic Games\\LumenRush\\Binaries\\Win64\\LumenRush.exe', name: 'Lumen Rush' },
    { exe: 'C:\\Program Files (x86)\\Steam\\steamapps\\common\\Nebula Drift\\Nebula.exe', name: 'Nebula Drift' },
    { exe: 'C:\\Tools\\viewer.exe', name: 'Viewer' }
  ])
})

test('exportPath takes the archive path from diagnostics.export', () => {
  const p = 'C:\\ProgramData\\SplitLane\\export\\splitlane-diagnostics-20261114-101500.zip'
  expect(exportPath({ path: p })).toBe(p)
  for (const bad of [{}, { path: 42 }, { path: 'export\\a.zip' }, { path: 'C:\\ProgramData\\SplitLane\\export\\a.txt' }, null, 'x']) {
    expect(() => exportPath(bad)).toThrow()
  }
})

test('validateSettingsPatch accepts language and refuses other values', () => {
  for (const ok of ['system', 'ru', 'en'] as const) expect(validateSettingsPatch({ language: ok })).toEqual({ language: ok })
  expect(validateSettingsPatch({ language: 'en', openAtLogin: true })).toEqual({ language: 'en', openAtLogin: true })
  for (const bad of ['de', 'EN', '', 1, null, true, {}]) {
    expect(() => validateSettingsPatch({ language: bad })).toThrow()
  }
})

test('validateSettingsPatch accepts checkUpdates and refuses pendingUpdate', () => {
  expect(validateSettingsPatch({ checkUpdates: false })).toEqual({ checkUpdates: false })
  expect(() => validateSettingsPatch({ checkUpdates: 'no' })).toThrow()
  expect(() => validateSettingsPatch({ pendingUpdate: '1.2.0' })).toThrow()
  expect(() => validateSettingsPatch({ pendingUpdate: null })).toThrow()
  expect(() => validateSettingsPatch({ checkUpdates: true, pendingUpdate: '1.2.0' })).toThrow()
})

test('validateSettingsPatch accepts notifications and refuses notifiedVersion', () => {
  expect(validateSettingsPatch({ notifications: false })).toEqual({ notifications: false })
  expect(() => validateSettingsPatch({ notifications: 'no' })).toThrow()
  expect(() => validateSettingsPatch({ notifiedVersion: '1.2.0' })).toThrow()
})

test('validateSettingsPatch refuses lastExitNode: the main process keeps it', () => {
  expect(() => validateSettingsPatch({ lastExitNode: { id: '100.64.0.2', name: 'x' } })).toThrow()
  expect(() => validateSettingsPatch({ lastExitNode: null })).toThrow()
})

test('openUpdateNotes opens only the release page of the current state', async () => {
  const h = ipcHarness()
  const notes = 'https://github.com/splitlane-app/splitlane/releases/tag/v1.2.0'
  const cases: [UpdateState, boolean][] = [
    [{ kind: 'available', version: '1.2.0', notesUrl: notes, hidden: false }, true],
    [{ kind: 'installRefused', version: '1.2.0', notesUrl: notes, reason: '', canRetry: true }, true],
    [{ kind: 'idle' }, false],
    [{ kind: 'installed', version: '1.2.0' }, false],
    [{ kind: 'available', version: '1.2.0', notesUrl: 'https://example.net/releases/tag/v1.2.0', hidden: false }, false],
    [{ kind: 'available', version: '1.2.0', notesUrl: 'http://github.com/splitlane-app/splitlane/releases/tag/v1.2.0', hidden: false }, false],
    [{ kind: 'available', version: '1.2.0', notesUrl: `${notes}?x=1`, hidden: false }, false],
    [{ kind: 'available', version: '1.2.0', notesUrl: `${notes}/`, hidden: false }, false],
    [{ kind: 'available', version: '1.2', notesUrl: 'https://github.com/splitlane-app/splitlane/releases/tag/v1.2', hidden: false }, false]
  ]
  for (const [update, opens] of cases) {
    h.d.updateState.mockReturnValue(update)
    h.d.openExternal.mockClear()
    expect(await h.invoke(IPC.openUpdateNotes), JSON.stringify(update)).toBe(opens)
    expect(h.d.openExternal).toHaveBeenCalledTimes(opens ? 1 : 0)
    if (opens) expect(h.d.openExternal).toHaveBeenCalledWith(notes)
  }
  h.d.updateState.mockReturnValue(cases[0][0])
  h.d.openExternal.mockRejectedValueOnce(new Error('no browser'))
  expect(await h.invoke(IPC.openUpdateNotes)).toBe(false)
})

test('the update requests reach the updater without arguments', async () => {
  const h = ipcHarness()
  await h.invoke(IPC.checkUpdates, 'ignored')
  await h.invoke(IPC.installUpdate)
  await h.invoke(IPC.cancelUpdate)
  await h.invoke(IPC.updateLater)
  await h.invoke(IPC.dismissUpdate)
  expect(h.d.checkUpdates).toHaveBeenCalledWith()
  expect(h.d.installUpdate).toHaveBeenCalledTimes(1)
  expect(h.d.cancelUpdate).toHaveBeenCalledTimes(1)
  expect(h.d.updateLater).toHaveBeenCalledTimes(1)
  expect(h.d.dismissUpdate).toHaveBeenCalledTimes(1)
  await expect(h.invokeFrom({ id: 'other' }, IPC.installUpdate)).rejects.toThrow()
  expect(h.d.installUpdate).toHaveBeenCalledTimes(1)
})

test('checkUpdates and installUpdate answer before the operation ends', async () => {
  const h = ipcHarness()
  h.d.installUpdate.mockImplementation(() => new Promise<void>(() => {}))
  h.d.checkUpdates.mockImplementation(() => new Promise<void>(() => {}))
  await expect(h.invoke(IPC.installUpdate)).resolves.toBeUndefined()
  await expect(h.invoke(IPC.checkUpdates)).resolves.toBeUndefined()
  h.d.installUpdate.mockRejectedValueOnce(new Error('boom'))
  await expect(h.invoke(IPC.installUpdate)).resolves.toBeUndefined()
  await Promise.resolve()
  expect(h.d.log).toHaveBeenCalledWith('ipc: installUpdate: boom')
})

test('routing.enable waits for a server setup: the page, the tray and the auto-enable get the same refusal', async () => {
  // runEnable is what the tray and the auto-enable call
  const link = fakeLink()
  let running = true
  expect(await runEnable(link, () => running)).toEqual({ ok: false, error: SETUP_RUNNING })
  expect(isSetupRunning(SETUP_RUNNING)).toBe(true)
  expect(isSetupRunning({ code: 'busy', message: 'another download' })).toBe(false)
  expect(link.call).not.toHaveBeenCalled()
  running = false
  expect(await runEnable(link, () => running)).toEqual({ ok: true })
  expect(link.call.mock.calls.map(([m]) => m)).toEqual(['routing.enable'])

  // the page's command: refused while the setup runs; the disable and other commands go through
  const h = ipcHarness()
  h.d.vpsBusy.mockReturnValue(true)
  expect(await h.invoke(IPC.command, { method: 'routing.enable' })).toEqual({ ok: false, error: SETUP_RUNNING })
  expect(h.d.userCommand).toHaveBeenCalledTimes(1)
  expect(h.d.log).toHaveBeenCalledWith('ipc: routing.enable refused: a server setup is running')
  expect(await h.invoke(IPC.command, { method: 'routing.disable' })).toEqual({ ok: true })
  expect(await h.invoke(IPC.command, { method: 'routing.allDirect', on: true })).toEqual({ ok: true })
  expect(h.link.call.mock.calls.map(([m]) => m)).toEqual(['routing.disable', 'routing.allDirect'])
  h.d.vpsBusy.mockReturnValue(false)
  expect(await h.invoke(IPC.command, { method: 'routing.enable' })).toEqual({ ok: true })
  expect(h.link.call.mock.calls.at(-1)?.[0]).toBe('routing.enable')
})
