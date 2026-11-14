import { expect, test } from 'vitest'
import { installedPrograms, pickedFile, resolveDropped, startMenuDirs, type ProgramsFs } from './programs'

test('startMenuDirs builds both folders', () => {
  expect(startMenuDirs({ ProgramData: 'C:\\ProgramData', AppData: 'C:\\Users\\Ann\\AppData\\Roaming' })).toEqual([
    'C:\\ProgramData\\Microsoft\\Windows\\Start Menu\\Programs',
    'C:\\Users\\Ann\\AppData\\Roaming\\Microsoft\\Windows\\Start Menu\\Programs',
  ])
  expect(startMenuDirs({ AppData: 'C:\\A' })).toEqual(['C:\\A\\Microsoft\\Windows\\Start Menu\\Programs'])
})

test('installedPrograms keeps exe targets, dedupes and sorts', async () => {
  const targets: Record<string, string | null> = {
    'C:\\m\\Zeta.lnk': 'C:\\Apps\\Zeta\\zeta.exe',
    'C:\\m\\Alpha.lnk': 'C:\\Apps\\Alpha\\alpha.exe',
    'C:\\u\\Alpha copy.lnk': 'c:\\apps\\alpha\\ALPHA.EXE',
    'C:\\m\\Doc.lnk': 'C:\\Docs\\readme.txt',
    'C:\\m\\Broken.lnk': null,
    'C:\\m\\Folder.lnk': 'C:\\Apps\\Folder',
  }
  const fs: ProgramsFs = {
    async listFiles(dir) {
      if (dir === 'C:\\bad') throw new Error('nope')
      if (dir === 'C:\\m') {
        return ['C:\\m\\Zeta.lnk', 'C:\\m\\Alpha.lnk', 'C:\\m\\Doc.lnk', 'C:\\m\\Broken.lnk', 'C:\\m\\Folder.lnk', 'C:\\m\\x.url']
      }
      return ['C:\\u\\Alpha copy.lnk']
    },
    readShortcut(lnk) {
      const t = targets[lnk]
      if (t === undefined) throw new Error('unknown')
      return t === null ? null : { target: t }
    },
  }
  expect(await installedPrograms(fs, ['C:\\bad', 'C:\\m', 'C:\\u'])).toEqual([
    { exe: 'C:\\Apps\\Alpha\\alpha.exe', name: 'Alpha' },
    { exe: 'C:\\Apps\\Zeta\\zeta.exe', name: 'Zeta' },
  ])
})

test('resolves dropped files to programs and skips the rest', () => {
  const links: Record<string, { target: string } | null> = {
    'C:\\L\\Beta.lnk': { target: 'D:\\Beta\\beta.exe' },
    'C:\\L\\Doc.lnk': { target: 'C:\\Docs\\a.docx' },
    'C:\\L\\Dir.lnk': { target: 'C:\\Docs' },
    'C:\\L\\Dead.lnk': null,
  }
  const read = (p: string) => {
    if (p === 'C:\\L\\Boom.lnk') throw new Error('x')
    return links[p] ?? null
  }
  const out = resolveDropped(
    [
      'C:\\Apps\\Gamma\\gamma.EXE',
      'C:\\L\\Beta.lnk',
      'C:\\L\\Doc.lnk',
      'C:\\L\\Dir.lnk',
      'C:\\L\\Dead.lnk',
      'C:\\L\\Boom.lnk',
      'C:\\Stuff',
      'C:\\Stuff\\run.bat',
      '\\\\server\\share\\a.exe',
      'rel\\b.exe',
      'c:\\apps\\gamma\\GAMMA.exe',
    ],
    read,
  )
  expect(out).toEqual([
    { kind: 'program', program: { exe: 'C:\\Apps\\Gamma\\gamma.EXE', name: '' } },
    { kind: 'program', program: { exe: 'D:\\Beta\\beta.exe', name: 'Beta' } },
    { kind: 'skipped', name: 'Doc.lnk' },
    { kind: 'skipped', name: 'Dir.lnk' },
    { kind: 'skipped', name: 'Dead.lnk' },
    { kind: 'skipped', name: 'Boom.lnk' },
    { kind: 'skipped', name: 'Stuff' },
    { kind: 'skipped', name: 'run.bat' },
    { kind: 'skipped', name: 'a.exe' },
    { kind: 'skipped', name: 'b.exe' },
  ])
})

test('pickedFile names a picked file that is not a program', () => {
  const read = (p: string) => (p === 'C:\\L\\Beta.lnk' ? { target: 'D:\\Beta\\beta.exe' } : null)
  expect(pickedFile('C:\\Apps\\Gamma\\gamma.exe', read)).toEqual({
    kind: 'program',
    program: { exe: 'C:\\Apps\\Gamma\\gamma.exe', name: '' },
  })
  expect(pickedFile('C:\\L\\Beta.lnk', read)).toEqual({ kind: 'program', program: { exe: 'D:\\Beta\\beta.exe', name: 'Beta' } })
  // the dialog also hands over network paths: the helper takes only drive paths
  expect(pickedFile('\\\\server\\share\\tool.exe', read)).toEqual({ kind: 'skipped', name: 'tool.exe' })
})
