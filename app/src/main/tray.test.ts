import { beforeEach, expect, test, vi } from 'vitest'
import type { HelperState } from '../shared/protocol'
import type { LinkStatus } from '../shared/status'

interface Item {
  label?: string
  type?: string
  enabled?: boolean
}

const menus: Item[][] = []
const setContextMenu = vi.fn()

vi.mock('electron', () => {
  class Tray {
    setToolTip = vi.fn()
    on = vi.fn()
    setImage = vi.fn()
    setContextMenu = setContextMenu
    destroy = vi.fn()
  }
  return {
    Menu: {
      buildFromTemplate: (t: Item[]) => {
        menus.push(t)
        return t
      }
    },
    Tray,
    nativeImage: { createFromBitmap: () => ({}) },
    screen: { getPrimaryDisplay: () => ({ scaleFactor: 1 }) }
  }
})

const { AppTray } = await import('./tray')

const actions = { open: vi.fn(), settings: vi.fn(), checkUpdates: vi.fn(), toggle: vi.fn(), quit: vi.fn() }
const link: LinkStatus = { kind: 'connecting' }
const labels = (m: Item[]): (string | undefined)[] => m.filter((i) => i.type !== 'separator').map((i) => i.label)

beforeEach(() => {
  menus.length = 0
  setContextMenu.mockClear()
})

test('the tray menu is built in the language and rebuilt when the language changes', () => {
  const tray = new AppTray(actions, 'en')
  expect(labels(menus.at(-1) ?? [])).toEqual(['Open', 'Settings…', 'Turn on', 'Check for updates', 'Quit'])

  tray.setLanguage('ru', link, null as HelperState | null)
  expect(labels(menus.at(-1) ?? [])).toEqual(['Открыть', 'Настройки…', 'Включить', 'Проверить обновления', 'Выход'])
  const built = menus.length

  // the same language again changes nothing
  tray.setLanguage('ru', link, null)
  expect(menus.length).toBe(built)

  tray.setLanguage('en', link, null)
  expect(labels(menus.at(-1) ?? [])[0]).toBe('Open')
})
