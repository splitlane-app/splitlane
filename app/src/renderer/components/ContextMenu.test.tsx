import { act, cleanup, fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'
import type { Command } from '../../shared/ipc'
import { boardView, type CardVM } from '../model/board'
import { activity, entry, fakeBridge, renderWith, rule, servicesFor, snap } from '../testing'
import { AppCard } from './AppCard'

afterEach(cleanup)

const MAIL = 'C:\\Apps\\Mail\\mail.exe'
const BROWSER = 'C:\\Apps\\Browser\\browser.exe'
const GAME_DIR = 'C:\\Games\\Game\\'
const GAME = `${GAME_DIR}game.exe`
const ID1 = '6f1c2a4e-0b7d-4c3a-9e21-5d8f7a1b2c01'
const ID2 = '6f1c2a4e-0b7d-4c3a-9e21-5d8f7a1b2c02'
const UNATTRIBUTED = 'WSL и виртуальные машины'

const board = snap({ apps: [rule(ID1, 'Mail', MAIL, 'vpn'), rule(ID2, 'Browser', BROWSER, 'direct')] })

/** The cards of the board in the phase on (the default lane is VPN) and the program card opens they asked for. */
function setup() {
  const { bridge, commands } = fakeBridge()
  const services = servicesFor(bridge, board)
  const live = activity([
    entry(GAME_DIR, { name: 'Game', exe: GAME, down: 4096 }),
    entry('unattributed', { kind: 'unattributed' })
  ])
  act(() => services.store.getState().setActivity(live))
  const s = board.state!
  const view = boardView(s.apps, s.defaultLane, services.store.getState().activity, s.addresses, '')
  const all = [...view.vpn.pinned, ...view.vpn.unpinned, ...view.direct.pinned]
  const byName = (name: string): CardVM => all.find((c) => c.name === name)!
  const opened: string[] = []
  renderWith(
    <>
      {['Mail', 'Game', 'Browser', UNATTRIBUTED].map((name) => (
        <AppCard key={name} card={byName(name)} onOpen={(key) => opened.push(key)} />
      ))}
    </>,
    services
  )
  return { commands, opened }
}

const cardOf = (label: string): HTMLElement => screen.getByRole('button', { name: label })
const items = (): string[] => screen.getAllByRole('menuitem').map((i) => i.textContent ?? '')
const focused = (): string => document.activeElement?.textContent ?? ''
const press = (key: string, init: { shiftKey?: boolean } = {}): void => {
  fireEvent.keyDown(document.activeElement!, { key, ...init })
}

test('ContextMenu offers the other lane and pin or unpin', async () => {
  const { commands, opened } = setup()

  fireEvent.contextMenu(cardOf('Mail, Через VPN'), { clientX: 40, clientY: 50 })
  const menu = screen.getByRole('menu', { name: 'Действия: Mail' })
  expect(items()).toEqual(['Пустить напрямую', 'Открепить'])
  expect([menu.style.left, menu.style.top]).toEqual(['40px', '50px'])
  press('Escape')
  expect(screen.queryByRole('menu')).toBeNull()

  // an unpinned program goes by the default lane: the other lane or pinned where it is
  fireEvent.contextMenu(cardOf('Game, Через VPN, не закреплена'))
  expect(items()).toEqual(['Пустить напрямую', 'Закрепить'])
  press('Escape')

  fireEvent.contextMenu(cardOf('Browser, Напрямую'))
  expect(items()).toEqual(['Пустить через VPN', 'Открепить'])
  press('Escape')

  // the tile of WSL and virtual machines never moves: no menu
  fireEvent.contextMenu(cardOf(`${UNATTRIBUTED}, Через VPN, не закреплена`))
  expect(screen.queryByRole('menu')).toBeNull()

  // an item does its action at once and closes the menu
  fireEvent.contextMenu(cardOf('Mail, Через VPN'))
  await act(async () => {
    fireEvent.click(screen.getByRole('menuitem', { name: 'Пустить напрямую' }))
  })
  expect(screen.queryByRole('menu')).toBeNull()
  fireEvent.contextMenu(cardOf('Game, Через VPN, не закреплена'))
  await act(async () => {
    fireEvent.click(screen.getByRole('menuitem', { name: 'Закрепить' }))
  })
  fireEvent.contextMenu(cardOf('Browser, Напрямую'))
  await act(async () => {
    fireEvent.click(screen.getByRole('menuitem', { name: 'Открепить' }))
  })

  // each change goes on the layout of the state (the fake helper answers ok and sends no new state)
  const sent = commands.map((c: Command) => (c.method === 'lanes.set' ? c.apps.map((r) => [r.exe, r.lane]) : c.method))
  expect(sent).toEqual([
    [
      [MAIL, 'direct'],
      [BROWSER, 'direct']
    ],
    [
      [MAIL, 'vpn'],
      [BROWSER, 'direct'],
      [GAME, 'vpn']
    ],
    [[MAIL, 'vpn']]
  ])
  // a click in the menu is not a click on the card
  expect(opened).toEqual([])
})

test('ContextMenu opens with Shift+F10 and closes with Escape', () => {
  const { commands, opened } = setup()
  const mail = cardOf('Mail, Через VPN')
  mail.focus()

  press('F10')
  expect(screen.queryByRole('menu')).toBeNull()
  press('F10', { shiftKey: true })
  expect(screen.getByRole('menu', { name: 'Действия: Mail' })).toBeTruthy()
  // the first item takes the focus; the arrows go round the items
  expect(focused()).toBe('Пустить напрямую')
  press('ArrowDown')
  expect(focused()).toBe('Открепить')
  press('ArrowDown')
  expect(focused()).toBe('Пустить напрямую')
  press('ArrowUp')
  expect(focused()).toBe('Открепить')
  press('Home')
  expect(focused()).toBe('Пустить напрямую')

  press('Escape')
  expect(screen.queryByRole('menu')).toBeNull()
  expect(document.activeElement).toBe(mail)

  // the menu key opens it too; a press outside closes it
  press('ContextMenu')
  expect(screen.getByRole('menu')).toBeTruthy()
  fireEvent.pointerDown(document.body)
  expect(screen.queryByRole('menu')).toBeNull()
  // and so does the focus going elsewhere
  mail.focus()
  press('ContextMenu')
  expect(screen.getByRole('menu')).toBeTruthy()
  act(() => cardOf('Browser, Напрямую').focus())
  expect(screen.queryByRole('menu')).toBeNull()

  // Enter opens the program card
  mail.focus()
  press('Enter')
  expect(opened).toEqual([ID1])
  expect(commands).toEqual([])
})
