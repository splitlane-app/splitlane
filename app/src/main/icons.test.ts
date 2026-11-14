import { createHash } from 'node:crypto'
import { expect, test, vi } from 'vitest'
import { IconCache, type IconSource } from './icons'

function memFs(mtime: number | null = 5) {
  const files = new Map<string, Buffer>()
  return {
    files,
    stat: vi.fn(async () => (mtime === null ? null : { mtimeMs: mtime })),
    read: vi.fn(async (p: string) => files.get(p) ?? null),
    write: vi.fn(async (p: string, b: Buffer) => {
      files.set(p, b)
    }),
  }
}
const PNG = Buffer.from([1, 2, 3, 4])
const URL = `data:image/png;base64,${PNG.toString('base64')}`

test('IconCache reads from disk after the first call', async () => {
  const fs = memFs()
  const source = vi.fn<IconSource>(async () => ({ png: PNG }))
  const cache = new IconCache('C:\\cache', source, fs)
  expect(await cache.get('C:\\Apps\\A.exe')).toBe(URL)
  const key = createHash('sha1').update('c:\\apps\\a.exe|5').digest('hex')
  expect([...fs.files.keys()]).toEqual([`C:\\cache\\${key}.png`])
  const again = new IconCache('C:\\cache', source, fs)
  expect(await again.get('C:\\Apps\\A.exe')).toBe(URL)
  expect(source).toHaveBeenCalledTimes(1)
})

test('IconCache shares one source call for concurrent requests', async () => {
  const fs = memFs()
  const source = vi.fn<IconSource>(async () => ({ png: PNG }))
  const cache = new IconCache('C:\\cache', source, fs)
  const r = await Promise.all([cache.get('C:\\a.exe'), cache.get('C:\\a.exe')])
  expect(r).toEqual([URL, URL])
  expect(source).toHaveBeenCalledTimes(1)
})

test('IconCache returns null without an icon', async () => {
  const fs = memFs()
  const source = vi.fn<IconSource>(async () => null)
  const cache = new IconCache('C:\\cache', source, fs)
  expect(await cache.get('C:\\a.exe')).toBeNull()
  expect(fs.write).not.toHaveBeenCalled()
  const missing = memFs(null)
  const s2 = vi.fn<IconSource>(async () => ({ png: PNG }))
  expect(await new IconCache('C:\\cache', s2, missing).get('C:\\gone.exe')).toBeNull()
  expect(s2).not.toHaveBeenCalled()
})
