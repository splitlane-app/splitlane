import { expect, test } from 'vitest'
import { MAIN_TEXT, mainText } from './mainText'

test('tray menu texts per language', () => {
  expect(mainText('ru').open).toBe('Открыть')
  expect(mainText('ru').quit).toBe('Выход')
  const en = mainText('en')
  expect([en.open, en.settings, en.enable, en.disable, en.checkUpdates, en.quit]).toEqual([
    'Open',
    'Settings…',
    'Turn on',
    'Turn off',
    'Check for updates',
    'Quit'
  ])
  expect([en.programsFilter, en.zipFilter]).toEqual(['Programs', 'ZIP archive'])
})

test('both languages have the same keys and no empty text', () => {
  expect(Object.keys(MAIN_TEXT.en).sort()).toEqual(Object.keys(MAIN_TEXT.ru).sort())
  for (const lang of ['ru', 'en'] as const) {
    for (const v of Object.values(MAIN_TEXT[lang])) expect(v.length).toBeGreaterThan(0)
  }
})

test('the notification texts carry the version of an update', () => {
  expect(MAIN_TEXT.ru.notifyUpdateTitle).toContain('{v}')
  expect(MAIN_TEXT.en.notifyUpdateTitle).toContain('{v}')
})
