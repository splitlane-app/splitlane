import { act, cleanup, fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'
import { fakeBridge, renderWith, servicesFor, snap, state } from '../testing'
import { Header } from './Header'

afterEach(cleanup)

test('Header toggle sends routing.enable in off and routing.disable in on', async () => {
  const { bridge, commands } = fakeBridge()
  const services = servicesFor(bridge, snap({ phase: 'off' }))
  renderWith(<Header />, services)

  const toggle = screen.getByRole('switch')
  expect(toggle.getAttribute('aria-checked')).toBe('false')
  expect(screen.getByText('Выключено')).toBeTruthy()
  await act(async () => {
    fireEvent.click(toggle)
  })
  expect(commands).toEqual([{ method: 'routing.enable' }])

  act(() => services.store.getState().setSnapshot(snap({ phase: 'on' })))
  expect(toggle.getAttribute('aria-checked')).toBe('true')
  expect(screen.getByText('Включено')).toBeTruthy()
  await act(async () => {
    fireEvent.click(toggle)
  })
  expect(commands).toEqual([{ method: 'routing.enable' }, { method: 'routing.disable' }])

  act(() => services.store.getState().setSnapshot(snap({ phase: 'error' })))
  await act(async () => {
    fireEvent.click(toggle)
  })
  expect(commands.at(-1)).toEqual({ method: 'routing.enable' })

  act(() => services.store.getState().setSnapshot(snap({ phase: 'stopping' })))
  expect((toggle as HTMLButtonElement).disabled).toBe(true)
})

test('Header shows the connection, searches and opens the add panel', () => {
  const { bridge } = fakeBridge()
  const services = servicesFor(bridge, snap({ phase: 'on' }))
  renderWith(<Header />, services)

  expect(screen.getByText('SplitLane')).toBeTruthy()
  const pill = screen.getByRole('button', { name: /Tailscale · node-a · прямое соединение/ })
  fireEvent.click(pill)
  expect(services.store.getState().ui.screen).toBe('connection')

  act(() => {
    const relay = state({ phase: 'on' })
    relay.tailscale = { ...relay.tailscale, path: 'relay' }
    services.store.getState().setSnapshot({ ...snap(), state: relay })
  })
  expect(screen.getByRole('button', { name: /Tailscale · node-a · через ретранслятор/ })).toBeTruthy()

  act(() => {
    const none = state({ phase: 'off' })
    none.tailscale = { ...none.tailscale, path: null }
    services.store.getState().setSnapshot({ ...snap(), state: none })
  })
  expect(screen.getByRole('button', { name: 'Tailscale · node-a' })).toBeTruthy()

  fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'mail' } })
  expect(services.store.getState().search).toBe('mail')

  fireEvent.click(screen.getByRole('button', { name: '+ Добавить' }))
  expect(services.store.getState().ui.addOpen).toBe(true)
})

test('Header settings button opens the connection screen with the settings', () => {
  const { bridge } = fakeBridge()
  const services = servicesFor(bridge, snap({ phase: 'off' }))
  renderWith(<Header />, services)

  const settings = screen.getByRole('button', { name: 'Настройки и подключение' })
  expect(settings.getAttribute('title')).toBe('Настройки и подключение')
  // next to «+ Добавить»
  expect(settings.previousElementSibling).toBe(screen.getByRole('button', { name: '+ Добавить' }))
  fireEvent.click(settings)
  expect(services.store.getState().ui.screen).toBe('connection')
})
