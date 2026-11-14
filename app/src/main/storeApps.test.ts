import { expect, test, vi } from 'vitest'
import { parseStoreApps, StoreAppsCache, STORE_APPS_SCRIPT } from './storeApps'

const PKG = 'C:\\Program Files\\WindowsApps\\Fake.Notes_1.2.0.0_x64__abc123\\notes.exe'

test('parseStoreApps keeps package executables only', () => {
  const json = JSON.stringify([
    { name: 'Notes', exe: PKG },
    { name: 'Notes again', exe: PKG.toUpperCase() },
    { name: 'Plain', exe: 'C:\\Apps\\plain.exe' },
    { name: '', exe: PKG.replace('notes', 'other') },
    { name: 'NoExe', exe: '' },
  ])
  expect(parseStoreApps(json)).toEqual([{ name: 'Notes', exe: PKG }])
  expect(parseStoreApps(JSON.stringify({ name: 'One', exe: PKG }))).toEqual([{ name: 'One', exe: PKG }])
  expect(parseStoreApps('')).toEqual([])
  expect(parseStoreApps('not json')).toEqual([])
})

test('script is ASCII and reads the manifest', () => {
  expect(/^[\x20-\x7e\r\n\t]*$/.test(STORE_APPS_SCRIPT)).toBe(true)
  expect(STORE_APPS_SCRIPT).toContain('Get-StartApps')
  expect(STORE_APPS_SCRIPT).toContain('AppxManifest.xml')
})

test('StoreAppsCache calls PowerShell once and survives a failure', async () => {
  const load = vi.fn(async () => JSON.stringify([{ name: 'Notes', exe: PKG }]))
  const cache = new StoreAppsCache(load)
  const [a, b] = await Promise.all([cache.get(), cache.get()])
  expect(a).toEqual([{ name: 'Notes', exe: PKG }])
  expect(b).toBe(a)
  await cache.get()
  expect(load).toHaveBeenCalledTimes(1)

  const bad = vi.fn(async (): Promise<string> => {
    throw new Error('powershell failed')
  })
  const failing = new StoreAppsCache(bad)
  expect(await failing.get()).toEqual([])
  expect(await failing.get()).toEqual([])
  expect(bad).toHaveBeenCalledTimes(1)
})
