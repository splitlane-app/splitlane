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

test('the VPS calls go on their own channels; the key crosses nowhere and the event push is a subscription', async () => {
  await import('./index')
  invoke.mockResolvedValueOnce({ name: 'id_ed25519' })
  expect(await exposed.vpsPickKey()).toEqual({ name: 'id_ed25519' })
  expect(invoke).toHaveBeenLastCalledWith(IPC.vpsPickKey)
  await exposed.vpsForgetKey()
  expect(invoke).toHaveBeenLastCalledWith(IPC.vpsForgetKey)
  const req = { address: '198.51.100.7', user: 'root', password: 'pw', useKey: false }
  await exposed.vpsSetup(req)
  expect(invoke).toHaveBeenLastCalledWith(IPC.vpsSetup, req)
  await exposed.vpsCancel()
  expect(invoke).toHaveBeenLastCalledWith(IPC.vpsCancel)
  expect(IPC.vpsEvent).toBe('splitlane:vps-event')
  expect(typeof exposed.onVpsEvent(() => {})).toBe('function')
})
