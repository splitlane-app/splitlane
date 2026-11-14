import { expect, test, vi, type Mock } from 'vitest'
import { runTask, taskRegistered, type Exec } from './task'

function fakeExec(code: number, stdout = '', stderr = ''): Mock<Exec> {
  return vi.fn<Exec>(async () => ({ code, stdout, stderr }))
}

test('taskRegistered reads the exit code', async () => {
  const yes = fakeExec(0, 'Folder: \\\r\nSplitLane Helper  N/A  Ready\r\n')
  expect(await taskRegistered(yes)).toBe(true)
  expect(yes).toHaveBeenCalledWith('schtasks.exe', ['/query', '/tn', 'SplitLane Helper'])

  const no = fakeExec(1, '', 'ERROR: The system cannot find the file specified.\r\n')
  expect(await taskRegistered(no)).toBe(false)
})

test('runTask throws with the output on failure', async () => {
  const ok = fakeExec(0, 'SUCCESS: Attempted to run the scheduled task "SplitLane Helper".\r\n')
  await expect(runTask(ok)).resolves.toBeUndefined()
  expect(ok).toHaveBeenCalledWith('schtasks.exe', ['/run', '/tn', 'SplitLane Helper'])

  const bad = fakeExec(1, 'partial output', 'ERROR: The system cannot find the file specified.')
  const e = await runTask(bad).catch((x: unknown) => x)
  expect(e).toBeInstanceOf(Error)
  expect((e as Error).message).toContain('code 1')
  expect((e as Error).message).toContain('partial output')
  expect((e as Error).message).toContain('ERROR: The system cannot find the file specified.')
})
