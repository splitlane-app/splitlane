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

test('the texts of the VPN lane fit either connector; only the Tailscale app and its login name Tailscale', () => {
  for (const lang of ['ru', 'en'] as const) {
    const m = MAIN_TEXT[lang]
    // the tray menu and the toasts name no exit node and no server: nothing to change between Tailscale and a key
    for (const key of ['notifyVpnDownTitle', 'notifyVpnDownBody', 'notifyErrorTitle', 'notifyErrorBody'] as const) {
      expect(m[key], key).not.toMatch(/exit node|Tailscale/i)
    }
  }
  expect(MAIN_TEXT.ru.notifyVpnDownTitle).toContain('VPN-сервер')
  expect(MAIN_TEXT.en.notifyVpnDownTitle).toContain('VPN server')
  // the file dialog of «Файл настроек» (the glossary: «Файл настроек», "Configuration file")
  expect(MAIN_TEXT.ru.confFilter).toBe('Файлы настроек WireGuard')
  expect(MAIN_TEXT.en.confFilter).toBe('WireGuard configuration files')
})
