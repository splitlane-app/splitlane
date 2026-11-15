import { expect, test } from 'vitest'
import type { Dropped } from '../../shared/programs'
import { explorerDrop, skippedNotice } from './explorerDrop'

const MAIL = 'C:\\Apps\\Mail\\mail.exe'
const GAME = 'C:\\Games\\Game\\game.exe'

test('explorerDrop splits programs and skipped names', async () => {
  const files = [
    new File([''], 'Mail.lnk'),
    new File([''], 'notes.txt'),
    new File([''], 'game.exe'),
    new File([''], 'Папка'),
    new File([''], 'Старая игра.url')
  ]
  const asked: File[][] = []
  const answer: Dropped[] = [
    { kind: 'program', program: { exe: MAIL, name: 'Mail' } },
    { kind: 'skipped', name: 'notes', reason: 'notProgram' },
    { kind: 'program', program: { exe: GAME, name: '' } },
    { kind: 'skipped', name: 'Папка', reason: 'notProgram' },
    { kind: 'skipped', name: 'Старая игра', reason: 'game' }
  ]
  const bridge = {
    resolveDropped: async (f: File[]): Promise<Dropped[]> => {
      asked.push(f)
      return answer
    }
  }

  expect(await explorerDrop(files, 'direct', bridge)).toEqual({
    programs: [
      { exe: MAIL, name: 'Mail' },
      { exe: GAME, name: '' }
    ],
    skipped: [
      { name: 'notes', reason: 'notProgram' },
      { name: 'Папка', reason: 'notProgram' },
      { name: 'Старая игра', reason: 'game' }
    ]
  })
  expect(asked).toEqual([files])

  // a drop with no files asks the main process nothing
  expect(await explorerDrop([], 'vpn', bridge)).toEqual({ programs: [], skipped: [] })
  expect(asked).toHaveLength(1)
})

test('skippedNotice names the files that are no programs and the game shortcuts apart', () => {
  expect(skippedNotice([])).toBeNull()
  expect(
    skippedNotice([
      { name: 'notes.txt', reason: 'notProgram' },
      { name: 'Site.url', reason: 'notProgram' }
    ])
  ).toBe('Не программа: notes.txt, Site.url')
  expect(skippedNotice([{ name: 'Звёздная тактика', reason: 'game' }])).toBe(
    'Звёздная тактика: это ярлык игры — запустите игру, и она появится в «Сейчас в сети»'
  )
  expect(
    skippedNotice([
      { name: 'Звёздная тактика', reason: 'game' },
      { name: 'notes.txt', reason: 'notProgram' },
      { name: 'Nebula Drift', reason: 'game' }
    ])
  ).toBe(
    'Не программа: notes.txt; Звёздная тактика, Nebula Drift: это ярлыки игр — запустите игры, и они появятся в «Сейчас в сети»'
  )
})
