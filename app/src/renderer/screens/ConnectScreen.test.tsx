import { act, cleanup, fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import type { Command, Snapshot } from '../../shared/ipc'
import type { HelperState } from '../../shared/protocol'
import { DEFAULT_SETTINGS } from '../../shared/settings'
import { sendCommand } from '../services'
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

test('ConnectScreen opens the login link once per link', async () => {
  const openLogin = vi.fn(async () => true)
  const { bridge, commands } = fakeBridge({ openLogin })
  const a = 'https://login.tailscale.com/a/one'
  const b = 'https://login.tailscale.com/a/two'
  const services = servicesFor(bridge, withTs({ backend: 'needs_login' }))
  renderWith(<ConnectScreen />, services)
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Войти в Tailscale' }))
  })
  expect(commands).toEqual([{ method: 'tailscale.login' }])
  expect(openLogin).not.toHaveBeenCalled()

  // the link of the login this page asked for opens by itself
  act(() => services.store.getState().setSnapshot(withTs({ backend: 'needs_login', authUrl: a })))
  expect(openLogin).toHaveBeenCalledTimes(1)
  expect(openLogin).toHaveBeenLastCalledWith(a)

  // a new snapshot with the same link does not open it again
  act(() => services.store.getState().setSnapshot(withTs({ backend: 'needs_login', authUrl: a }, { phase: 'off', reason: null })))
  expect(openLogin).toHaveBeenCalledTimes(1)

  fireEvent.click(screen.getByRole('button', { name: 'Открыть ссылку снова' }))
  expect(openLogin).toHaveBeenCalledTimes(2)

  // a later link that no login of this page asked for waits for the button
  act(() => services.store.getState().setSnapshot(withTs({ backend: 'needs_login', authUrl: b })))
  expect(openLogin).toHaveBeenCalledTimes(2)
  fireEvent.click(screen.getByRole('button', { name: 'Открыть ссылку снова' }))
  expect(openLogin).toHaveBeenCalledTimes(3)
  expect(openLogin).toHaveBeenLastCalledWith(b)
})

test('ConnectScreen does not open the link the helper keeps after a logout', () => {
  const openLogin = vi.fn(async () => true)
  const { bridge } = fakeBridge({ openLogin })
  const kept = 'https://login.tailscale.com/a/kept'
  const services = servicesFor(bridge, withTs({ backend: 'needs_login', authUrl: kept }, { phase: 'off' }, true))
  renderWith(<ConnectScreen />, services)
  expect(openLogin).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'Открыть ссылку снова' }))
  expect(openLogin).toHaveBeenCalledTimes(1)
  expect(openLogin).toHaveBeenLastCalledWith(kept)
})

test('a login request waits for its link, and a failure or a logout drops it', async () => {
  const openLogin = vi.fn(async () => true)
  const { bridge } = fakeBridge({ openLogin })
  const expired = withTs({ backend: 'expired' }, { phase: 'degraded', reason: { code: 'login_expired', params: {} } }, true)
  const services = servicesFor(bridge, expired)
  const { store } = services
  // «Выключить деление и войти» on the board: the connect screen is not there yet
  await sendCommand(services, { method: 'tailscale.login' })
  expect(store.getState().loginRequested).toBe(true)
  const fresh = 'https://login.tailscale.com/a/fresh'
  store.getState().setSnapshot(withTs({ backend: 'needs_login', authUrl: fresh }, { phase: 'off' }, true))
  renderWith(<ConnectScreen />, services)
  expect(openLogin).toHaveBeenCalledTimes(1)
  expect(openLogin).toHaveBeenLastCalledWith(fresh)
  expect(store.getState().loginRequested).toBe(false)

  const send = (s: typeof services, c: Command): Promise<void> =>
    act(async () => {
      await sendCommand(s, c)
    })
  act(() => store.getState().setSnapshot(withTs({ backend: 'needs_login' }, { phase: 'off' }, true)))
  // a failed login leaves nothing to open
  const failing = fakeBridge({ openLogin }, { ok: false, error: { code: 'busy', message: 'busy' } })
  await send({ ...services, bridge: failing.bridge }, { method: 'tailscale.login' })
  expect(store.getState().loginRequested).toBe(false)

  // a logout drops a login request that never got its link
  await send(services, { method: 'tailscale.login' })
  expect(store.getState().loginRequested).toBe(true)
  await send(services, { method: 'tailscale.logout' })
  expect(store.getState().loginRequested).toBe(false)
  act(() =>
    store.getState().setSnapshot(withTs({ backend: 'needs_login', authUrl: 'https://login.tailscale.com/a/kept' }, { phase: 'off' }, true))
  )
  expect(openLogin).toHaveBeenCalledTimes(1)
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

test('ConnectScreen shows the banner of a failed first enable, of the start and of the restore', async () => {
  let saved = 0
  const { bridge, commands } = fakeBridge({
    saveDiagnostics: async () => {
      saved++
      return { saved: false }
    }
  })
  const failed = withTs({ backend: 'stopped' }, { phase: 'error', reason: { code: 'same_address', params: {} } })
  const services = servicesFor(bridge, failed)
  renderWith(<ConnectScreen />, services)
  expect(
    screen.getByText(
      'Не удалось включить: exit node выходит в интернет с того же адреса, что и этот компьютер, такой exit node не подходит'
    )
  ).toBeTruthy()
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Повторить' }))
  })
  expect(commands).toEqual([{ method: 'routing.enable' }])
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить журнал' }))
  })
  expect(saved).toBe(1)

  act(() => services.store.getState().setSnapshot(withTs({ backend: 'running' }, { phase: 'starting' })))
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Отменить' }))
  })
  expect(commands.at(-1)).toEqual({ method: 'routing.disable' })

  const command = 'tailscale set --exit-node=100.64.0.2'
  act(() =>
    services.store
      .getState()
      .setSnapshot(withTs({ backend: 'stopped' }, { phase: 'stopping', reason: { code: 'restore_pending', params: { command } } }))
  )
  expect(screen.getByText(command)).toBeTruthy()
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Выключить без возврата' }))
  })
  expect(commands.at(-1)).toEqual({ method: 'routing.disable', skipRestore: true })
})

test('ConnectScreen shows the banners of a restarting and an unresponsive link, and none in off', async () => {
  let retried = 0
  const { bridge } = fakeBridge({
    retryLink: async () => {
      retried++
    }
  })
  const off = withTs({ backend: 'needs_login' })
  const services = servicesFor(bridge, off)
  renderWith(<ConnectScreen />, services)
  // off: the enable button of the steps is the action, the banner would repeat it
  expect(screen.queryByText('Выключено. Программы ходят как обычно, без деления')).toBeNull()
  expect(screen.queryByRole('alert')).toBeNull()
  expect(screen.queryByRole('status')).toBeNull()

  act(() => services.store.getState().setSnapshot({ ...off, link: { kind: 'restarting', attempt: 2 } }))
  expect(screen.getByText('Служебный процесс перезапускается…')).toBeTruthy()

  act(() => services.store.getState().setSnapshot({ ...off, link: { kind: 'unresponsive' } }))
  expect(screen.getByText('Служебный процесс не отвечает')).toBeTruthy()
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Повторить' }))
  })
  expect(retried).toBe(1)
})

test('connection screen shows the state banner, off included', async () => {
  const { bridge, commands } = fakeBridge()
  const off = withTs({}, { phase: 'off' }, true)
  const services = servicesFor(bridge, off)
  renderWith(<ConnectionScreen />, services)
  expect(screen.getByText('Выключено. Программы ходят как обычно, без деления')).toBeTruthy()
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Включить' }))
  })
  expect(commands).toEqual([{ method: 'routing.enable' }])

  act(() => services.store.getState().setSnapshot({ ...off, link: { kind: 'unresponsive' } }))
  expect(screen.getByText('Служебный процесс не отвечает')).toBeTruthy()
  act(() =>
    services.store
      .getState()
      .setSnapshot(withTs({}, { phase: 'error', reason: { code: 'ip_forwarding', params: {} } }, true))
  )
  expect(screen.getByText(/^Не удалось включить: включён общий доступ к интернету/)).toBeTruthy()
  expect(screen.getByRole('button', { name: 'Сохранить журнал' })).toBeTruthy()
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

test('connection screen shows a dictionary text when a setting is not saved', async () => {
  const { bridge } = fakeBridge({
    setSettings: async () => {
      throw new Error('EPERM: operation not permitted, open settings.json')
    }
  })
  const services = servicesFor(bridge, withTs({}, { phase: 'off' }, true))
  renderWith(<ConnectionScreen />, services)
  await act(async () => {
    fireEvent.click(screen.getByRole('switch', { name: 'Запускать при входе в Windows' }))
  })
  expect(services.store.getState().notice).toBe('Не удалось сохранить настройку')
  expect(screen.getByText('Не удалось сохранить настройку')).toBeTruthy()
  expect(screen.queryByText(/EPERM/)).toBeNull()
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
