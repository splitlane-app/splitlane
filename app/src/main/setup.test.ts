import { describe, expect, it } from 'vitest'
import {
  HELPER_FILES,
  encodePowerShell,
  helperSourceDir,
  installHelper,
  installScript,
  parseInstallOutput,
  parseSid
} from './setup'
import type { Exec } from './task'

const SID = 'S-1-5-21-1111111111-2222222222-3333333333-1001'
const EXE = 'C:\\Program Files\\SplitLane\\resources\\helper\\splitlane-helper.exe'
const OUT = 'C:\\Users\\u\\AppData\\Local\\Temp\\splitlane-install-1.txt'

describe('setup', () => {
  it('helperSourceDir for packaged and dev', () => {
    expect(
      helperSourceDir({ isPackaged: true, resourcesPath: 'C:\\App\\resources', appPath: 'C:\\App\\resources\\app.asar' })
    ).toBe('C:\\App\\resources\\helper')
    expect(helperSourceDir({ isPackaged: false, resourcesPath: 'x', appPath: 'C:\\code\\router\\app' })).toBe(
      'C:\\code\\router\\dist\\helper'
    )
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

  it('parseInstallOutput ok, refused, failed', () => {
    expect(parseInstallOutput('INSTALL: copying\r\nINSTALL-OK\r\n\r\n')).toEqual({ kind: 'ok' })
    expect(parseInstallOutput('INSTALL-REFUSED: other user\n')).toEqual({ kind: 'refused', reason: 'other user' })
    const f = parseInstallOutput('INSTALL: a\nboom')
    expect(f.kind).toBe('failed')
    expect(f.kind === 'failed' && f.detail).toContain('boom')
    expect(parseInstallOutput('  ').kind).toBe('failed')
  })

  const dir = 'C:\\h'
  const all = async (): Promise<boolean> => true
  const whoami = { code: 0, stdout: `"PC\\u","${SID}"\r\n`, stderr: '' }

  it('installHelper reports missing files', async () => {
    const exec: Exec = async () => {
      throw new Error('must not run')
    }
    const r = await installHelper({
      exec,
      sourceDir: dir,
      tempDir: 'C:\\t',
      readText: async () => '',
      exists: async (p) => !p.endsWith('libcronet.dll')
    })
    expect(r).toEqual({ kind: 'failed', detail: 'missing files: libcronet.dll' })
    expect(HELPER_FILES).toHaveLength(6)
  })

  it('installHelper maps exit 1223 to cancelled', async () => {
    const exec: Exec = async (file) => (file === 'whoami' ? whoami : { code: 1223, stdout: '', stderr: '' })
    const r = await installHelper({ exec, sourceDir: dir, tempDir: 'C:\\t', readText: async () => '', exists: all })
    expect(r).toEqual({ kind: 'cancelled' })
  })

  it('installHelper reads the output file', async () => {
    const calls: Array<[string, string[]]> = []
    const exec: Exec = async (file, args) => {
      calls.push([file, args])
      return file === 'whoami' ? whoami : { code: 0, stdout: '', stderr: '' }
    }
    let read = ''
    const r = await installHelper({
      exec,
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
    expect(calls[0]).toEqual(['whoami', ['/user', '/fo', 'csv', '/nh']])
    expect(calls[1][0]).toBe('powershell.exe')
    const script = Buffer.from(calls[1][1][3], 'base64').toString('utf16le')
    expect(script).toContain('C:\\h\\splitlane-helper.exe')
    expect(script).toContain(SID)

    const bad = await installHelper({
      exec,
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
