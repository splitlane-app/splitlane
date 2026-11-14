import { act, cleanup, fireEvent, screen, within } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'
import type { Command, CommandResult } from '../../shared/ipc'
import { activity, entry, fakeBridge, renderWith, rule, servicesFor, snap } from '../testing'
import { Board } from './Board'
import { findCard } from './cardFocus'

afterEach(cleanup)

const MAIL = 'C:\\Apps\\Mail\\mail.exe'
const GAME_DIR = 'C:\\Games\\Game\\'
const GAME = `${GAME_DIR}Game.exe`
const ID1 = '6f1c2a4e-0b7d-4c3a-9e21-5d8f7a1b2c01'

/**
 * The board in on with Mail pinned and Game and the Windows services unpinned. The helper never answers a
 * lanes.set, so a change stays on screen as sent.
 */
function setup() {
  const sent: Command[] = []
  const { bridge } = fakeBridge({
    command: (c) => {
      sent.push(c)
      return new Promise<CommandResult>(() => {})
    }
  })
  const services = servicesFor(bridge, snap({ apps: [rule(ID1, 'Mail', MAIL, 'vpn')] }))
  act(() =>
    services.store.getState().setActivity(
      activity([
        entry(ID1, { ruleId: ID1, name: 'Mail', exe: MAIL, down: 10 }),
        entry(GAME_DIR, { name: 'Game', exe: GAME, down: 5 }),
        entry('system', { kind: 'system', down: 3 })
      ])
    )
  )
  renderWith(<Board />, services)
  return { sent }
}

const card = (name: string): HTMLElement => document.querySelector<HTMLElement>(`[data-card][data-name="${name}"]`)!
const frame = (): Promise<void> =>
  act(async () => {
    await new Promise<void>((r) => requestAnimationFrame(() => r()))
  })

test('the pin keeps the focus on its card when pinning changes the key of the card', async () => {
  const { sent } = setup()
  const pinOf = (name: string, label: string): HTMLElement => within(card(name)).getByRole('button', { name: label })
  const before = card('Game')
  pinOf('Game', 'Закрепить').focus()
  await act(async () => {
    fireEvent.click(pinOf('Game', 'Закрепить'))
  })
  await frame()
  expect(sent).toHaveLength(1)
  expect(before.isConnected).toBe(false)
  expect(card('Game').dataset.pinned).toBe('true')
  expect(document.activeElement).toBe(pinOf('Game', 'Открепить'))

  // the tile of the Windows services becomes a rule of kind system with its own key
  pinOf('Службы Windows', 'Закрепить').focus()
  await act(async () => {
    fireEvent.click(pinOf('Службы Windows', 'Закрепить'))
  })
  await frame()
  expect(card('Службы Windows').dataset.pinned).toBe('true')
  expect(document.activeElement).toBe(pinOf('Службы Windows', 'Открепить'))
})

test('the menu items «Закрепить» and «Открепить» give the focus back to the moved card', async () => {
  setup()
  fireEvent.contextMenu(screen.getByRole('button', { name: 'Game, Через VPN, не закреплена' }))
  await act(async () => {
    fireEvent.click(screen.getByRole('menuitem', { name: 'Закрепить' }))
  })
  await frame()
  expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Game, Через VPN' }))

  fireEvent.contextMenu(screen.getByRole('button', { name: 'Mail, Через VPN' }))
  await act(async () => {
    fireEvent.click(screen.getByRole('menuitem', { name: 'Открепить' }))
  })
  await frame()
  expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Mail, Через VPN, не закреплена' }))
})

test('findCard finds a card by its key, else by its file ignoring case, else the Windows services, never a placeholder', () => {
  document.body.innerHTML = `
    <div data-card="p1" data-exe="C:\\A\\a.exe" data-kind="app" inert><div data-card-main></div></div>
    <div data-card="r1" data-exe="C:\\A\\a.exe" data-kind="app"><div data-card-main></div></div>
    <div data-card="r2" data-exe="" data-kind="system"><div data-card-main></div></div>`
  expect(findCard({ key: 'r1', exe: '', kind: 'app' })?.dataset.card).toBe('r1')
  expect(findCard({ key: 'c:\\a\\', exe: 'c:\\a\\A.EXE', kind: 'app' })?.dataset.card).toBe('r1')
  expect(findCard({ key: 'system', exe: '', kind: 'system' })?.dataset.card).toBe('r2')
  expect(findCard({ key: 'gone', exe: 'C:\\B\\b.exe', kind: 'app' })).toBeNull()
  expect(findCard({ key: 'p1', exe: '', kind: 'app' })).toBeNull()
  document.body.innerHTML = ''
})
