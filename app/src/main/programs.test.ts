import { expect, test } from 'vitest'
import type { Program } from '../shared/programs'
import { installedPrograms, pickedFile, resolveDropped, startMenuDirs, type DropReaders, type ProgramsFs } from './programs'

const STARFALL: Program = { exe: 'D:\\Games\\SteamLibrary\\steamapps\\common\\Starfall\\bin\\starfall.exe', name: 'Звёздная тактика' }

/** `.url` files of the tests: a resolved game, a game that is not installed, a web link. */
const URLS: Record<string, Program | 'game' | null> = {
  'C:\\Desk\\Starfall.url': STARFALL,
  'C:\\Desk\\Starfall copy.url': STARFALL,
  'C:\\Desk\\Old Game.url': 'game',
  'C:\\Desk\\Site.url': null,
  'C:\\m\\Games\\Starfall.url': STARFALL,
  'C:\\m\\x.url': null,
}

async function readUrl(p: string): Promise<Program | 'game' | null> {
  if (p === 'C:\\Desk\\Boom.url') throw new Error('x')
  return URLS[p] ?? null
}

test('startMenuDirs builds both folders', () => {
  expect(startMenuDirs({ ProgramData: 'C:\\ProgramData', AppData: 'C:\\Users\\Ann\\AppData\\Roaming' })).toEqual([
    'C:\\ProgramData\\Microsoft\\Windows\\Start Menu\\Programs',
    'C:\\Users\\Ann\\AppData\\Roaming\\Microsoft\\Windows\\Start Menu\\Programs',
  ])
  expect(startMenuDirs({ AppData: 'C:\\A' })).toEqual(['C:\\A\\Microsoft\\Windows\\Start Menu\\Programs'])
})

test('installedPrograms keeps exe targets and game shortcuts, dedupes and sorts', async () => {
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
        return [
          'C:\\m\\Zeta.lnk',
          'C:\\m\\Alpha.lnk',
          'C:\\m\\Doc.lnk',
          'C:\\m\\Broken.lnk',
          'C:\\m\\Folder.lnk',
          'C:\\m\\x.url',
          'C:\\m\\Games\\Starfall.url',
        ]
      }
      return ['C:\\u\\Alpha copy.lnk']
    },
    readShortcut(lnk) {
      const t = targets[lnk]
      if (t === undefined) throw new Error('unknown')
      return t === null ? null : { target: t }
    },
    readUrl,
  }
  expect(await installedPrograms(fs, ['C:\\bad', 'C:\\m', 'C:\\u'])).toEqual([
    // a game shortcut of the Start menu is named after its game
    STARFALL,
    { exe: 'C:\\Apps\\Alpha\\alpha.exe', name: 'Alpha' },
    { exe: 'C:\\Apps\\Zeta\\zeta.exe', name: 'Zeta' },
  ])
})

test('resolves dropped files to programs and skips the rest', async () => {
  const links: Record<string, { target: string } | null> = {
    'C:\\L\\Beta.lnk': { target: 'D:\\Beta\\beta.exe' },
    'C:\\L\\Doc.lnk': { target: 'C:\\Docs\\a.docx' },
    'C:\\L\\Dir.lnk': { target: 'C:\\Docs' },
    'C:\\L\\Dead.lnk': null,
  }
  const readers: DropReaders = {
    readShortcut: (p: string) => {
      if (p === 'C:\\L\\Boom.lnk') throw new Error('x')
      return links[p] ?? null
    },
    readUrl,
  }
  const out = await resolveDropped(
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
    readers,
  )
  expect(out).toEqual([
    { kind: 'program', program: { exe: 'C:\\Apps\\Gamma\\gamma.EXE', name: '' } },
    { kind: 'program', program: { exe: 'D:\\Beta\\beta.exe', name: 'Beta' } },
    { kind: 'skipped', name: 'Doc.lnk', reason: 'notProgram' },
    { kind: 'skipped', name: 'Dir.lnk', reason: 'notProgram' },
    { kind: 'skipped', name: 'Dead.lnk', reason: 'notProgram' },
    { kind: 'skipped', name: 'Boom.lnk', reason: 'notProgram' },
    { kind: 'skipped', name: 'Stuff', reason: 'notProgram' },
    { kind: 'skipped', name: 'run.bat', reason: 'notProgram' },
    { kind: 'skipped', name: 'a.exe', reason: 'notProgram' },
    { kind: 'skipped', name: 'b.exe', reason: 'notProgram' },
  ])
})

test('resolveDropped makes a game program of a game shortcut and names an unresolved one', async () => {
  const readers: DropReaders = { readShortcut: () => null, readUrl }
  expect(
    await resolveDropped(
      [
        'C:\\Desk\\Starfall.url',
        'C:\\Desk\\Old Game.url',
        'C:\\Desk\\Site.url',
        'C:\\Desk\\Boom.url',
        'C:\\Desk\\Starfall copy.url',
        '\\\\server\\share\\Starfall.url',
      ],
      readers,
    ),
  ).toEqual([
    { kind: 'program', program: STARFALL },
    { kind: 'skipped', name: 'Old Game', reason: 'game' },
    { kind: 'skipped', name: 'Site.url', reason: 'notProgram' },
    { kind: 'skipped', name: 'Boom.url', reason: 'notProgram' },
    // the second shortcut of the same game adds nothing; a network path is never read
    { kind: 'skipped', name: 'Starfall.url', reason: 'notProgram' },
  ])
})

test('pickedFile names a picked file that is not a program', async () => {
  const readers: DropReaders = {
    readShortcut: (p: string) => (p === 'C:\\L\\Beta.lnk' ? { target: 'D:\\Beta\\beta.exe' } : null),
    readUrl,
  }
  expect(await pickedFile('C:\\Apps\\Gamma\\gamma.exe', readers)).toEqual({
    kind: 'program',
    program: { exe: 'C:\\Apps\\Gamma\\gamma.exe', name: '' },
  })
  expect(await pickedFile('C:\\L\\Beta.lnk', readers)).toEqual({ kind: 'program', program: { exe: 'D:\\Beta\\beta.exe', name: 'Beta' } })
  expect(await pickedFile('C:\\Desk\\Starfall.url', readers)).toEqual({ kind: 'program', program: STARFALL })
  expect(await pickedFile('C:\\Desk\\Old Game.url', readers)).toEqual({ kind: 'skipped', name: 'Old Game', reason: 'game' })
  // the dialog also hands over network paths: the helper takes only drive paths
  expect(await pickedFile('\\\\server\\share\\tool.exe', readers)).toEqual({ kind: 'skipped', name: 'tool.exe', reason: 'notProgram' })
})
