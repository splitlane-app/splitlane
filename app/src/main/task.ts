import { win32 } from 'node:path'
import { TASK_NAME } from '../shared/protocol'

/** Runs a program without a shell and reports its exit code and output. */
export interface Exec {
  (file: string, args: string[]): Promise<{ code: number; stdout: string; stderr: string }>
}

/** The programs of Windows the window runs, by full path: never looked up through PATH. */
export interface SystemTools {
  schtasks: string
  whoami: string
  powershell: string
}

const DEFAULT_SYSTEM_ROOT = 'C:\\Windows'
const DRIVE_PATH = /^[A-Za-z]:\\/

/** The tools under %SystemRoot%\System32; C:\Windows when SystemRoot is not set or is no absolute drive path. */
export function systemTools(env: Record<string, string | undefined>): SystemTools {
  const root = env['SystemRoot']
  const base = root !== undefined && DRIVE_PATH.test(root) ? root : DEFAULT_SYSTEM_ROOT
  const system32 = win32.join(base, 'System32')
  return {
    schtasks: win32.join(system32, 'schtasks.exe'),
    whoami: win32.join(system32, 'whoami.exe'),
    powershell: win32.join(system32, 'WindowsPowerShell', 'v1.0', 'powershell.exe')
  }
}

/** `schtasks /query /tn "SplitLane Helper"`: exit code 0 means the task exists. */
export async function taskRegistered(exec: Exec, tools: Pick<SystemTools, 'schtasks'>): Promise<boolean> {
  const r = await exec(tools.schtasks, ['/query', '/tn', TASK_NAME])
  return r.code === 0
}

/** `schtasks /run /tn "SplitLane Helper"` (no UAC needed); a non-zero exit code throws with the output. */
export async function runTask(exec: Exec, tools: Pick<SystemTools, 'schtasks'>): Promise<void> {
  const r = await exec(tools.schtasks, ['/run', '/tn', TASK_NAME])
  if (r.code === 0) return
  const output = [r.stdout.trim(), r.stderr.trim()].filter((s) => s !== '').join('\n')
  throw new Error(`schtasks /run /tn "${TASK_NAME}" failed with code ${r.code}${output === '' ? '' : `: ${output}`}`)
}
