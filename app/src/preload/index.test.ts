import { beforeEach, expect, test, vi } from 'vitest'
import { IPC } from '../shared/ipc'

const invoke = vi.fn(async (_channel: string, ..._args: unknown[]): Promise<unknown> => undefined)
let exposed: Record<string, (...args: unknown[]) => unknown> = {}

vi.mock('electron', () => ({
  contextBridge: {
    exposeInMainWorld: (_name: string, api: Record<string, (...args: unknown[]) => unknown>) => {
      exposed = api
    }
  },
  ipcRenderer: { invoke: (c: string, ...a: unknown[]) => invoke(c, ...a), on: vi.fn(), removeListener: vi.fn() },
  webUtils: { getPathForFile: () => '' }
}))

beforeEach(() => invoke.mockClear())

test('proxyReveal sends the id on its own channel and returns the answer', async () => {
  await import('./index')
  invoke.mockResolvedValueOnce({ address: 'https://sub.example.com/t' })
  expect(await exposed.proxyReveal('sub-0123456789abcdef')).toEqual({ address: 'https://sub.example.com/t' })
  expect(invoke).toHaveBeenCalledWith(IPC.proxyReveal, 'sub-0123456789abcdef')
  expect(IPC.proxyReveal).toBe('splitlane:proxy-reveal')
})
