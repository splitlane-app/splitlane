import { act, cleanup, fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { App } from './App'
import type { Snapshot } from '../shared/ipc'
import { DEFAULT_SETTINGS } from '../shared/settings'
import { setLang } from './i18n/t'
import { fakeBridge, renderWith, rule, servicesFor, snap as baseSnap } from './testing'

const snap: typeof baseSnap = (over) => ({ ...baseSnap(over), settings: { ...DEFAULT_SETTINGS, connectedOnce: true } })

afterEach(() => {
  cleanup()
  setLang('ru')
})

test('shows the product title', () => {
  const { bridge } = fakeBridge()
  renderWith(<App />, servicesFor(bridge))
  expect(screen.getByText('SplitLane')).toBeTruthy()
  expect(screen.getByText('Подключение к служебному процессу…')).toBeTruthy()
})

test('App picks the screen by the link and shows the board once there is a state', () => {
  const { bridge } = fakeBridge()
  const services = servicesFor(bridge, { ...snap(), link: { kind: 'setup-needed' }, state: null })
  renderWith(<App />, services)
  expect(screen.getByRole('heading', { name: 'Первая настройка' })).toBeTruthy()

  act(() => services.store.getState().setSnapshot(snap({ phase: 'off' })))
  expect(screen.getByRole('region', { name: 'Через VPN' })).toBeTruthy()
  expect(screen.getByRole('region', { name: 'Напрямую' })).toBeTruthy()

  // a restarting link keeps the board of the last state, with its banner
  act(() => services.store.getState().setSnapshot({ ...snap({ phase: 'off' }), link: { kind: 'restarting', attempt: 1 } }))
  expect(screen.getByRole('region', { name: 'Через VPN' })).toBeTruthy()
  expect(screen.getByText('Служебный процесс перезапускается…')).toBeTruthy()

  act(() => services.store.getState().showConnection(true))
  expect(screen.getByRole('heading', { name: 'Подключение' })).toBeTruthy()
})

const MAIL = 'C:\\Apps\\Mail\\mail.exe'
const ID = '6f1c2a4e-0b7d-4c3a-9e21-5d8f7a1b2c01'
const NOTES = 'https://github.com/splitlane-app/splitlane/releases/tag/v1.2.0'
const INSTRUCTIONS = 'splitlane-dnd-instructions-board'

test('switching the language re-renders open screens', async () => {
  const { bridge } = fakeBridge()
  const ru: Snapshot = {
    ...snap({ apps: [rule(ID, 'Mail', MAIL, 'vpn')] }),
    update: { kind: 'available', version: '1.2.0', notesUrl: NOTES, hidden: false }
  }
  // the same state, settings and update: only the language changes, nothing else would make the page render again
  const en: Snapshot = { ...ru, language: 'en' }
  const services = servicesFor(bridge, ru)
  renderWith(<App />, services)
  const items = (): string[] => screen.getAllByRole('menuitem').map((i) => i.textContent ?? '')

  // the board with the update banner, a tile and the open menu of a card
  expect(screen.getByRole('region', { name: 'Через VPN' })).toBeTruthy()
  expect(screen.getByText('Доступна версия 1.2.0')).toBeTruthy()
  expect(screen.getByRole('button', { name: /^Службы Windows, Через VPN/ })).toBeTruthy()
  fireEvent.contextMenu(screen.getByRole('button', { name: 'Mail, Через VPN' }))
  expect(items()).toEqual(['Пустить напрямую', 'Открепить'])
  await vi.waitFor(() => expect(document.getElementById(INSTRUCTIONS)?.textContent).toMatch(/^Пробел/))

  act(() => services.store.getState().setSnapshot(en))
  expect(screen.getByRole('region', { name: 'Via VPN' })).toBeTruthy()
  expect(screen.getByRole('region', { name: 'Direct' })).toBeTruthy()
  expect(screen.getByText('Version 1.2.0 is available')).toBeTruthy()
  expect(screen.getByRole('button', { name: "What's new" })).toBeTruthy()
  expect(screen.getByRole('button', { name: /^Windows services, Via VPN/ })).toBeTruthy()
  expect(screen.getByRole('menu', { name: 'Actions for Mail' })).toBeTruthy()
  expect(items()).toEqual(['Send direct', 'Unpin'])
  expect(screen.getByRole('switch', { name: 'Split routing' })).toBeTruthy()
  expect(document.getElementById(INSTRUCTIONS)?.textContent).toMatch(/^Press Space/)

  // «Подключение» open while the language changes back and forth
  fireEvent.keyDown(document.activeElement!, { key: 'Escape' })
  act(() => services.store.getState().showConnection(true))
  expect(screen.getByRole('heading', { name: 'Connection' })).toBeTruthy()
  expect(screen.getByRole('combobox', { name: 'Language' })).toBeTruthy()
  act(() => services.store.getState().setSnapshot(ru))
  expect(screen.getByRole('heading', { name: 'Подключение' })).toBeTruthy()
  expect(screen.getByRole('combobox', { name: 'Язык' })).toBeTruthy()
  expect(screen.getByRole('button', { name: 'Проверить обновления' })).toBeTruthy()
  act(() => services.store.getState().setSnapshot(en))
  expect(screen.getByRole('button', { name: 'Check for updates' })).toBeTruthy()
  expect(screen.getByRole('button', { name: 'Back to lanes' })).toBeTruthy()
})

test('document lang follows the language', () => {
  const { bridge } = fakeBridge()
  const services = servicesFor(bridge)
  renderWith(<App />, services)
  // before the first snapshot: the language the page started with
  expect(document.documentElement.lang).toBe('ru')
  act(() => services.store.getState().setSnapshot({ ...snap(), language: 'en' }))
  expect(document.documentElement.lang).toBe('en')
  act(() => services.store.getState().setSnapshot(snap()))
  expect(document.documentElement.lang).toBe('ru')
})
