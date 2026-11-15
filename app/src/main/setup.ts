import { randomBytes } from 'node:crypto'
import { win32 } from 'node:path'
import type { InstallState } from '../shared/ipc'
import type { InstallOutcome, LinkStatus } from '../shared/status'
import type { Exec, SystemTools } from './task'

/** The six files that must sit next to `splitlane-helper.exe` for `install`. */
export const HELPER_FILES = [
  'splitlane-helper.exe',
  'sing-box.exe',
  'libcronet.dll',
  'sing-box-LICENSE',
  'LICENSE',
  'THIRD-PARTY.md'
] as const

const SID_RE = /^S-1-\d+(-\d+)+$/
const UAC_CANCELLED = 1223

/** Where the helper files live: `helper` next to the window's exe when packaged, `dist\helper` of the repo in development. */
export function helperSourceDir(o: { isPackaged: boolean; execPath: string; appPath: string }): string {
  return o.isPackaged ? win32.join(win32.dirname(o.execPath), 'helper') : win32.resolve(o.appPath, '..', 'dist', 'helper')
}

/** True when the exe lies inside `<programFiles>\SplitLane\` (case ignored, on a folder boundary); no Program Files: false. */
export function installedAt(execPath: string, programFiles: string | undefined): boolean {
  if (programFiles === undefined || programFiles.trim() === '') return false
  const dir = foldAscii(win32.join(programFiles, 'SplitLane')).replace(/\\+$/, '')
  return foldAscii(execPath).startsWith(`${dir}\\`)
}

/** A-Z to a-z only: any other character (Unicode case mapping included) is compared exactly. */
function foldAscii(s: string): string {
  return s.replace(/[A-Z]/g, (c) => String.fromCharCode(c.charCodeAt(0) + 32))
}

/** The install runs only in development or from the installed folder: a copy elsewhere must not start UAC. */
export function canInstall(isPackaged: boolean, execPath: string, programFiles: string | undefined): boolean {
  return !isPackaged || installedAt(execPath, programFiles)
}

export const NOT_INSTALLED_REASON = 'this copy of SplitLane is not installed: run the SplitLane installer'

/** Takes the SID from `whoami /user /fo csv /nh` output (`"DOMAIN\user","S-1-5-..."`). */
export function parseSid(whoamiCsv: string): string {
  const m = /"(S-[^"]*)"/.exec(whoamiCsv)
  const sid = m === null ? '' : m[1]
  if (!SID_RE.test(sid)) throw new Error(`cannot find the user SID in whoami output: ${whoamiCsv.trim().slice(0, 200)}`)
  return sid
}

export function checkPath(p: string): void {
  if (p.includes('"')) throw new Error(`path must not contain double quotes: ${p}`)
  if (/[\u0000-\u001f\u007f]/.test(p)) throw new Error('path must not contain control characters')
  if (!/^[A-Za-z]:\\/.test(p)) throw new Error(`path must be absolute: ${p}`)
  if (p.includes('%')) throw new Error(`path must not contain %: ${p}`)
}

/** A PowerShell single-quoted string: U+0027 and U+2018..U+201B are all single-quote characters to PowerShell, so each is doubled. */
export function psSingleQuoted(s: string): string {
  return `'${s.replace(/['‘-‛]/g, (c) => c + c)}'`
}

/** Lines of PowerShell's stderr without the CLIXML serialization (`#< CLIXML` header, `<Objs ...>` progress records). */
export function stderrLines(stderr: string): string[] {
  return stderr
    .split(/\r?\n/)
    .map((l) => l.trim())
    .filter((l) => l !== '' && !l.startsWith('<') && !l.startsWith('#<'))
}

/** PowerShell lines that turn a failed launch into exit 1223 (a cancelled UAC) or exit 1 with the message on stderr. */
export function launchCatch(uacCancelled: number): string[] {
  return [
    '} catch {',
    '  $e = $_.Exception',
    '  while ($null -ne $e) {',
    `    if (($e -is [System.ComponentModel.Win32Exception]) -and ($e.NativeErrorCode -eq ${uacCancelled})) { exit ${uacCancelled} }`,
    '    $e = $e.InnerException',
    '  }',
    '  [Console]::Error.WriteLine($_.Exception.Message)',
    '  exit 1',
    '}'
  ]
}

/**
 * PowerShell text that runs `<exe> install --user-sid <sid>` elevated (UAC), with output redirected to `outFile`.
 * The whole cmd line is wrapped in one more pair of quotes because `cmd /c` strips the first and last one.
 * A cancelled UAC prompt makes Start-Process throw: exit 1223.
 */
export function installScript(exe: string, sid: string, outFile: string): string {
  checkPath(exe)
  checkPath(outFile)
  if (!SID_RE.test(sid)) throw new Error(`invalid SID: ${sid}`)
  const cmd = `""${exe}" install --user-sid ${sid} > "${outFile}" 2>&1"`
  return [
    "$ProgressPreference = 'SilentlyContinue'",
    'try {',
    '  $psi = New-Object System.Diagnostics.ProcessStartInfo',
    "  $psi.FileName = 'cmd.exe'",
    `  $psi.Arguments = ${psSingleQuoted(`/c ${cmd}`)}`,
    '  $psi.UseShellExecute = $true',
    "  $psi.Verb = 'runas'",
    '  $psi.WindowStyle = [System.Diagnostics.ProcessWindowStyle]::Hidden',
    '  $p = [System.Diagnostics.Process]::Start($psi)',
    '  if ($null -eq $p) { throw "the installer process was not started" }',
    '  $p.WaitForExit()',
    ...launchCatch(UAC_CANCELLED),
    'exit 0',
    ''
  ].join('\r\n')
}

/** Arguments for powershell.exe that run `script` without quoting problems. */
export function encodePowerShell(script: string): string[] {
  return ['-NoProfile', '-NonInteractive', '-EncodedCommand', Buffer.from(script, 'utf16le').toString('base64')]
}

/**
 * The install state after a new link status: a finished install stays on screen («Готово…», or why it failed)
 * until the link is ready, then it is idle again, so that a setup needed later in the same run starts afresh.
 */
export function installAfterLink(install: InstallState, link: LinkStatus): InstallState {
  return link.kind === 'ready' && install.kind === 'done' ? { kind: 'idle' } : install
}

/** The last non-empty line decides: `INSTALL-OK`, `INSTALL-REFUSED: <reason>`, anything else is a failure. */
export function parseInstallOutput(text: string): InstallOutcome {
  const lines = text
    .split(/\r?\n/)
    .map((l) => l.trim())
    .filter((l) => l !== '')
  const last = lines.length === 0 ? '' : lines[lines.length - 1]
  if (last === 'INSTALL-OK') return { kind: 'ok' }
  if (last.startsWith('INSTALL-REFUSED:')) return { kind: 'refused', reason: last.slice('INSTALL-REFUSED:'.length).trim() }
  return { kind: 'failed', detail: lines.length === 0 ? 'the installer produced no output' : lines.slice(-10).join('\n') }
}

function tail(s: string): string {
  return s.trim().split(/\r?\n/).slice(-10).join('\n')
}

/** Installs the helper: checks the files, finds the user SID, runs `install` elevated and parses its output. */
export async function installHelper(d: {
  exec: Exec
  /** whoami and powershell by full path (systemTools). */
  tools: Pick<SystemTools, 'whoami' | 'powershell'>
  sourceDir: string
  tempDir: string
  readText(path: string): Promise<string>
  exists(path: string): Promise<boolean>
  remove?(path: string): Promise<void>
}): Promise<InstallOutcome> {
  const missing: string[] = []
  for (const f of HELPER_FILES) {
    if (!(await d.exists(win32.join(d.sourceDir, f)))) missing.push(f)
  }
  if (missing.length > 0) return { kind: 'failed', detail: `missing files: ${missing.join(', ')}` }

  let sid: string
  try {
    const w = await d.exec(d.tools.whoami, ['/user', '/fo', 'csv', '/nh'])
    if (w.code !== 0) return { kind: 'failed', detail: `whoami failed with code ${w.code}: ${tail(w.stderr || w.stdout)}` }
    sid = parseSid(w.stdout)
  } catch (e) {
    return { kind: 'failed', detail: e instanceof Error ? e.message : String(e) }
  }

  const outFile = win32.join(d.tempDir, `splitlane-install-${randomBytes(6).toString('hex')}.txt`)
  try {
    let r: { code: number; stdout: string; stderr: string }
    try {
      const script = installScript(win32.join(d.sourceDir, 'splitlane-helper.exe'), sid, outFile)
      r = await d.exec(d.tools.powershell, encodePowerShell(script))
    } catch (e) {
      return { kind: 'failed', detail: e instanceof Error ? e.message : String(e) }
    }
    if (r.code === UAC_CANCELLED) return { kind: 'cancelled' }
    if (r.code !== 0) return { kind: 'failed', detail: `powershell failed with code ${r.code}: ${tail(stderrLines(r.stderr).join('\n') || r.stdout)}` }

    try {
      return parseInstallOutput(await d.readText(outFile))
    } catch (e) {
      return { kind: 'failed', detail: `cannot read the installer output: ${e instanceof Error ? e.message : String(e)}` }
    }
  } finally {
    try {
      await d.remove?.(outFile)
    } catch {
      // best effort
    }
  }
}
