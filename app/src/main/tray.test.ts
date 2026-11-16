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

test('while a server setup runs the enable is greyed out and says why; the end brings it back', () => {
  const ready: LinkStatus = { kind: 'ready', hello: { protocol: 3, helper: 'x', core: 'y' } }
  const off = { phase: 'off', reason: null, allDirect: false } as unknown as HelperState
  const tray = new AppTray(actions, 'ru')
  tray.update(ready, off)
  const toggleItem = (): Item | undefined => (menus.at(-1) ?? []).find((i) => i.label?.startsWith('Включить'))
  expect(toggleItem()).toMatchObject({ label: 'Включить', enabled: true })
  tray.setSetupRunning(true, ready, off)
  expect(toggleItem()).toMatchObject({ label: 'Включить — дождитесь конца настройки сервера', enabled: false })
  // a new state or language keeps it until the setup ends
  tray.update(ready, off)
  expect(toggleItem()?.enabled).toBe(false)
  tray.setLanguage('en', ready, off)
  expect((menus.at(-1) ?? []).find((i) => i.label?.startsWith('Turn on'))).toMatchObject({
    label: 'Turn on — wait for the server setup to finish',
    enabled: false
  })
  tray.setSetupRunning(false, ready, off)
  expect((menus.at(-1) ?? []).find((i) => i.label?.startsWith('Turn on'))).toMatchObject({ label: 'Turn on', enabled: true })
})
