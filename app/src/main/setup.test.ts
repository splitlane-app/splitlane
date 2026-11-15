import { describe, expect, it } from 'vitest'
import type { InstallState } from '../shared/ipc'
import type { LinkStatus } from '../shared/status'
import {
  HELPER_FILES,
  canInstall,
  encodePowerShell,
  helperSourceDir,
  installAfterLink,
  installHelper,
  installScript,
  installedAt,
  parseInstallOutput,
  parseSid
} from './setup'
import { systemTools, type Exec } from './task'

const SID = 'S-1-5-21-1111111111-2222222222-3333333333-1001'
const EXE = 'C:\\Program Files\\SplitLane\\resources\\helper\\splitlane-helper.exe'
const OUT = 'C:\\Users\\u\\AppData\\Local\\Temp\\splitlane-install-1.txt'

describe('setup', () => {
  it('helperSourceDir for packaged and dev', () => {
    expect(
      helperSourceDir({
        isPackaged: true,
        execPath: 'C:\\Program Files\\SplitLane\\SplitLane.exe',
        appPath: 'C:\\Program Files\\SplitLane\\resources\\app.asar'
      })
    ).toBe('C:\\Program Files\\SplitLane\\helper')
    expect(helperSourceDir({ isPackaged: false, execPath: 'x', appPath: 'C:\\code\\router\\app' })).toBe(
      'C:\\code\\router\\dist\\helper'
    )
  })

  it('installedAt inside, outside, prefix, case, empty', () => {
    const pf = 'C:\\Program Files'
    expect(installedAt('C:\\Program Files\\SplitLane\\SplitLane.exe', pf)).toBe(true)
    expect(installedAt('C:\\code\\router\\app\\release\\win-unpacked\\SplitLane.exe', pf)).toBe(false)
    expect(installedAt('C:\\Program Files\\SplitLaneX\\SplitLane.exe', pf)).toBe(false)
    expect(installedAt('c:\\program files\\splitlane\\SplitLane.exe', 'C:\\PROGRAM FILES')).toBe(true)
    expect(installedAt('C:\\Program Files\\SplitLane\\SplitLane.exe', undefined)).toBe(false)
    expect(installedAt('C:\\Program Files\\SplitLane\\SplitLane.exe', '')).toBe(false)
    // only ASCII letters fold: a dotted capital I is not an i
    expect(installedAt('C:\\Program Files\\SplitLane\\SplitLane.exe', 'C:\\Program F\u0130les')).toBe(false)
    expect(installedAt('C:\\Program F\u0130les\\SplitLane\\SplitLane.exe', 'C:\\Program Files')).toBe(false)
  })

  it('canInstall refuses a packaged copy outside the install folder', () => {
    const pf = 'C:\\Program Files'
    expect(canInstall(true, 'C:\\Program Files\\SplitLane\\SplitLane.exe', pf)).toBe(true)
    expect(canInstall(true, 'C:\\x\\win-unpacked\\SplitLane.exe', pf)).toBe(false)
    expect(canInstall(true, 'C:\\Program Files\\SplitLane\\SplitLane.exe', undefined)).toBe(false)
    expect(canInstall(false, 'C:\\x\\electron.exe', undefined)).toBe(true)
  })

  it('parseSid takes the SID from whoami csv', () => {
    expect(parseSid(`"DESKTOP\\user","${SID}"\r\n`)).toBe(SID)
  })

  it('parseSid rejects garbage', () => {
    expect(() => parseSid('')).toThrow(Error)
    expect(() => parseSid('"a","S-1-5"')).toThrow(Error)
    expect(() => parseSid('ERROR: nope')).toThrow(Error)
  })

  it('installScript quotes paths and maps a cancelled UAC to 1223', () => {
    const s = installScript(EXE, SID, OUT)
    expect(s).toContain(
      `Start-Process -FilePath cmd.exe -ArgumentList '/c', '""${EXE}" install --user-sid ${SID} > "${OUT}" 2>&1"' -Verb RunAs -Wait -WindowStyle Hidden -ErrorAction Stop`
    )
    expect(s).toContain('exit 1223')
    expect(s).toContain('exit 0')
    expect(s).toMatch(/try\s*\{/)
    expect(s).toMatch(/catch/)
  })

  it('installScript keeps a Cyrillic path and refuses %', () => {
    const exe = 'C:\\Users\\Иван\\app\\splitlane-helper.exe'
    const out = 'C:\\Users\\Иван\\AppData\\Local\\Temp\\o.txt'
    const script = Buffer.from(encodePowerShell(installScript(exe, SID, out))[3], 'base64').toString('utf16le')
    expect(script).toContain(exe)
    expect(script).toContain(out)
    expect(() => installScript('C:\\%TEMP%\\x.exe', SID, OUT)).toThrow(Error)
  })

  it('installScript distinguishes a cancelled UAC from other failures', () => {
    const s = installScript(EXE, SID, OUT)
    expect(s).toContain('NativeErrorCode')
    expect(s).toContain('InnerException')
    expect(s).toMatch(/exit 1\r?\n/)
  })

  it('installScript refuses quotes in paths', () => {
    expect(() => installScript('C:\\a"b\\x.exe', SID, OUT)).toThrow(Error)
    expect(() => installScript(EXE, SID, "C:\\a'b.txt")).toThrow(Error)
    expect(() => installScript('relative\\x.exe', SID, OUT)).toThrow(Error)
    expect(() => installScript(EXE, 'S-1-bad', OUT)).toThrow(Error)
  })

  it('encodePowerShell is UTF-16LE base64', () => {
    const a = encodePowerShell('exit 0')
    expect(a.slice(0, 3)).toEqual(['-NoProfile', '-NonInteractive', '-EncodedCommand'])
    expect(Buffer.from(a[3], 'base64').toString('utf16le')).toBe('exit 0')
  })

  it('installAfterLink keeps the outcome until the link is ready, then starts afresh', () => {
    const ready: LinkStatus = { kind: 'ready', hello: { protocol: 1, helper: '0.1.0', core: '1.14.4' } }
    const done: InstallState = { kind: 'done', outcome: { kind: 'ok' } }
    // «Готово, служебный процесс запускается…» stays while the link connects
    expect(installAfterLink(done, { kind: 'connecting' })).toBe(done)
    expect(installAfterLink(done, { kind: 'setup-needed' })).toBe(done)
    expect(installAfterLink(done, ready)).toEqual({ kind: 'idle' })
    const failed: InstallState = { kind: 'done', outcome: { kind: 'failed', detail: 'x' } }
    expect(installAfterLink(failed, ready)).toEqual({ kind: 'idle' })
    const running: InstallState = { kind: 'running' }
    expect(installAfterLink(running, ready)).toBe(running)
    const idle: InstallState = { kind: 'idle' }
    expect(installAfterLink(idle, ready)).toBe(idle)
  })

  it('parseInstallOutput ok, refused, failed', () => {
    expect(parseInstallOutput('INSTALL: copying\r\nINSTALL-OK\r\n\r\n')).toEqual({ kind: 'ok' })
    expect(parseInstallOutput('INSTALL-REFUSED: other user\n')).toEqual({ kind: 'refused', reason: 'other user' })
    const f = parseInstallOutput('INSTALL: a\nboom')
    expect(f.kind).toBe('failed')
    expect(f.kind === 'failed' && f.detail).toContain('boom')
    expect(parseInstallOutput('  ').kind).toBe('failed')
  })

  const dir = 'C:\\h'
  const tools = systemTools({ SystemRoot: 'D:\\Win' })
  const all = async (): Promise<boolean> => true
  const whoami = { code: 0, stdout: `"PC\\u","${SID}"\r\n`, stderr: '' }

  it('installHelper reports missing files', async () => {
    const exec: Exec = async () => {
      throw new Error('must not run')
    }
    const r = await installHelper({
      exec,
      tools,
      sourceDir: dir,
      tempDir: 'C:\\t',
      readText: async () => '',
      exists: async (p) => !p.endsWith('libcronet.dll')
    })
    expect(r).toEqual({ kind: 'failed', detail: 'missing files: libcronet.dll' })
    expect(HELPER_FILES).toHaveLength(6)
  })

  it('installHelper maps exit 1223 to cancelled', async () => {
    const exec: Exec = async (file) => (file === tools.whoami ? whoami : { code: 1223, stdout: '', stderr: '' })
    const r = await installHelper({ exec, tools, sourceDir: dir, tempDir: 'C:\\t', readText: async () => '', exists: all })
    expect(r).toEqual({ kind: 'cancelled' })
  })

  it('installHelper reports a PowerShell failure with its stderr', async () => {
    const exec: Exec = async (file) => (file === tools.whoami ? whoami : { code: 1, stdout: '', stderr: 'blocked by policy' })
    const r = await installHelper({ exec, tools, sourceDir: dir, tempDir: 'C:\\t', readText: async () => '', exists: all })
    expect(r.kind).toBe('failed')
    expect(r.kind === 'failed' && r.detail).toContain('blocked by policy')
  })

  it('installHelper removes the output file', async () => {
    const exec: Exec = async (file) => (file === tools.whoami ? whoami : { code: 0, stdout: '', stderr: '' })
    const removed: string[] = []
    await installHelper({
      exec,
      tools,
      sourceDir: dir,
      tempDir: 'C:\\t',
      readText: async () => 'INSTALL-OK',
      exists: all,
      remove: async (p) => {
        removed.push(p)
        throw new Error('ignored')
      }
    })
    expect(removed).toHaveLength(1)
    expect(removed[0]).toMatch(/splitlane-install-.+\.txt$/)
  })

  it('installHelper reads the output file', async () => {
    const calls: Array<[string, string[]]> = []
    const exec: Exec = async (file, args) => {
      calls.push([file, args])
      return file === tools.whoami ? whoami : { code: 0, stdout: '', stderr: '' }
    }
    let read = ''
    const r = await installHelper({
      exec,
      tools,
      sourceDir: dir,
      tempDir: 'C:\\t',
      readText: async (p) => {
        read = p
        return 'INSTALL-OK\n'
      },
      exists: all
    })
    expect(r).toEqual({ kind: 'ok' })
    expect(read).toMatch(/^C:\\t\\splitlane-install-.+\.txt$/)
    // by full path, never through PATH
    expect(calls[0]).toEqual(['D:\\Win\\System32\\whoami.exe', ['/user', '/fo', 'csv', '/nh']])
    expect(calls[1][0]).toBe('D:\\Win\\System32\\WindowsPowerShell\\v1.0\\powershell.exe')
    const script = Buffer.from(calls[1][1][3], 'base64').toString('utf16le')
    expect(script).toContain('C:\\h\\splitlane-helper.exe')
    expect(script).toContain(SID)

    const bad = await installHelper({
      exec,
      tools,
      sourceDir: dir,
      tempDir: 'C:\\t',
      readText: async () => {
        throw new Error('ENOENT')
      },
      exists: all
    })
    expect(bad.kind).toBe('failed')
  })
})
