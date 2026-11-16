import { act, cleanup, fireEvent, screen, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import type { ProxyOutcome, Snapshot } from '../../shared/ipc'
import type { HelperState, Subscription } from '../../shared/protocol'
import { DEFAULT_SETTINGS } from '../../shared/settings'
import { setLang } from '../i18n/t'
import { fakeBridge, proxyServer, proxyState, renderWith, servicesFor, snap, state } from '../testing'
import { App } from '../App'
import { ConnectionScreen, ConnectScreen } from './ConnectScreen'

afterEach(() => {
  cleanup()
  setLang('ru')
})

const SUB: Subscription = { id: 'sub-0123456789abcdef', name: 'sub.example.com', updatedAt: '2026-11-16T10:00:00Z', count: 2, error: null }
const A = proxyServer('0123456789abcdef', 'edge-a.example.com')
const B = proxyServer('fedcba9876543210', 'edge-b.example.com', { protocol: 'trojan', source: SUB.id })

const at = (s: HelperState, once = false): Snapshot => ({ ...snap(), state: s, settings: { ...DEFAULT_SETTINGS, connectedOnce: once } })
const firstRun = (over: Partial<HelperState['tailscale']> = {}): HelperState =>
  state({ phase: 'off', tailscale: { ...state().tailscale, backend: 'needs_login', selectedExitNode: null, exitNodes: [], path: null, ...over } })

const tile = (name: RegExp): HTMLElement => within(screen.getByRole('radiogroup', { name: 'Способы подключения' })).getByRole('radio', { name })
const checkedTile = (): string | undefined =>
  within(screen.getByRole('radiogroup', { name: 'Способы подключения' }))
    .getAllByRole('radio')
    .find((r) => r.getAttribute('aria-checked') === 'true')?.textContent ?? undefined
const steps = (): (string | null)[] => within(screen.getByRole('list', { name: 'Шаги' })).getAllByRole('listitem').map((li) => li.textContent)
const currentStep = (): string | null | undefined =>
  within(screen.getByRole('list', { name: 'Шаги' }))
    .getAllByRole('listitem')
    .find((li) => li.getAttribute('aria-current') === 'step')?.textContent

test('the tiles are one radio group: the arrows, Home and End choose and move the focus; Tab enters on the chosen one', () => {
  const { bridge } = fakeBridge()
  renderWith(<ConnectScreen />, servicesFor(bridge, at(firstRun())))
  const [ts, key, file] = within(screen.getByRole('radiogroup', { name: 'Способы подключения' })).getAllByRole('radio')
  expect(checkedTile()).toMatch(/^Tailscale/)
  expect([ts, key, file].map((r) => r!.tabIndex)).toEqual([0, -1, -1])
  // the notes say what each tile is; «скоро» is gone
  expect(screen.queryByText('скоро')).toBeNull()
  expect(key!.getAttribute('aria-describedby')).not.toBeNull()

  ts!.focus()
  fireEvent.keyDown(ts!, { key: 'ArrowRight' })
  expect(checkedTile()).toMatch(/^Ключ или подписка/)
  expect(document.activeElement).toBe(key)
  expect([ts, key, file].map((r) => r!.tabIndex)).toEqual([-1, 0, -1])
  expect(steps()).toEqual(['1Добавьте ключ или подписку', '2Выберите сервер', '3Включите деление'])

  fireEvent.keyDown(key!, { key: 'ArrowDown' })
  expect(checkedTile()).toMatch(/^Файл настроек/)
  expect(steps()[0]).toBe('1Выберите файл настроек')
  fireEvent.keyDown(file!, { key: 'ArrowRight' })
  expect(checkedTile()).toMatch(/^Tailscale/)
  expect(document.activeElement).toBe(ts)
  fireEvent.keyDown(ts!, { key: 'ArrowLeft' })
  expect(checkedTile()).toMatch(/^Файл настроек/)
  fireEvent.keyDown(file!, { key: 'Home' })
  expect(checkedTile()).toMatch(/^Tailscale/)
  expect(steps()).toEqual(['1Войдите в Tailscale', '2Выберите exit node', '3Включите деление'])
  fireEvent.keyDown(ts!, { key: 'End' })
  expect(checkedTile()).toMatch(/^Файл настроек/)
  fireEvent.click(tile(/Ключ или подписка/))
  expect(checkedTile()).toMatch(/^Ключ или подписка/)
})

test('the screen opens on the tile of the connector in force', () => {
  const { bridge } = fakeBridge()
  renderWith(<ConnectScreen />, servicesFor(bridge, at(proxyState([A], A.id, [], { phase: 'off' }))))
  expect(checkedTile()).toMatch(/^Ключ или подписка/)
})

test('the key tile walks add, choose a server, enable; the enable only with a server in force', async () => {
  const { bridge, commands } = fakeBridge()
  const services = servicesFor(bridge, at(firstRun()))
  renderWith(<ConnectScreen />, services)
  fireEvent.click(tile(/Ключ или подписка/))
  expect(currentStep()).toBe('1Добавьте ключ или подписку')
  expect(screen.getByRole('textbox', { name: 'Ссылки или адрес подписки' })).toBeTruthy()
  expect(screen.queryByRole('button', { name: 'Войти в Tailscale' })).toBeNull()
  const enable = (): HTMLElement | null => screen.queryByRole('button', { name: 'Включить деление' })
  expect(enable()).toBeNull()

  // the server was added for later while Tailscale stays the connector: it is still to be chosen
  const later = firstRun()
  act(() => services.store.getState().setSnapshot(at({ ...later, proxy: { servers: [A, B], selected: A.id, subscriptions: [SUB] } })))
  expect(currentStep()).toBe('2Выберите сервер')
  expect(enable()).toBeNull()
  expect((screen.getByRole('radio', { name: /edge-a/ }) as HTMLInputElement).checked).toBe(false)
  fireEvent.click(screen.getByRole('radio', { name: /edge-a/ }))
  await vi.waitFor(() => expect(commands).toEqual([{ method: 'proxy.select', id: A.id }]))

  // the proxy connector without a selected server
  act(() => services.store.getState().setSnapshot(at(proxyState([A, B], null, [SUB], { phase: 'off' }))))
  expect(currentStep()).toBe('2Выберите сервер')
  expect(enable()).toBeNull()

  act(() => services.store.getState().setSnapshot(at(proxyState([A, B], A.id, [SUB], { phase: 'off' }))))
  expect(currentStep()).toBe('3Включите деление')
  expect((screen.getByRole('radio', { name: /edge-a/ }) as HTMLInputElement).checked).toBe(true)
  await act(async () => {
    fireEvent.click(enable()!)
  })
  expect(commands.at(-1)).toEqual({ method: 'routing.enable' })
})

test('the file tile chooses a file; with a server in force the enable warns about the exit node of the Tailscale app', async () => {
  const proxyAddFile = vi.fn(async (): Promise<ProxyOutcome> => ({ kind: 'cancelled' }))
  const { bridge } = fakeBridge({ proxyAddFile })
  const services = servicesFor(bridge, at(firstRun()))
  renderWith(<ConnectScreen />, services)
  fireEvent.click(tile(/Файл настроек/))
  expect(currentStep()).toBe('1Выберите файл настроек')
  expect(screen.queryByRole('textbox')).toBeNull()
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Выбрать файл .conf…' }))
  })
  expect(proxyAddFile).toHaveBeenCalledTimes(1)

  const wg = proxyServer('00000000000000c0', 'home', { protocol: 'wireguard', source: 'file' })
  const s = proxyState([wg], wg.id, [], { phase: 'off', systemTailscale: { installed: true, exitNodeOn: true, restoreOnDisable: false } })
  act(() => services.store.getState().setSnapshot(at(s)))
  expect(currentStep()).toBe('3Включите деление')
  expect(screen.getByText(/В приложении Tailscale сейчас включён exit node/)).toBeTruthy()
  expect(screen.getByRole('button', { name: 'Включить деление' })).toBeTruthy()
})

test('the Tailscale tile while a server is in force: the exit node is chosen again, there is no enable yet', async () => {
  const { bridge, commands } = fakeBridge()
  const s = proxyState([A], A.id, [], { phase: 'off', tailscale: { ...state().tailscale, backend: 'stopped', path: null } })
  renderWith(<ConnectScreen />, servicesFor(bridge, at(s)))
  fireEvent.click(tile(/^Tailscale/))
  expect(currentStep()).toBe('2Выберите exit node')
  const node = screen.getByRole('radio', { name: 'node-a' }) as HTMLInputElement
  expect(node.checked).toBe(false)
  expect(screen.queryByRole('button', { name: 'Включить деление' })).toBeNull()
  fireEvent.click(node)
  await vi.waitFor(() => expect(commands).toEqual([{ method: 'tailscale.setExitNode', id: '100.64.0.2' }]))
})

test('«Подключение» opens on the part of the connector in force and switches with the tiles', () => {
  const { bridge } = fakeBridge()
  renderWith(<ConnectionScreen />, servicesFor(bridge, at(proxyState([A, B], A.id, [SUB], { phase: 'off' }), true)))
  expect(checkedTile()).toBe('Ключ или подписка')
  expect(screen.getByRole('textbox', { name: 'Ссылки или адрес подписки' })).toBeTruthy()
  expect(screen.getByRole('group', { name: 'Серверы' })).toBeTruthy()
  expect(screen.getByRole('button', { name: 'Удалить edge-a.example.com' })).toBeTruthy()
  expect(screen.getByRole('heading', { name: 'Подписки' })).toBeTruthy()
  expect(screen.queryByRole('button', { name: 'Выйти из Tailscale' })).toBeNull()
  expect(screen.queryByText('Устройство в сети Tailscale')).toBeNull()

  fireEvent.click(tile(/^Tailscale/))
  expect(screen.getByText('Устройство в сети Tailscale')).toBeTruthy()
  expect(screen.getByRole('radiogroup', { name: 'Exit node' })).toBeTruthy()
  expect(screen.getByRole('button', { name: 'Выйти из Tailscale' })).toBeTruthy()
  expect(screen.queryByRole('heading', { name: 'Подписки' })).toBeNull()

  fireEvent.click(tile(/Файл настроек/))
  expect(screen.getByRole('button', { name: 'Выбрать файл .conf…' })).toBeTruthy()
  expect(screen.queryByRole('textbox')).toBeNull()
  // the settings stay below either part
  expect(screen.getByRole('switch', { name: 'Запускать при входе в Windows' })).toBeTruthy()
})

// a user of the proxy list who has never logged in to Tailscale: the node is stopped, no exit node is known
const neverLoggedIn = { ...state().tailscale, backend: 'stopped' as const, selectedExitNode: null, exitNodes: [], path: null }

test('«Подключение»: a user of the proxy list logs in to Tailscale there, then chooses an exit node', async () => {
  const openLogin = vi.fn(async () => true)
  const { bridge, commands } = fakeBridge({ openLogin })
  const services = servicesFor(bridge, at(proxyState([A], A.id, [], { phase: 'off', tailscale: neverLoggedIn }), true))
  services.store.getState().showConnection(true)
  renderWith(<App />, services)
  fireEvent.click(tile(/^Tailscale/))
  expect(screen.getByRole('heading', { level: 1 }).textContent).toBe('Подключение')
  expect(screen.getByText('Откроется страница входа в браузере')).toBeTruthy()
  expect(screen.queryByRole('button', { name: 'Выйти из Tailscale' })).toBeNull()
  expect(screen.queryByText('Устройство в сети Tailscale')).toBeNull()
  expect(screen.queryByRole('radiogroup', { name: 'Exit node' })).toBeNull()
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Войти в Tailscale' }))
  })
  expect(commands).toEqual([{ method: 'tailscale.login' }])

  // the setup core gives the link of this login: it opens by itself, once, and again on the button
  const link = 'https://login.tailscale.com/a/made-up'
  const waiting = { ...neverLoggedIn, backend: 'needs_login' as const, authUrl: link }
  act(() => services.store.getState().setSnapshot(at(proxyState([A], A.id, [], { phase: 'off', tailscale: waiting }), true)))
  expect(screen.getByRole('heading', { level: 1 }).textContent).toBe('Подключение')
  expect(openLogin).toHaveBeenCalledTimes(1)
  expect(openLogin).toHaveBeenLastCalledWith(link)
  expect(screen.getByText(/Страница входа открыта в браузере/)).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: 'Открыть ссылку снова' }))
  expect(openLogin).toHaveBeenCalledTimes(2)

  const approval = { ...neverLoggedIn, backend: 'needs_approval' as const }
  act(() => services.store.getState().setSnapshot(at(proxyState([A], A.id, [], { phase: 'off', tailscale: approval }), true)))
  expect(screen.getByText(/ждёт одобрения администратором/)).toBeTruthy()

  // logged in: the device, the exit nodes and «Выйти», as for a Tailscale user; choosing a node switches
  const node = { id: '100.64.0.2', name: 'node-a', online: true }
  const loggedIn = { ...neverLoggedIn, backend: 'running' as const, exitNodes: [node] }
  act(() => services.store.getState().setSnapshot(at(proxyState([A], A.id, [], { phase: 'off', tailscale: loggedIn }), true)))
  expect(screen.queryByRole('button', { name: 'Войти в Tailscale' })).toBeNull()
  expect(screen.getByText('Устройство в сети Tailscale')).toBeTruthy()
  expect(screen.getByRole('button', { name: 'Выйти из Tailscale' })).toBeTruthy()
  fireEvent.click(screen.getByRole('radio', { name: 'node-a' }))
  await vi.waitFor(() => expect(commands.at(-1)).toEqual({ method: 'tailscale.setExitNode', id: '100.64.0.2' }))
})

test('while routing runs on a server the Tailscale login waits for it to be off, on both screens', () => {
  for (const phase of ['on', 'degraded', 'starting', 'stopping'] as const) {
    for (const ui of [<ConnectionScreen key="c" />, <ConnectScreen key="w" />]) {
      const { bridge } = fakeBridge()
      renderWith(ui, servicesFor(bridge, at(proxyState([A], A.id, [], { phase, tailscale: neverLoggedIn }), true)))
      fireEvent.click(tile(/^Tailscale/))
      expect(screen.getByText('Войти в Tailscale можно, когда деление выключено'), phase).toBeTruthy()
      expect(screen.queryByRole('button', { name: 'Войти в Tailscale' }), phase).toBeNull()
      expect(screen.queryByRole('button', { name: 'Выйти из Tailscale' }), phase).toBeNull()
      cleanup()
    }
  }
  // the same in English
  setLang('en')
  const { bridge } = fakeBridge()
  renderWith(
    <ConnectionScreen />,
    servicesFor(bridge, { ...at(proxyState([A], A.id, [], { phase: 'on', tailscale: neverLoggedIn }), true), language: 'en' })
  )
  fireEvent.click(within(screen.getByRole('radiogroup', { name: 'Connection methods' })).getByRole('radio', { name: /^Tailscale/ }))
  expect(screen.getByText('You can log in to Tailscale when split routing is off')).toBeTruthy()
})

test('a user of the proxy list logged in to Tailscale before sees the exit nodes and «Выйти» while routing runs on a server', () => {
  const { bridge } = fakeBridge()
  const known = { ...neverLoggedIn, exitNodes: [{ id: '100.64.0.2', name: 'node-a', online: true }] }
  renderWith(<ConnectionScreen />, servicesFor(bridge, at(proxyState([A], A.id, [], { phase: 'on', tailscale: known }), true)))
  fireEvent.click(tile(/^Tailscale/))
  expect(screen.getByRole('radiogroup', { name: 'Exit node' })).toBeTruthy()
  expect((screen.getByRole('button', { name: 'Выйти из Tailscale' }) as HTMLButtonElement).disabled).toBe(true)
  expect(screen.queryByText('Войти в Tailscale можно, когда деление выключено')).toBeNull()
})

test('a removal or a choice that the helper could not save says so of the list of servers', async () => {
  const failed = { ok: false as const, error: { code: 'apply_failed' as const, message: 'servers.json not saved' } }
  const { bridge, commands } = fakeBridge({}, failed)
  const services = servicesFor(bridge, at(proxyState([A, B], A.id, [SUB], { phase: 'off' }), true))
  renderWith(<ConnectionScreen />, services)
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Удалить edge-b.example.com' }))
  })
  expect(commands.at(-1)).toEqual({ method: 'proxy.remove', id: B.id })
  expect(services.store.getState().notice).toBe('Не удалось прочитать или сохранить список серверов')
  act(() => services.store.getState().dismissNotice())
  fireEvent.click(screen.getByRole('radio', { name: /edge-b/ }))
  await vi.waitFor(() => expect(commands.at(-1)).toEqual({ method: 'proxy.select', id: B.id }))
  await vi.waitFor(() => expect(services.store.getState().notice).toBe('Не удалось прочитать или сохранить список серверов'))
})

const subItem = (): HTMLElement => screen.getByText('sub.example.com', { selector: 'strong' }).closest('li') as HTMLElement

test('a subscription shows its servers, its last update and its last error', () => {
  const { bridge } = fakeBridge()
  const failed = { ...SUB, error: 'fetch_timeout' }
  const services = servicesFor(bridge, at(proxyState([A, B], A.id, [failed], { phase: 'off' }), true))
  renderWith(<ConnectionScreen />, services)
  const item = subItem()
  expect(within(item).getByText('Серверов: 2')).toBeTruthy()
  const time = new Intl.DateTimeFormat('ru', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(SUB.updatedAt))
  expect(within(item).getByText(`Обновлена: ${time}`)).toBeTruthy()
  expect(within(item).getByText('Последнее обновление не удалось: Сервер подписки не ответил за 20 секунд')).toBeTruthy()

  act(() => services.store.getState().setSnapshot(at(proxyState([A, B], A.id, [{ ...SUB, updatedAt: '' }], { phase: 'off' }), true)))
  expect(within(subItem()).queryByText(/^Обновлена/)).toBeNull()
  expect(within(subItem()).queryByText(/^Последнее обновление/)).toBeNull()
})

test('«Обновить»: says it is refreshing, then shows the result beside its subscription', async () => {
  let answer: (o: ProxyOutcome) => void = () => {}
  const proxyRefresh = vi.fn((_id: string) => new Promise<ProxyOutcome>((r) => (answer = r)))
  const { bridge } = fakeBridge({ proxyRefresh })
  renderWith(<ConnectionScreen />, servicesFor(bridge, at(proxyState([A, B], A.id, [SUB], { phase: 'off' }), true)))
  const refresh = (): HTMLButtonElement => within(subItem()).getByRole('button', { name: /^(Обновить|Обновляю…)$/ }) as HTMLButtonElement
  fireEvent.click(refresh())
  expect(proxyRefresh).toHaveBeenCalledWith(SUB.id)
  expect(refresh().textContent).toBe('Обновляю…')
  expect(refresh().disabled).toBe(true)
  await act(async () => {
    answer({ kind: 'done', result: { added: ['00000000000000d0'], existing: 1, rejected: [{ line: 4, code: 'bad_link' }], rejectedMore: 0 } })
  })
  expect(refresh().textContent).toBe('Обновить')
  const status = within(subItem()).getByRole('status')
  expect([...status.querySelectorAll('p, li')].map((e) => e.textContent)).toEqual([
    'Добавлено серверов: 1',
    'Уже были в списке: 1',
    'Строка 4: Ссылка повреждена или записана с ошибкой'
  ])
})

test('a refused refresh: the subscription of the selected server while live, another download, a failed download', async () => {
  const proxyRefresh = vi.fn(async (): Promise<ProxyOutcome> => ({ kind: 'failed', error: { code: 'wrong_phase', message: 'on' } }))
  const { bridge } = fakeBridge({ proxyRefresh })
  renderWith(<ConnectionScreen />, servicesFor(bridge, at(proxyState([A, B], B.id, [SUB], { phase: 'on' }), true)))
  const click = (): Promise<void> =>
    act(async () => {
      fireEvent.click(within(subItem()).getByRole('button', { name: 'Обновить' }))
    })
  await click()
  expect(within(subItem()).getByRole('alert').textContent).toBe('Подписку выбранного сервера можно обновить, когда деление выключено')
  proxyRefresh.mockResolvedValueOnce({ kind: 'failed', error: { code: 'busy', message: 'another subscription is being downloaded' } })
  await click()
  expect(within(subItem()).getByRole('alert').textContent).toBe('Уже идёт загрузка подписки')
  proxyRefresh.mockResolvedValueOnce({ kind: 'failed', error: { code: 'fetch_http_status', message: 'HTTP 404' } })
  await click()
  expect(within(subItem()).getByRole('alert').textContent).toBe('Сервер подписки ответил ошибкой. Проверьте адрес')
})

test('«Удалить» of a subscription asks once more in place; «Отмена» and Esc keep it', async () => {
  const { bridge, commands } = fakeBridge()
  const services = servicesFor(bridge, at(proxyState([A, B], A.id, [SUB], { phase: 'off' }), true))
  services.store.getState().showConnection(true)
  renderWith(<ConnectionScreen />, services)
  const ask = (): void => {
    fireEvent.click(within(subItem()).getByRole('button', { name: 'Удалить' }))
  }

  ask()
  expect(within(subItem()).getByText('Удалить подписку и её серверы?')).toBeTruthy()
  expect(within(subItem()).queryByRole('button', { name: 'Обновить' })).toBeNull()
  const cancel = within(subItem()).getByRole('button', { name: 'Отмена' })
  expect(document.activeElement).toBe(cancel)
  fireEvent.click(cancel)
  expect(within(subItem()).queryByText('Удалить подписку и её серверы?')).toBeNull()
  expect(document.activeElement).toBe(within(subItem()).getByRole('button', { name: 'Удалить' }))
  expect(commands).toEqual([])

  // Esc answers the question; it does not leave «Подключение»
  ask()
  fireEvent.keyDown(within(subItem()).getByRole('button', { name: 'Отмена' }), { key: 'Escape' })
  expect(within(subItem()).queryByText('Удалить подписку и её серверы?')).toBeNull()
  expect(services.store.getState().ui.screen).toBe('connection')
  expect(commands).toEqual([])

  ask()
  await act(async () => {
    fireEvent.click(within(subItem()).getByRole('button', { name: 'Удалить' }))
  })
  expect(commands).toEqual([{ method: 'proxy.remove', id: SUB.id }])
})

test('while live routing runs on a server, neither it nor its subscription can be removed', () => {
  const { bridge } = fakeBridge()
  const services = servicesFor(bridge, at(proxyState([A, B], B.id, [SUB], { phase: 'on' }), true))
  renderWith(<ConnectionScreen />, services)
  const server = screen.getByRole('button', { name: 'Удалить edge-b.example.com' }) as HTMLButtonElement
  expect(server.disabled).toBe(true)
  expect(screen.getByText('Выбранный сервер можно удалить, когда деление выключено')).toBeTruthy()
  expect((screen.getByRole('button', { name: 'Удалить edge-a.example.com' }) as HTMLButtonElement).disabled).toBe(false)
  const sub = within(subItem()).getByRole('button', { name: 'Удалить' }) as HTMLButtonElement
  expect(sub.disabled).toBe(true)
  expect(within(subItem()).getByText('Подписку выбранного сервера можно удалить, когда деление выключено')).toBeTruthy()
  // the refresh is left to the helper, which says why it refuses
  expect((within(subItem()).getByRole('button', { name: 'Обновить' }) as HTMLButtonElement).disabled).toBe(false)

  act(() => services.store.getState().setSnapshot(at(proxyState([A, B], B.id, [SUB], { phase: 'off' }), true)))
  expect((screen.getByRole('button', { name: 'Удалить edge-b.example.com' }) as HTMLButtonElement).disabled).toBe(false)
  expect((within(subItem()).getByRole('button', { name: 'Удалить' }) as HTMLButtonElement).disabled).toBe(false)
})

test('Esc in the field of a key does not leave «Подключение» (the text would be lost)', () => {
  const { bridge } = fakeBridge()
  const services = servicesFor(bridge, at(proxyState([], null, [], { phase: 'off' }), true))
  services.store.getState().showConnection(true)
  renderWith(<ConnectionScreen />, services)
  fireEvent.keyDown(screen.getByRole('textbox', { name: 'Ссылки или адрес подписки' }), { key: 'Escape' })
  expect(services.store.getState().ui.screen).toBe('connection')
  fireEvent.keyDown(screen.getByRole('heading', { name: 'Подключение' }), { key: 'Escape' })
  expect(services.store.getState().ui.screen).toBe('board')
})

test('«Подключение» in English', () => {
  setLang('en')
  const { bridge } = fakeBridge()
  renderWith(<ConnectionScreen />, servicesFor(bridge, { ...at(proxyState([A, B], A.id, [SUB], { phase: 'on' }), true), language: 'en' }))
  expect(within(screen.getByRole('radiogroup', { name: 'Connection methods' })).getAllByRole('radio').map((r) => r.textContent)).toEqual([
    'Tailscale',
    'Key or subscription',
    'Configuration file'
  ])
  expect(screen.getByRole('radio', { name: /edge-b\.example\.comTrojan · sub\.example\.com/ })).toBeTruthy()
  expect(screen.getByText('Changing the server briefly restarts split routing')).toBeTruthy()
  expect(screen.getByText('You can remove the selected server when split routing is off')).toBeTruthy()
  expect(within(subItem()).getByText('Servers: 2')).toBeTruthy()
  expect(within(subItem()).getByRole('button', { name: 'Refresh' })).toBeTruthy()
})

test('a removed server hands the focus to the next row, else the legend, else the add field', async () => {
  const C = proxyServer('00000000000000c0', 'home', { protocol: 'wireguard', source: 'file' })
  const { bridge, commands } = fakeBridge()
  const services = servicesFor(bridge, at(proxyState([A, B, C], null, [SUB], { phase: 'off' }), true))
  renderWith(<ConnectionScreen />, services)
  const removeOf = (name: string): HTMLElement => screen.getByRole('button', { name: `Удалить ${name}` })
  const press = async (name: string): Promise<void> => {
    const b = removeOf(name)
    b.focus()
    await act(async () => {
      fireEvent.click(b)
    })
  }
  const show = (servers: (typeof A)[]): void =>
    act(() => services.store.getState().setSnapshot(at(proxyState(servers, null, [SUB], { phase: 'off' }), true)))

  await press('edge-a.example.com')
  expect(commands.at(-1)).toEqual({ method: 'proxy.remove', id: A.id })
  // the button keeps the focus until its row goes
  expect(document.activeElement).toBe(removeOf('edge-a.example.com'))
  show([B, C])
  expect(document.activeElement).toBe(removeOf('edge-b.example.com'))

  await press('home')
  show([B])
  expect(document.activeElement?.tagName).toBe('LEGEND')
  expect(document.activeElement?.textContent).toBe('Серверы')

  await press('edge-b.example.com')
  show([])
  expect(document.activeElement).toBe(screen.getByRole('textbox', { name: 'Ссылки или адрес подписки' }))
})

test('a removed subscription hands the focus to the next one, else the heading, else the add field', async () => {
  const S2: Subscription = { ...SUB, id: 'sub-ffffffffffffffff', name: 'other.example.com' }
  const S3: Subscription = { ...SUB, id: 'sub-eeeeeeeeeeeeeeee', name: 'third.example.com' }
  const { bridge } = fakeBridge()
  const services = servicesFor(bridge, at(proxyState([A], null, [SUB, S2, S3], { phase: 'off' }), true))
  renderWith(<ConnectionScreen />, services)
  const item = (name: string): HTMLElement => screen.getByText(name, { selector: 'strong' }).closest('li') as HTMLElement
  const removeSub = async (name: string): Promise<void> => {
    fireEvent.click(within(item(name)).getByRole('button', { name: 'Удалить' }))
    await act(async () => {
      fireEvent.click(within(item(name)).getByRole('button', { name: 'Удалить' }))
    })
  }
  const show = (subs: Subscription[]): void =>
    act(() => services.store.getState().setSnapshot(at(proxyState([A], null, subs, { phase: 'off' }), true)))

  await removeSub('sub.example.com')
  show([S2, S3])
  expect(document.activeElement).toBe(within(item('other.example.com')).getByRole('button', { name: 'Обновить' }))

  await removeSub('third.example.com')
  show([S2])
  expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Подписки' }))

  await removeSub('other.example.com')
  show([])
  expect(document.activeElement).toBe(screen.getByRole('textbox', { name: 'Ссылки или адрес подписки' }))
})

test('after a refresh the focus goes back to «Обновить»; the kept error is not said twice', async () => {
  let answer: (o: ProxyOutcome) => void = () => {}
  const { bridge } = fakeBridge({ proxyRefresh: () => new Promise<ProxyOutcome>((r) => (answer = r)) })
  const failed = { ...SUB, error: 'fetch_timeout' }
  renderWith(<ConnectionScreen />, servicesFor(bridge, at(proxyState([A, B], A.id, [failed], { phase: 'off' }), true)))
  const refresh = (): HTMLElement => within(subItem()).getByRole('button', { name: /^(Обновить|Обновляю…)$/ })
  const kept = 'Последнее обновление не удалось: Сервер подписки не ответил за 20 секунд'
  expect(within(subItem()).getByText(kept)).toBeTruthy()

  fireEvent.click(refresh())
  expect(document.activeElement).toBe(document.body)
  await act(async () => {
    answer({ kind: 'failed', error: { code: 'fetch_timeout', message: 'timeout' } })
  })
  expect(document.activeElement).toBe(refresh())
  expect(within(subItem()).getByRole('alert').textContent).toBe('Сервер подписки не ответил за 20 секунд')
  expect(within(subItem()).queryByText(kept)).toBeNull()

  // another reason now: both lines, each its own
  fireEvent.click(refresh())
  await act(async () => {
    answer({ kind: 'failed', error: { code: 'fetch_http_status', message: 'HTTP 500' } })
  })
  expect(within(subItem()).getByRole('alert').textContent).toBe('Сервер подписки ответил ошибкой. Проверьте адрес')
  expect(within(subItem()).getByText(kept)).toBeTruthy()
})

test('busy while routing stops on the subscription of the selected server is the general text', async () => {
  const busy: ProxyOutcome = { kind: 'failed', error: { code: 'busy', message: 'busy' } }
  const { bridge } = fakeBridge({ proxyRefresh: async () => busy })
  const S2: Subscription = { ...SUB, id: 'sub-ffffffffffffffff', name: 'other.example.com' }
  renderWith(<ConnectionScreen />, servicesFor(bridge, at(proxyState([A, B], B.id, [SUB, S2], { phase: 'stopping' }), true)))
  const item = (name: string): HTMLElement => screen.getByText(name, { selector: 'strong' }).closest('li') as HTMLElement
  await act(async () => {
    fireEvent.click(within(item('sub.example.com')).getByRole('button', { name: 'Обновить' }))
  })
  expect(within(item('sub.example.com')).getByRole('alert').textContent).toBe(
    'Служебный процесс занят. Повторите через несколько секунд'
  )
  // another subscription: the helper is busy with a download
  await act(async () => {
    fireEvent.click(within(item('other.example.com')).getByRole('button', { name: 'Обновить' }))
  })
  expect(within(item('other.example.com')).getByRole('alert').textContent).toBe('Уже идёт загрузка подписки')
})

test('after the first run «Подключение» stays when the selected server goes, by a removal or a refresh', () => {
  const { bridge } = fakeBridge()
  const services = servicesFor(bridge, at(proxyState([A, B], A.id, [SUB], { phase: 'off' }), true))
  services.store.getState().showConnection(true)
  renderWith(<App />, services)
  const title = (): string | null => screen.getByRole('heading', { level: 1 }).textContent
  expect(title()).toBe('Подключение')
  // the selected server removed: no selection, the list still has a server
  act(() => services.store.getState().setSnapshot(at(proxyState([B], null, [SUB], { phase: 'off' }), true)))
  expect(title()).toBe('Подключение')
  expect(screen.getByRole('group', { name: 'Серверы' })).toBeTruthy()
  // a refresh replaced the servers of the subscription, the selected one with them
  const C = proxyServer('00000000000000c0', 'edge-c.example.com', { source: SUB.id })
  act(() => services.store.getState().setSnapshot(at(proxyState([C], null, [SUB], { phase: 'off' }), true)))
  expect(title()).toBe('Подключение')
  // the board says what is missing
  act(() => services.store.getState().showConnection(false))
  expect(screen.getByRole('button', { name: 'сервер не выбран' })).toBeTruthy()
  // no server at all: the connect screen
  act(() => services.store.getState().setSnapshot(at(proxyState([], null, [], { phase: 'off' }), true)))
  expect(title()).toBe('Подключите VPN')
})

test('an add answered after the connect screen went is told by a notice with the numbers', async () => {
  let answer: (o: ProxyOutcome) => void = () => {}
  const { bridge } = fakeBridge({ proxyAdd: () => new Promise<ProxyOutcome>((r) => (answer = r)) })
  const services = servicesFor(bridge, at(proxyState([], null, [], { phase: 'off' }), true))
  renderWith(<App />, services)
  expect(screen.getByRole('heading', { level: 1 }).textContent).toBe('Подключите VPN')
  fireEvent.change(screen.getByRole('textbox', { name: 'Ссылки или адрес подписки' }), {
    target: { value: 'vless://00000000-0000-4000-8000-000000000001@edge-a.example.com:443#made-up' }
  })
  fireEvent.click(screen.getByRole('button', { name: 'Добавить' }))
  // the state of the add comes before its answer: the server is selected and the board takes over
  act(() => services.store.getState().setSnapshot(at(proxyState([A], A.id, [], { phase: 'off' }), true)))
  expect(screen.getByRole('region', { name: 'Через VPN' })).toBeTruthy()
  await act(async () => {
    answer({ kind: 'done', result: { added: [A.id], existing: 0, rejected: [], rejectedMore: 0 } })
  })
  expect(services.store.getState().notice).toBe('Добавлено серверов: 1')
  expect(screen.getByText('Добавлено серверов: 1')).toBeTruthy()
})
