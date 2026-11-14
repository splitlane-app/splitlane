import { expect, test } from 'vitest'
import type { Dropped } from '../../shared/programs'
import { explorerDrop } from './explorerDrop'

const MAIL = 'C:\\Apps\\Mail\\mail.exe'
const GAME = 'C:\\Games\\Game\\game.exe'

test('explorerDrop splits programs and skipped names', async () => {
  const files = [
    new File([''], 'Mail.lnk'),
    new File([''], 'notes.txt'),
    new File([''], 'game.exe'),
    new File([''], 'Папка')
  ]
  const asked: File[][] = []
  const answer: Dropped[] = [
    { kind: 'program', program: { exe: MAIL, name: 'Mail' } },
    { kind: 'skipped', name: 'notes' },
    { kind: 'program', program: { exe: GAME, name: '' } },
    { kind: 'skipped', name: 'Папка' }
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
    skipped: ['notes', 'Папка']
  })
  expect(asked).toEqual([files])

  // a drop with no files asks the main process nothing
  expect(await explorerDrop([], 'vpn', bridge)).toEqual({ programs: [], skipped: [] })
  expect(asked).toHaveLength(1)
})
