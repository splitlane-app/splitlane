import { act, cleanup, screen } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'
import { App } from './App'
import { DEFAULT_SETTINGS } from '../shared/settings'
import { fakeBridge, renderWith, servicesFor, snap as baseSnap } from './testing'

const snap: typeof baseSnap = (over) => ({ ...baseSnap(over), settings: { ...DEFAULT_SETTINGS, connectedOnce: true } })

afterEach(cleanup)

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
