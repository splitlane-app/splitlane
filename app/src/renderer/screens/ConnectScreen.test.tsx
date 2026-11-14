import { act, cleanup, fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import type { Snapshot } from '../../shared/ipc'
import type { HelperState } from '../../shared/protocol'
import { DEFAULT_SETTINGS } from '../../shared/settings'
import { fakeBridge, renderWith, servicesFor, snap, state } from '../testing'
import { ConnectionScreen, ConnectScreen } from './ConnectScreen'

afterEach(cleanup)

const withTs = (
  over: Partial<HelperState['tailscale']>,
  top: Partial<HelperState> = { phase: 'off' },
  once = false
): Snapshot => ({
  ...snap({ ...top, tailscale: { ...state().tailscale, ...over } }),
  settings: { ...DEFAULT_SETTINGS, connectedOnce: once }
})

test('ConnectScreen shows the methods and the login step', async () => {
  const { bridge, commands } = fakeBridge()
  renderWith(<ConnectScreen />, servicesFor(bridge, withTs({ backend: 'needs_login' })))
  expect(screen.getByRole('heading', { name: 'Подключите VPN' })).toBeTruthy()
  expect(screen.getByText('Ключ или подписка')).toBeTruthy()
  expect(screen.getByText('Файл настроек')).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: 'Войти в Tailscale' }))
  await vi.waitFor(() => expect(commands).toEqual([{ method: 'tailscale.login' }]))
})

test('ConnectScreen opens the login link once per link', () => {
  const openLogin = vi.fn(async () => true)
  const { bridge } = fakeBridge({ openLogin })
  const a = 'https://login.tailscale.com/a/one'
  const b = 'https://login.tailscale.com/a/two'
  const services = servicesFor(bridge, withTs({ backend: 'needs_login', authUrl: a }))
  renderWith(<ConnectScreen />, services)
  expect(openLogin).toHaveBeenCalledTimes(1)
  expect(openLogin).toHaveBeenLastCalledWith(a)

  // a new snapshot with the same link does not open it again
  act(() => services.store.getState().setSnapshot(withTs({ backend: 'needs_login', authUrl: a }, { phase: 'off', reason: null })))
  expect(openLogin).toHaveBeenCalledTimes(1)

  fireEvent.click(screen.getByRole('button', { name: 'Открыть ссылку снова' }))
  expect(openLogin).toHaveBeenCalledTimes(2)

  act(() => services.store.getState().setSnapshot(withTs({ backend: 'needs_login', authUrl: b })))
  expect(openLogin).toHaveBeenCalledTimes(3)
  expect(openLogin).toHaveBeenLastCalledWith(b)
})

test('ConnectScreen waits for the approval of the node', () => {
  const { bridge } = fakeBridge()
  renderWith(<ConnectScreen />, servicesFor(bridge, withTs({ backend: 'needs_approval' })))
  expect(screen.getByText(/ждёт одобрения администратором/)).toBeTruthy()
})

test('ConnectScreen lists exit nodes, online first, and chooses one', async () => {
  const { bridge, commands } = fakeBridge()
  const exitNodes = [
    { id: '100.64.0.5', name: 'zeta', online: false },
    { id: '100.64.0.4', name: 'beta', online: true },
    { id: '100.64.0.3', name: 'alpha', online: true }
  ]
  renderWith(
    <ConnectScreen />,
    servicesFor(bridge, withTs({ backend: 'running', exitNodes, selectedExitNode: null }))
  )
  const names = screen.getAllByRole('button', { pressed: false }).map((b) => b.textContent)
  expect(names).toEqual(['alpha', 'beta', 'zetaне в сети'])
  fireEvent.click(screen.getByRole('button', { name: /zeta/ }))
  await vi.waitFor(() => expect(commands).toEqual([{ method: 'tailscale.setExitNode', id: '100.64.0.5' }]))
})

test('ConnectScreen warns about the system exit node before enabling', async () => {
  const { bridge, commands } = fakeBridge()
  const on: HelperState['systemTailscale'] = { installed: true, exitNodeOn: true, restoreOnDisable: false }
  const services = servicesFor(bridge, { ...withTs({ backend: 'running' }), state: { ...withTs({ backend: 'running' }).state!, systemTailscale: on } })
  renderWith(<ConnectScreen />, services)
  expect(screen.getByText(/В приложении Tailscale сейчас включён exit node/)).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: 'Включить деление' }))
  await vi.waitFor(() => expect(commands).toEqual([{ method: 'routing.enable' }]))

  act(() => services.store.getState().setSnapshot(withTs({ backend: 'running' })))
  expect(screen.queryByText(/В приложении Tailscale сейчас включён exit node/)).toBeNull()
})

test('ConnectScreen records the first connection once', () => {
  const setSettings = vi.fn(async () => DEFAULT_SETTINGS)
  const { bridge } = fakeBridge({ setSettings })
  const services = servicesFor(bridge, withTs({ backend: 'running' }, { phase: 'starting' }))
  renderWith(<ConnectScreen />, services)
  expect(setSettings).not.toHaveBeenCalled()
  act(() => services.store.getState().setSnapshot(withTs({ backend: 'running' }, { phase: 'on' })))
  act(() => services.store.getState().setSnapshot(withTs({ backend: 'running' }, { phase: 'on', allDirect: false })))
  expect(setSettings).toHaveBeenCalledTimes(1)
  expect(setSettings).toHaveBeenCalledWith({ connectedOnce: true })
})

test('connection screen disables logout while routing is on', async () => {
  const { bridge, commands } = fakeBridge()
  const services = servicesFor(bridge, withTs({ deviceName: 'laptop-1' }, { phase: 'on' }, true))
  renderWith(<ConnectionScreen />, services)
  expect(screen.getByText('laptop-1')).toBeTruthy()
  const logout = screen.getByRole('button', { name: 'Выйти из Tailscale' }) as HTMLButtonElement
  expect(logout.disabled).toBe(true)
  expect(screen.getByText('Смена exit node ненадолго перезапустит деление')).toBeTruthy()

  act(() => services.store.getState().setSnapshot(withTs({ deviceName: 'laptop-1' }, { phase: 'off' }, true)))
  expect((screen.getByRole('button', { name: 'Выйти из Tailscale' }) as HTMLButtonElement).disabled).toBe(false)
  fireEvent.click(screen.getByRole('button', { name: 'Выйти из Tailscale' }))
  await vi.waitFor(() => expect(commands).toEqual([{ method: 'tailscale.logout' }]))
})

test('connection screen changes the settings and goes back', () => {
  const setSettings = vi.fn(async () => DEFAULT_SETTINGS)
  const { bridge } = fakeBridge({ setSettings })
  const services = servicesFor(bridge, withTs({}, { phase: 'off' }, true))
  services.store.getState().showConnection(true)
  renderWith(<ConnectionScreen />, services)
  fireEvent.click(screen.getByRole('switch', { name: 'Запускать при входе в Windows' }))
  expect(setSettings).toHaveBeenLastCalledWith({ openAtLogin: true })
  fireEvent.click(screen.getByRole('switch', { name: 'Включать деление при запуске' }))
  expect(setSettings).toHaveBeenLastCalledWith({ enableOnStart: true })
  fireEvent.click(screen.getByRole('button', { name: 'Назад к дорожкам' }))
  expect(services.store.getState().ui.screen).toBe('board')
})
