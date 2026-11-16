import { act, cleanup, fireEvent, screen, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import type { Snapshot, VpsEvent, VpsOutcome, VpsRequest } from '../../shared/ipc'
import type { HelperState, Subscription } from '../../shared/protocol'
import { DEFAULT_SETTINGS } from '../../shared/settings'
import { App } from '../App'
import { setLang } from '../i18n/t'
import { fakeBridge, proxyServer, proxyState, renderWith, servicesFor, snap, state } from '../testing'
import { ConnectionScreen, ConnectScreen } from './ConnectScreen'

afterEach(() => {
  cleanup()
  setLang('ru')
})

const SUB: Subscription = { id: 'sub-0123456789abcdef', name: '203-0-113-10.sslip.io', updatedAt: '2026-11-16T10:00:00Z', count: 2, error: null }
const A = proxyServer('0123456789abcdef', 'SplitLane-VLESS', { source: SUB.id })
const B = proxyServer('fedcba9876543210', 'SplitLane-Hysteria2', { protocol: 'hysteria2', source: SUB.id })
const DONE: VpsOutcome = { kind: 'done', servers: 2, subscriptionId: SUB.id, warnings: [] }

const at = (s: HelperState, once = false): Snapshot => ({ ...snap(), state: s, settings: { ...DEFAULT_SETTINGS, connectedOnce: once } })
const firstRun = (): HelperState =>
  state({ phase: 'off', tailscale: { ...state().tailscale, backend: 'needs_login', selectedExitNode: null, exitNodes: [], path: null } })
const added = (): HelperState => proxyState([A, B], A.id, [SUB], { phase: 'off' })

function vpsBridge() {
  let emit: (e: VpsEvent) => void = () => {}
  let answer: (o: VpsOutcome) => void = () => {}
  const vpsSetup = vi.fn((_req: VpsRequest) => new Promise<VpsOutcome>((r) => (answer = r)))
  const vpsForgetKey = vi.fn(async () => {})
  const fake = fakeBridge({
    vpsSetup,
    vpsForgetKey,
    onVpsEvent: (cb) => {
      emit = cb
      return () => {}
    }
  })
  return {
    ...fake,
    vpsSetup,
    vpsForgetKey,
    emit: (e: VpsEvent) => act(() => emit(e)),
    answer: (o: VpsOutcome) =>
      act(async () => {
        answer(o)
      })
  }
}

const tiles = (name = 'Способы подключения'): HTMLElement[] => within(screen.getByRole('radiogroup', { name })).getAllByRole('radio')
const tile = (name: RegExp): HTMLElement => within(screen.getByRole('radiogroup', { name: 'Способы подключения' })).getByRole('radio', { name })
const steps = (): (string | null)[] => within(screen.getByRole('list', { name: 'Шаги' })).getAllByRole('listitem').map((li) => li.textContent)
const currentStep = (): string | null | undefined =>
  within(screen.getByRole('list', { name: 'Шаги' }))
    .getAllByRole('listitem')
    .find((li) => li.getAttribute('aria-current') === 'step')?.textContent

function fillAndStart(): void {
  fireEvent.change(screen.getByRole('textbox', { name: 'Адрес сервера' }), { target: { value: '203.0.113.10' } })
  fireEvent.change(screen.getByLabelText('Пароль', { selector: 'input[type="password"]' }), { target: { value: 'made-up password' } })
  fireEvent.click(screen.getByRole('button', { name: 'Настроить' }))
}

test('«Свой VPS» is the fourth tile of both screens; the arrows go round all four', () => {
  const { bridge } = vpsBridge()
  renderWith(<ConnectScreen />, servicesFor(bridge, at(firstRun())))
  const [ts, , file, vps] = tiles()
  expect(tiles().map((r) => r.textContent)).toEqual([
    'Tailscaleexit node вашей сети Tailscale',
    'Ключ или подпискаVLESS, Trojan, Shadowsocks и другие',
    'Файл настроекWireGuard, файл .conf',
    'Свой VPSSplitLane сам настроит арендованный сервер'
  ])
  fireEvent.click(file!)
  file!.focus()
  fireEvent.keyDown(file!, { key: 'ArrowRight' })
  expect(vps!.getAttribute('aria-checked')).toBe('true')
  expect(document.activeElement).toBe(vps)
  fireEvent.keyDown(vps!, { key: 'ArrowDown' })
  expect(ts!.getAttribute('aria-checked')).toBe('true')
  fireEvent.keyDown(ts!, { key: 'End' })
  expect(vps!.getAttribute('aria-checked')).toBe('true')
  fireEvent.keyDown(vps!, { key: 'ArrowLeft' })
  expect(file!.getAttribute('aria-checked')).toBe('true')
  cleanup()

  renderWith(<ConnectionScreen />, servicesFor(bridge, at(added(), true)))
  expect(tiles().map((r) => r.textContent)).toEqual(['Tailscale', 'Ключ или подписка', 'Файл настроек', 'Свой VPS'])
  cleanup()

  setLang('en')
  renderWith(<ConnectionScreen />, servicesFor(bridge, { ...at(added(), true), language: 'en' }))
  expect(tiles('Connection methods').map((r) => r.textContent)).toEqual(['Tailscale', 'Key or subscription', 'Configuration file', 'Your own VPS'])
})

test('the wizard: enter the server, wait for the setup, then «Включить деление»', async () => {
  const v = vpsBridge()
  const services = servicesFor(v.bridge, at(firstRun()))
  renderWith(<ConnectScreen />, services)
  fireEvent.click(tile(/^Свой VPS/))
  expect(steps()).toEqual(['1Введите данные сервера', '2Дождитесь настройки', '3Включите деление'])
  expect(currentStep()).toBe('1Введите данные сервера')
  expect(screen.queryByRole('button', { name: 'Войти в Tailscale' })).toBeNull()
  expect(screen.queryByRole('textbox', { name: 'Ссылки или адрес подписки' })).toBeNull()

  fillAndStart()
  expect(v.vpsSetup).toHaveBeenCalledWith({ address: '203.0.113.10', user: 'root', password: 'made-up password', useKey: false })
  expect(currentStep()).toBe('2Дождитесь настройки')
  await v.emit({ step: 'add', state: 'start' })
  // the add selects the first server: the state comes before the answer
  act(() => services.store.getState().setSnapshot(at(added())))
  expect(screen.queryByRole('button', { name: 'Включить деление' })).toBeNull()
  await v.answer(DONE)
  expect(currentStep()).toBe('3Включите деление')
  expect(screen.getByText('Готово. Добавлено серверов: 2')).toBeTruthy()
  // the wizard has no list: the server is chosen, the next thing is the enable
  expect(screen.queryByRole('group', { name: 'Серверы' })).toBeNull()
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Включить деление' }))
  })
  expect(v.commands).toEqual([{ method: 'routing.enable' }])
})

test('the wizard offers no enable on the VPS tile before a setup, even with a server in force', () => {
  const v = vpsBridge()
  renderWith(<ConnectScreen />, servicesFor(v.bridge, at(proxyState([A], A.id, [], { phase: 'off' }))))
  fireEvent.click(tile(/^Свой VPS/))
  expect(currentStep()).toBe('1Введите данные сервера')
  expect(screen.queryByRole('button', { name: 'Включить деление' })).toBeNull()
})

test('a failed setup in the wizard stays on the wait step until «Попробовать снова»', async () => {
  const v = vpsBridge()
  renderWith(<ConnectScreen />, servicesFor(v.bridge, at(firstRun())))
  fireEvent.click(tile(/^Свой VPS/))
  fillAndStart()
  await v.answer({ kind: 'failed', code: 'ssh_unreachable', detail: 'timeout' })
  expect(currentStep()).toBe('2Дождитесь настройки')
  expect(screen.getByRole('alert').textContent).toMatch(/^Не удалось подключиться к серверу/)
  fireEvent.click(screen.getByRole('button', { name: 'Попробовать снова' }))
  expect(currentStep()).toBe('1Введите данные сервера')
  expect((screen.getByRole('textbox', { name: 'Адрес сервера' }) as HTMLInputElement).value).toBe('203.0.113.10')
})

test('«Подключение»: the setup, then the list shows the new servers', async () => {
  const v = vpsBridge()
  const services = servicesFor(v.bridge, at(proxyState([], null, [], { phase: 'off' }), true))
  services.store.getState().showConnection(true)
  renderWith(<ConnectionScreen />, services)
  fireEvent.click(tile(/^Свой VPS/))
  // no wizard steps here
  expect(screen.queryByRole('list', { name: 'Шаги' })).toBeNull()
  fillAndStart()
  act(() => services.store.getState().setSnapshot(at(added(), true)))
  await v.answer(DONE)
  expect(screen.getByText('Готово. Добавлено серверов: 2')).toBeTruthy()
  const list = screen.getByRole('group', { name: 'Серверы' })
  expect(within(list).getAllByRole('radio').map((r) => (r as HTMLInputElement).checked)).toEqual([true, false])
  expect(screen.queryByRole('button', { name: 'Включить деление' })).toBeNull()
  // the settings stay below
  expect(screen.getByRole('switch', { name: 'Запускать при входе в Windows' })).toBeTruthy()
})

test('Esc in a field of the form does not leave «Подключение» (what was typed would be lost)', () => {
  const v = vpsBridge()
  const services = servicesFor(v.bridge, at(added(), true))
  services.store.getState().showConnection(true)
  renderWith(<ConnectionScreen />, services)
  fireEvent.click(tile(/^Свой VPS/))
  fireEvent.keyDown(screen.getByRole('textbox', { name: 'Адрес сервера' }), { key: 'Escape' })
  fireEvent.keyDown(screen.getByLabelText('Пароль', { selector: 'input[type="password"]' }), { key: 'Escape' })
  expect(services.store.getState().ui.screen).toBe('connection')
  // a radio is no field: Esc goes back
  fireEvent.keyDown(screen.getByRole('radio', { name: 'Пароль' }), { key: 'Escape' })
  expect(services.store.getState().ui.screen).toBe('board')
})

test('leaving the tile forgets the key; coming back while the setup runs opens on its progress', async () => {
  const v = vpsBridge()
  const services = servicesFor(v.bridge, at(proxyState([], null, [], { phase: 'off' }), true))
  services.store.getState().showConnection(true)
  const first = renderWith(<ConnectionScreen />, services)
  fireEvent.click(tile(/^Свой VPS/))
  fireEvent.click(tile(/^Ключ или подписка/))
  expect(v.vpsForgetKey).toHaveBeenCalledTimes(1)
  fireEvent.click(tile(/^Свой VPS/))
  fillAndStart()
  await v.emit({ step: 'connect', state: 'start' })
  first.unmount()
  expect(v.vpsForgetKey).toHaveBeenCalledTimes(2)

  renderWith(<ConnectionScreen />, services)
  expect(tile(/^Свой VPS/).getAttribute('aria-checked')).toBe('true')
  expect(within(screen.getByRole('list', { name: 'Ход настройки' })).getAllByRole('listitem')[0]!.textContent).toBe(
    'Подключение к серверуидёт'
  )
})

test('a setup that ends after the wizard went (the board took over) is told by a notice', async () => {
  const v = vpsBridge()
  // after the first run with an empty list: the connect screen is due until a server is there
  const services = servicesFor(v.bridge, at(proxyState([], null, [], { phase: 'off' }), true))
  renderWith(<App />, services)
  expect(screen.getByRole('heading', { level: 1 }).textContent).toBe('Подключите VPN')
  fireEvent.click(tile(/^Свой VPS/))
  fillAndStart()
  act(() => services.store.getState().setSnapshot(at(added(), true)))
  expect(screen.getByRole('region', { name: 'Через VPN' })).toBeTruthy()
  await v.answer(DONE)
  expect(services.store.getState().notice).toBe('Готово. Добавлено серверов: 2')
  expect(screen.getByText('Готово. Добавлено серверов: 2')).toBeTruthy()
  expect(services.store.getState().vps).toBeNull()
})
