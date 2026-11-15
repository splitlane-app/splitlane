import { act, cleanup, fireEvent, screen, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import type { Command, Snapshot } from '../../shared/ipc'
import type { HelperState } from '../../shared/protocol'
import { DEFAULT_SETTINGS, type WindowSettings } from '../../shared/settings'
import { setLang } from '../i18n/t'
import { sendCommand } from '../services'
import { fakeBridge, renderWith, servicesFor, snap, state } from '../testing'
import { ConnectionScreen, ConnectScreen } from './ConnectScreen'

afterEach(() => {
  cleanup()
  setLang('ru')
})

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
  const group = screen.getByRole('radiogroup', { name: 'Exit node' })
  const radios = within(group).getAllByRole('radio') as HTMLInputElement[]
  expect(radios.map((r) => r.closest('label')?.textContent)).toEqual(['alpha', 'beta', 'zetaне в сети'])
  expect(radios.every((r) => !r.checked)).toBe(true)
  // routing is off: the choice goes at once, the wizard does not wait
  fireEvent.click(screen.getByRole('radio', { name: /zeta/ }))
  await vi.waitFor(() => expect(commands).toEqual([{ method: 'tailscale.setExitNode', id: '100.64.0.5' }]))
})

test('quick changes of the exit node send only the last one, and the pending one shows as checked', async () => {
  const { bridge, commands } = fakeBridge()
  const exitNodes = [
    { id: '100.64.0.3', name: 'alpha', online: true },
    { id: '100.64.0.4', name: 'beta', online: true },
    { id: '100.64.0.5', name: 'gamma', online: true }
  ]
  renderWith(<ConnectionScreen />, servicesFor(bridge, withTs({ exitNodes, selectedExitNode: '100.64.0.3' }, { phase: 'on' }, true)))
  vi.useFakeTimers()
  try {
    fireEvent.click(screen.getByRole('radio', { name: 'beta' }))
    await act(async () => {
      vi.advanceTimersByTime(400)
    })
    fireEvent.click(screen.getByRole('radio', { name: 'gamma' }))
    expect((screen.getByRole('radio', { name: 'gamma' }) as HTMLInputElement).checked).toBe(true)
    expect((screen.getByRole('radio', { name: 'alpha' }) as HTMLInputElement).checked).toBe(false)
    await act(async () => {
      vi.advanceTimersByTime(499)
    })
    expect(commands).toEqual([])
    await act(async () => {
      vi.advanceTimersByTime(1)
    })
    expect(commands).toEqual([{ method: 'tailscale.setExitNode', id: '100.64.0.5' }])
  } finally {
    vi.useRealTimers()
  }
})

test('a pending exit node choice is applied when the screen goes away', async () => {
  const { bridge, commands } = fakeBridge()
  const exitNodes = [
    { id: '100.64.0.3', name: 'alpha', online: true },
    { id: '100.64.0.4', name: 'beta', online: true }
  ]
  const view = renderWith(<ConnectionScreen />, servicesFor(bridge, withTs({ exitNodes, selectedExitNode: '100.64.0.3' }, { phase: 'on' }, true)))
  vi.useFakeTimers()
  try {
    fireEvent.click(screen.getByRole('radio', { name: 'beta' }))
    expect(commands).toEqual([])
    await act(async () => {
      view.unmount()
    })
    expect(commands).toEqual([{ method: 'tailscale.setExitNode', id: '100.64.0.4' }])
    await act(async () => {
      vi.advanceTimersByTime(1000)
    })
    expect(commands).toHaveLength(1)
  } finally {
    vi.useRealTimers()
  }
})

test('going back to the selected exit node within the pause sends nothing', async () => {
  const { bridge, commands } = fakeBridge()
  const exitNodes = [
    { id: '100.64.0.3', name: 'alpha', online: true },
    { id: '100.64.0.4', name: 'beta', online: true }
  ]
  renderWith(<ConnectionScreen />, servicesFor(bridge, withTs({ exitNodes, selectedExitNode: '100.64.0.3' }, { phase: 'on' }, true)))
  vi.useFakeTimers()
  try {
    fireEvent.click(screen.getByRole('radio', { name: 'beta' }))
    await act(async () => {
      vi.advanceTimersByTime(200)
    })
    fireEvent.click(screen.getByRole('radio', { name: 'alpha' }))
    expect((screen.getByRole('radio', { name: 'alpha' }) as HTMLInputElement).checked).toBe(true)
    await act(async () => {
      vi.advanceTimersByTime(2000)
    })
    expect(commands).toEqual([])
  } finally {
    vi.useRealTimers()
  }
})

test('the pending choice stays checked until the command is answered', async () => {
  let answer: (r: { ok: true }) => void = () => {}
  const { bridge } = fakeBridge({ command: () => new Promise((r) => (answer = r)) })
  const exitNodes = [
    { id: '100.64.0.3', name: 'alpha', online: true },
    { id: '100.64.0.4', name: 'beta', online: true }
  ]
  renderWith(<ConnectionScreen />, servicesFor(bridge, withTs({ exitNodes, selectedExitNode: '100.64.0.3' }, { phase: 'off' }, true)))
  fireEvent.click(screen.getByRole('radio', { name: 'beta' }))
  expect((screen.getByRole('radio', { name: 'beta' }) as HTMLInputElement).checked).toBe(true)
  await act(async () => {
    answer({ ok: true })
  })
  expect((screen.getByRole('radio', { name: 'alpha' }) as HTMLInputElement).checked).toBe(true)
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

test('ConnectScreen shows the versions', async () => {
  const { bridge } = fakeBridge({ appInfo: async () => ({ version: '0.2.0', installed: true }) })
  renderWith(<ConnectionScreen />, servicesFor(bridge, withTs({}, { phase: 'off' }, true)))
  expect(await screen.findByText('SplitLane 0.2.0 · служба 0.1.0 · ядро 1.14.4')).toBeTruthy()
})

test('ConnectScreen shows the app version without a link', async () => {
  const { bridge } = fakeBridge({ appInfo: async () => ({ version: '0.2.0', installed: true }) })
  const off = withTs({}, { phase: 'off' }, true)
  renderWith(<ConnectionScreen />, servicesFor(bridge, { ...off, link: { kind: 'unresponsive' } }))
  expect(await screen.findByText('SplitLane 0.2.0')).toBeTruthy()
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

const NOTES = 'https://github.com/splitlane-app/splitlane/releases/tag/v1.2.0'

test('ConnectionScreen shows the update line and the check button', async () => {
  const checkUpdates = vi.fn(async () => {})
  const { bridge } = fakeBridge({ checkUpdates })
  const services = servicesFor(bridge, withTs({}, { phase: 'off' }, true))
  renderWith(<ConnectionScreen />, services)
  expect(screen.queryByText('Установлена последняя версия')).toBeNull()
  const show = (update: Snapshot['update']): void =>
    act(() => services.store.getState().setSnapshot({ ...withTs({}, { phase: 'off' }, true), update }))
  show({ kind: 'upToDate' })
  expect(screen.getByText('Установлена последняя версия')).toBeTruthy()
  show({ kind: 'checkFailed', reason: 'HTTP 403 (GitHub request limit?)' })
  expect(screen.getByText('Не удалось проверить обновления: HTTP 403 (превышен лимит запросов к GitHub?)')).toBeTruthy()
  show({ kind: 'available', version: '1.2.0', notesUrl: NOTES, hidden: true })
  expect(screen.getByText('Доступна версия 1.2.0')).toBeTruthy()
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Проверить обновления' }))
  })
  expect(checkUpdates).toHaveBeenCalledTimes(1)
})

test('the check button is disabled while checking or downloading', () => {
  const { bridge } = fakeBridge()
  const services = servicesFor(bridge, withTs({}, { phase: 'off' }, true))
  renderWith(<ConnectionScreen />, services)
  const button = (): HTMLButtonElement => screen.getByRole('button', { name: 'Проверить обновления' }) as HTMLButtonElement
  const show = (update: Snapshot['update']): void =>
    act(() => services.store.getState().setSnapshot({ ...withTs({}, { phase: 'off' }, true), update }))
  expect(button().disabled).toBe(false)
  show({ kind: 'checking', manual: true })
  expect(button().disabled).toBe(true)
  expect(screen.getByText('Проверка обновлений…')).toBeTruthy()
  show({ kind: 'downloading', version: '1.2.0', notesUrl: NOTES, percent: 3 })
  expect(button().disabled).toBe(true)
  show({ kind: 'verifying', version: '1.2.0', notesUrl: NOTES })
  expect(button().disabled).toBe(true)
  show({ kind: 'launching', version: '1.2.0', notesUrl: NOTES })
  expect(button().disabled).toBe(true)
  show({ kind: 'upToDate' })
  expect(button().disabled).toBe(false)
})

test('the language selector saves the setting', async () => {
  const setSettings = vi.fn(async (patch: Partial<WindowSettings>) => ({ ...DEFAULT_SETTINGS, ...patch }))
  const { bridge } = fakeBridge({ setSettings })
  const services = servicesFor(bridge, withTs({}, { phase: 'off' }, true))
  renderWith(<ConnectionScreen />, services)
  const select = (name: string): HTMLSelectElement => screen.getByRole('combobox', { name }) as HTMLSelectElement
  const options = (s: HTMLSelectElement): [string, string, string | null][] =>
    [...s.options].map((o) => [o.value, o.textContent ?? '', o.getAttribute('lang')])

  // the names of the languages in their own language; «Как в Windows» in the language of the window
  expect(select('Язык').value).toBe('system')
  expect(options(select('Язык'))).toEqual([
    ['system', 'Как в Windows', null],
    ['ru', 'Русский', 'ru'],
    ['en', 'English', 'en']
  ])
  fireEvent.change(select('Язык'), { target: { value: 'en' } })
  expect(setSettings).toHaveBeenLastCalledWith({ language: 'en' })

  // the main process answers with a snapshot in the new language
  const s = withTs({}, { phase: 'off' }, true)
  act(() => services.store.getState().setSnapshot({ ...s, settings: { ...s.settings, language: 'en' }, language: 'en' }))
  expect(select('Language').value).toBe('en')
  expect(options(select('Language'))).toEqual([
    ['system', 'Same as Windows', null],
    ['ru', 'Русский', 'ru'],
    ['en', 'English', 'en']
  ])
  fireEvent.change(select('Language'), { target: { value: 'system' } })
  expect(setSettings).toHaveBeenLastCalledWith({ language: 'system' })
  fireEvent.change(select('Language'), { target: { value: 'ru' } })
  expect(setSettings).toHaveBeenLastCalledWith({ language: 'ru' })
  expect(setSettings).toHaveBeenCalledTimes(3)

  // a failed save says so, in the language of the window
  setSettings.mockRejectedValueOnce(new Error('EPERM'))
  await act(async () => {
    fireEvent.change(select('Language'), { target: { value: 'ru' } })
  })
  expect(services.store.getState().notice).toBe("Couldn't save the setting")
})

test('the checkUpdates toggle saves the setting', () => {
  const setSettings = vi.fn(async () => DEFAULT_SETTINGS)
  const { bridge } = fakeBridge({ setSettings })
  renderWith(<ConnectionScreen />, servicesFor(bridge, withTs({}, { phase: 'off' }, true)))
  const sw = screen.getByRole('switch', { name: 'Проверять обновления' })
  expect(sw.getAttribute('aria-checked')).toBe('true')
  fireEvent.click(sw)
  expect(setSettings).toHaveBeenLastCalledWith({ checkUpdates: false })
})

test('ConnectScreen has the language selector, which saves the setting', () => {
  const setSettings = vi.fn(async (patch: Partial<WindowSettings>) => ({ ...DEFAULT_SETTINGS, ...patch }))
  const { bridge } = fakeBridge({ setSettings })
  renderWith(<ConnectScreen />, servicesFor(bridge, withTs({ backend: 'needs_login' })))
  fireEvent.change(screen.getByRole('combobox', { name: 'Язык' }), { target: { value: 'en' } })
  expect(setSettings).toHaveBeenLastCalledWith({ language: 'en' })
})

test('the back control is the first thing of «Подключение» and is not repeated at the bottom', () => {
  const { bridge } = fakeBridge()
  const services = servicesFor(bridge, withTs({}, { phase: 'off' }, true))
  services.store.getState().showConnection(true)
  renderWith(<ConnectionScreen />, services)
  const back = screen.getAllByRole('button', { name: 'Назад к дорожкам' })
  expect(back).toHaveLength(1)
  expect(screen.getAllByRole('button')[0]).toBe(back[0])
  const title = screen.getByRole('heading', { name: 'Подключение' })
  expect(back[0]!.compareDocumentPosition(title) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(back[0]!.textContent).toBe('← Назад к дорожкам')
  fireEvent.click(back[0]!)
  expect(services.store.getState().ui.screen).toBe('board')

  setLang('en')
  cleanup()
  renderWith(<ConnectionScreen />, services)
  expect(screen.getByRole('button', { name: 'Back to lanes' }).textContent).toBe('← Back to lanes')
})

test('Esc closes «Подключение» unless a select or a listbox has the key', () => {
  const { bridge } = fakeBridge()
  const services = servicesFor(bridge, withTs({}, { phase: 'off' }, true))
  services.store.getState().showConnection(true)
  renderWith(<ConnectionScreen />, services)
  fireEvent.keyDown(screen.getByRole('combobox', { name: 'Язык' }), { key: 'Escape' })
  expect(services.store.getState().ui.screen).toBe('connection')
  const box = document.createElement('div')
  box.setAttribute('role', 'listbox')
  document.body.append(box)
  fireEvent.keyDown(box, { key: 'Escape' })
  box.remove()
  expect(services.store.getState().ui.screen).toBe('connection')
  fireEvent.keyDown(screen.getByRole('heading', { name: 'Подключение' }), { key: 'Enter' })
  expect(services.store.getState().ui.screen).toBe('connection')
  fireEvent.keyDown(screen.getByRole('heading', { name: 'Подключение' }), { key: 'Escape' })
  expect(services.store.getState().ui.screen).toBe('board')
})

test('Esc does nothing once «Подключение» is gone', () => {
  const { bridge } = fakeBridge()
  const services = servicesFor(bridge, withTs({}, { phase: 'off' }, true))
  services.store.getState().showConnection(true)
  const view = renderWith(<ConnectionScreen />, services)
  view.unmount()
  fireEvent.keyDown(document.body, { key: 'Escape' })
  expect(services.store.getState().ui.screen).toBe('connection')
})

test('the exit node of «Подключение» is one radio group: the selected one checked, no duplicate row', async () => {
  const { bridge, commands } = fakeBridge()
  const exitNodes = [
    { id: '100.64.0.2', name: 'node-a', online: true },
    { id: '100.64.0.3', name: 'node-b', online: false }
  ]
  renderWith(
    <ConnectionScreen />,
    servicesFor(bridge, withTs({ exitNodes, selectedExitNode: '100.64.0.2' }, { phase: 'on' }, true))
  )
  expect(screen.queryByText('Выбранный exit node')).toBeNull()
  const group = screen.getByRole('radiogroup', { name: 'Exit node' })
  const [a, b] = within(group).getAllByRole('radio') as HTMLInputElement[]
  expect(a!.checked).toBe(true)
  expect(b!.checked).toBe(false)
  expect(screen.getByText('не в сети')).toBeTruthy()
  // the hint about the restart stays below the group while routing is live
  const hint = screen.getByText('Смена exit node ненадолго перезапустит деление')
  expect(group.compareDocumentPosition(hint) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  fireEvent.click(b!)
  await vi.waitFor(() => expect(commands).toEqual([{ method: 'tailscale.setExitNode', id: '100.64.0.3' }]), { timeout: 2000 })
})

test('«Подключение» shows the remembered name as the checked radio while the list is empty', () => {
  const { bridge } = fakeBridge()
  const base = withTs({ exitNodes: [], selectedExitNode: '100.64.0.2' }, { phase: 'off' }, true)
  const named = { ...base, settings: { ...base.settings, lastExitNode: { id: '100.64.0.2', name: 'node-a' } } }
  const services = servicesFor(bridge, named)
  renderWith(<ConnectionScreen />, services)
  expect((screen.getByRole('radio', { name: 'node-a' }) as HTMLInputElement).checked).toBe(true)
  act(() => services.store.getState().setSnapshot(base))
  expect((screen.getByRole('radio', { name: '100.64.0.2' }) as HTMLInputElement).checked).toBe(true)
})

test('«Подключение» offers the banner actions next to the update line', async () => {
  const installUpdate = vi.fn(async () => {})
  const cancelUpdate = vi.fn(async () => {})
  const { bridge } = fakeBridge({ installUpdate, cancelUpdate })
  const services = servicesFor(bridge, withTs({}, { phase: 'off' }, true))
  renderWith(<ConnectionScreen />, services)
  const show = (update: Snapshot['update']): void =>
    act(() => services.store.getState().setSnapshot({ ...withTs({}, { phase: 'off' }, true), update }))
  const click = async (name: string): Promise<void> => {
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name }))
    })
  }

  show({ kind: 'available', version: '1.2.0', notesUrl: NOTES, hidden: true })
  const line = screen.getByText('Доступна версия 1.2.0').closest('[role="status"]') as HTMLElement
  await act(async () => {
    fireEvent.click(within(line).getByRole('button', { name: 'Обновить' }))
  })
  expect(installUpdate).toHaveBeenCalledTimes(1)

  show({ kind: 'installRefused', version: '1.2.0', notesUrl: NOTES, reason: '', canRetry: true })
  await click('Повторить')
  expect(installUpdate).toHaveBeenCalledTimes(2)

  show({ kind: 'downloading', version: '1.2.0', notesUrl: NOTES, percent: 4 })
  await click('Отменить')
  expect(cancelUpdate).toHaveBeenCalledTimes(1)

  show({ kind: 'installRefused', version: '1.2.0', notesUrl: NOTES, reason: '', canRetry: false })
  expect(screen.queryByRole('button', { name: 'Повторить' })).toBeNull()
  show({ kind: 'upToDate' })
  expect(screen.queryByRole('button', { name: 'Обновить' })).toBeNull()
})

test('the notifications toggle saves the setting', () => {
  const setSettings = vi.fn(async () => DEFAULT_SETTINGS)
  const { bridge } = fakeBridge({ setSettings })
  renderWith(<ConnectionScreen />, servicesFor(bridge, withTs({}, { phase: 'off' }, true)))
  const sw = screen.getByRole('switch', { name: 'Показывать уведомления' })
  expect(sw.getAttribute('aria-checked')).toBe('true')
  fireEvent.click(sw)
  expect(setSettings).toHaveBeenLastCalledWith({ notifications: false })
})
