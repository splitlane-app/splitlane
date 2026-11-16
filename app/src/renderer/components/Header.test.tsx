import { act, cleanup, fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'
import { SETUP_RUNNING } from '../../shared/ipc'
import { setLang } from '../i18n/t'
import { commandErrorText } from '../model/proxy'
import { newRun, runEnd } from '../model/vps'
import { fakeBridge, proxyServer, proxyState, renderWith, servicesFor, snap, state } from '../testing'
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

test('the pill shows the remembered name of the exit node while the list is empty, the address only without one', () => {
  const { bridge } = fakeBridge()
  const off = (lastExitNode: { id: string; name: string } | null) => {
    const base = snap({ phase: 'off' })
    const st = base.state!
    return {
      ...base,
      state: { ...st, tailscale: { ...st.tailscale, exitNodes: [], selectedExitNode: '100.64.0.2', path: null } },
      settings: { ...base.settings, lastExitNode }
    }
  }
  const services = servicesFor(bridge, off({ id: '100.64.0.2', name: 'node-a' }))
  renderWith(<Header />, services)
  expect(screen.getByRole('button', { name: 'Tailscale · node-a' })).toBeTruthy()

  // a name remembered for another node is not used
  act(() => services.store.getState().setSnapshot(off({ id: '100.64.0.9', name: 'other' })))
  expect(screen.getByRole('button', { name: 'Tailscale · 100.64.0.2' })).toBeTruthy()
  act(() => services.store.getState().setSnapshot(off(null)))
  expect(screen.getByRole('button', { name: 'Tailscale · 100.64.0.2' })).toBeTruthy()
})

test('the pill of the proxy: «<server> · <protocol>», up only while routing is on', () => {
  const { bridge } = fakeBridge()
  const a = proxyServer('0123456789abcdef', 'edge-a.example.com', { protocol: 'hysteria2' })
  const at = (s: ReturnType<typeof proxyState>) => ({ ...snap(), state: s })
  const services = servicesFor(bridge, at(proxyState([a], a.id, [], { phase: 'on' })))
  renderWith(<Header />, services)
  const pill = (): HTMLElement => screen.getByRole('button', { name: /edge-a|сервер не выбран/ })
  expect(pill().textContent).toBe('edge-a.example.com · Hysteria2')
  expect(pill().dataset.tone).toBe('up')
  // the Tailscale part (node, path) is not shown for the proxy
  expect(screen.queryByText(/Tailscale/)).toBeNull()

  act(() => services.store.getState().setSnapshot(at(proxyState([a], a.id, [], { phase: 'off' }))))
  expect(pill().dataset.tone).toBe('idle')
  act(() => services.store.getState().setSnapshot(at(proxyState([a], a.id, [], { phase: 'degraded' }))))
  expect(pill().dataset.tone).toBe('warn')
  act(() => services.store.getState().setSnapshot(at(proxyState([a], null, [], { phase: 'off' }))))
  expect(pill().textContent).toBe('сервер не выбран')
})

test('while a server setup runs the toggle cannot turn split routing on, and says why', async () => {
  const { bridge, commands } = fakeBridge()
  const services = servicesFor(bridge, snap({ phase: 'off' }))
  services.store.getState().setVps(() => newRun(0, false))
  renderWith(<Header />, services)
  const toggle = screen.getByRole('switch') as HTMLButtonElement
  expect(toggle.disabled).toBe(true)
  expect(toggle.title).toBe('Дождитесь конца настройки сервера')
  // the end of the setup gives it back
  act(() => services.store.getState().setVps((r) => (r === null ? r : runEnd(r, { kind: 'cancelled' }, 1))))
  expect(toggle.disabled).toBe(false)
  expect(toggle.title).toBe('')
  await act(async () => {
    fireEvent.click(toggle)
  })
  expect(commands).toEqual([{ method: 'routing.enable' }])
})

test('an enable the main process refuses for a running setup is told by its notice (a page that reloaded meanwhile)', async () => {
  const { bridge } = fakeBridge({}, { ok: false, error: SETUP_RUNNING })
  const services = servicesFor(bridge, snap({ phase: 'off' }))
  renderWith(<Header />, services)
  await act(async () => {
    fireEvent.click(screen.getByRole('switch'))
  })
  expect(services.store.getState().notice).toBe('Дождитесь конца настройки сервера')
  setLang('en')
  expect(commandErrorText({ method: 'routing.enable' }, SETUP_RUNNING)).toBe('Wait for the server setup to finish')
  // another busy of the helper keeps its own text
  expect(commandErrorText({ method: 'routing.enable' }, { code: 'busy', message: 'busy' })).toBe(
    'The service is busy. Try again in a few seconds'
  )
  setLang('ru')
})
