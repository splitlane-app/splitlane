import { expect, test, vi, type Mock } from 'vitest'
import { runTask, systemTools, taskRegistered, type Exec } from './task'

function fakeExec(code: number, stdout = '', stderr = ''): Mock<Exec> {
  return vi.fn<Exec>(async () => ({ code, stdout, stderr }))
}

const TOOLS = systemTools({ SystemRoot: 'D:\\Win' })

test('systemTools names the programs of Windows by full path under SystemRoot', () => {
  expect(TOOLS).toEqual({
    schtasks: 'D:\\Win\\System32\\schtasks.exe',
    whoami: 'D:\\Win\\System32\\whoami.exe',
    reg: 'D:\\Win\\System32\\reg.exe',
    powershell: 'D:\\Win\\System32\\WindowsPowerShell\\v1.0\\powershell.exe'
  })
  expect(systemTools({ SystemRoot: 'C:\\WINDOWS\\' }).schtasks).toBe('C:\\WINDOWS\\System32\\schtasks.exe')
  // without SystemRoot, or with one that is no absolute path, the usual place
  for (const env of [{}, { SystemRoot: '' }, { SystemRoot: 'Windows' }, { SystemRoot: '\\\\server\\share' }]) {
    expect(systemTools(env).schtasks).toBe('C:\\Windows\\System32\\schtasks.exe')
  }
})

test('taskRegistered reads the exit code', async () => {
  const yes = fakeExec(0, 'Folder: \\\r\nSplitLane Helper  N/A  Ready\r\n')
  expect(await taskRegistered(yes, TOOLS)).toBe(true)
  expect(yes).toHaveBeenCalledWith('D:\\Win\\System32\\schtasks.exe', ['/query', '/tn', 'SplitLane Helper'])

  const no = fakeExec(1, '', 'ERROR: The system cannot find the file specified.\r\n')
  expect(await taskRegistered(no, TOOLS)).toBe(false)
})

test('runTask throws with the output on failure', async () => {
  const ok = fakeExec(0, 'SUCCESS: Attempted to run the scheduled task "SplitLane Helper".\r\n')
  await expect(runTask(ok, TOOLS)).resolves.toBeUndefined()
  expect(ok).toHaveBeenCalledWith('D:\\Win\\System32\\schtasks.exe', ['/run', '/tn', 'SplitLane Helper'])

  const bad = fakeExec(1, 'partial output', 'ERROR: The system cannot find the file specified.')
  const e = await runTask(bad, TOOLS).catch((x: unknown) => x)
  expect(e).toBeInstanceOf(Error)
  expect((e as Error).message).toContain('code 1')
  expect((e as Error).message).toContain('partial output')
  expect((e as Error).message).toContain('ERROR: The system cannot find the file specified.')
})
