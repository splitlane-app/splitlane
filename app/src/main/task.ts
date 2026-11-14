import { TASK_NAME } from '../shared/protocol'

/** Runs a program without a shell and reports its exit code and output. */
export interface Exec {
  (file: string, args: string[]): Promise<{ code: number; stdout: string; stderr: string }>
}

const SCHTASKS = 'schtasks.exe'

/** `schtasks /query /tn "SplitLane Helper"`: exit code 0 means the task exists. */
export async function taskRegistered(exec: Exec): Promise<boolean> {
  const r = await exec(SCHTASKS, ['/query', '/tn', TASK_NAME])
  return r.code === 0
}

/** `schtasks /run /tn "SplitLane Helper"` (no UAC needed); a non-zero exit code throws with the output. */
export async function runTask(exec: Exec): Promise<void> {
  const r = await exec(SCHTASKS, ['/run', '/tn', TASK_NAME])
  if (r.code === 0) return
  const output = [r.stdout.trim(), r.stderr.trim()].filter((s) => s !== '').join('\n')
  throw new Error(`schtasks /run /tn "${TASK_NAME}" failed with code ${r.code}${output === '' ? '' : `: ${output}`}`)
}
