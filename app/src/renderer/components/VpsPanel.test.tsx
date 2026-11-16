import { act, cleanup, fireEvent, screen, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import type { Snapshot, SplitLaneBridge, VpsEvent, VpsOutcome, VpsRequest } from '../../shared/ipc'
import type { HelperState } from '../../shared/protocol'
import { DEFAULT_SETTINGS, type WindowSettings } from '../../shared/settings'
import { setLang } from '../i18n/t'
import { VPS_ERROR_CODES, vpsErrorText } from '../model/vps'
import type { Services } from '../services'
import { fakeBridge, proxyServer, proxyState, renderWith, servicesFor, snap } from '../testing'
import { VpsPanel } from './VpsPanel'

afterEach(() => {
  cleanup()
  setLang('ru')
  vi.useRealTimers()
})

// Review Focus 1: a password with the characters a shell would read; made up
const SECRET = `p'a"s$ s\\w!рд-made-up`
const PHRASE = 'made-up key phrase'

const off = (over: Partial<HelperState> = {}): Snapshot => ({ ...snap(), state: proxyState([], null, [], { phase: 'off', ...over }) })

/** A bridge whose setup waits for the test, and whose progress the test sends. */
function vpsBridge(over: Partial<SplitLaneBridge> = {}) {
  let emit: (e: VpsEvent) => void = () => {}
  let answer: (o: VpsOutcome) => void = () => {}
  const vpsSetup = vi.fn((_req: VpsRequest) => new Promise<VpsOutcome>((r) => (answer = r)))
  const vpsForgetKey = vi.fn(async () => {})
  const vpsCancel = vi.fn(async () => {})
  const setSettings = vi.fn(async (p: Partial<WindowSettings>) => ({ ...DEFAULT_SETTINGS, ...p }))
  const listeners = { count: 0 }
  const { bridge, commands } = fakeBridge({
    vpsSetup,
    vpsForgetKey,
    vpsCancel,
    setSettings,
    onVpsEvent: (cb) => {
      emit = cb
      listeners.count++
      return () => {
        listeners.count--
        if (emit === cb) emit = () => {}
      }
    },
    ...over
  })
  return {
    bridge,
    commands,
    vpsSetup,
    vpsForgetKey,
    vpsCancel,
    setSettings,
    listeners,
    emit: (...events: VpsEvent[]) =>
      act(() => {
        for (const e of events) emit(e)
      }),
    answer: (o: VpsOutcome) =>
      act(async () => {
        answer(o)
      })
  }
}

const address = (): HTMLInputElement => screen.getByRole('textbox', { name: 'Адрес сервера' }) as HTMLInputElement
const login = (): HTMLInputElement => screen.getByRole('textbox', { name: 'Логин' }) as HTMLInputElement
const password = (): HTMLInputElement => screen.getByLabelText('Пароль', { selector: 'input[type="password"]' }) as HTMLInputElement
const passphrase = (): HTMLInputElement =>
  screen.getByLabelText('Пароль ключа (если есть)', { selector: 'input[type="password"]' }) as HTMLInputElement
const setUp = (): HTMLButtonElement => screen.getByRole('button', { name: 'Настроить' }) as HTMLButtonElement
const type = (el: HTMLInputElement, value: string): void => {
  fireEvent.change(el, { target: { value } })
}
const items = (): (string | null)[] =>
  within(screen.getByRole('list', { name: 'Ход настройки' }))
    .getAllByRole('listitem')
    .map((li) => li.textContent)

/** The form filled for a password login, and «Настроить». */
function start(addr = '203.0.113.10'): void {
  type(address(), addr)
  type(password(), SECRET)
  fireEvent.click(setUp())
}

const DONE: VpsOutcome = { kind: 'done', servers: 2, subscriptionId: 'sub-0123456789abcdef', warnings: [] }

test('the form: the hint of the hosting, the fields, root as the login, the password way in', () => {
  const { bridge } = vpsBridge()
  renderWith(<VpsPanel />, servicesFor(bridge, off()))
  expect(screen.getByText('Адрес, логин и пароль хостинг присылает в письме после аренды. Нужна Debian 12+ или Ubuntu 22.04+')).toBeTruthy()
  expect(address().value).toBe('')
  expect(screen.getByText(/^IP-адрес или имя\. Если SSH не на порту 22, добавьте :порт/)).toBeTruthy()
  expect(login().value).toBe('root')
  const way = screen.getByRole('radiogroup', { name: 'Способ входа' })
  expect(within(way).getAllByRole('radio').map((r) => r.closest('label')?.textContent)).toEqual(['Пароль', 'Файл ключа'])
  expect((within(way).getByRole('radio', { name: 'Пароль' }) as HTMLInputElement).checked).toBe(true)
  expect(password().value).toBe('')
  expect(screen.queryByRole('button', { name: 'Выбрать файл ключа…' })).toBeNull()
  expect(screen.getByText('Пароль и ключ нужны только для этой настройки: SplitLane их не сохраняет')).toBeTruthy()
  expect(setUp().disabled).toBe(false)
  // no progress before the setup
  expect(screen.queryByRole('list', { name: 'Ход настройки' })).toBeNull()
})

test('the checks of the form show under their fields, and nothing is sent until they pass', () => {
  const { bridge, vpsSetup } = vpsBridge()
  renderWith(<VpsPanel />, servicesFor(bridge, off()))
  // nothing is said before the first «Настроить»
  type(address(), 'http://203.0.113.10')
  expect(screen.queryByText(/Это не похоже на адрес сервера/)).toBeNull()

  type(login(), 'Root')
  fireEvent.click(setUp())
  expect(vpsSetup).not.toHaveBeenCalled()
  const addressError = screen.getByText('Это не похоже на адрес сервера. Пример: 203.0.113.10, vps.example.com или vps.example.com:2222')
  expect(address().getAttribute('aria-invalid')).toBe('true')
  expect(address().getAttribute('aria-describedby')).toContain(addressError.id)
  const userError = screen.getByText('Логин — до 32 строчных латинских букв, цифр, «_» и «-», в начале — буква или «_»')
  expect(login().getAttribute('aria-describedby')).toContain(userError.id)
  expect(screen.getByText('Введите пароль')).toBeTruthy()
  // the first field to fix has the focus
  expect(document.activeElement).toBe(address())
  // each field sits above its error
  expect(address().compareDocumentPosition(addressError) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(addressError.compareDocumentPosition(login()) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

  // after that the errors follow the typing
  type(address(), '')
  expect(screen.getByText('Введите адрес сервера')).toBeTruthy()
  type(address(), ' [2001:db8::7]:22 ')
  expect(screen.queryByText('Введите адрес сервера')).toBeNull()
  expect(address().getAttribute('aria-invalid')).toBe('false')
  type(login(), ' ubuntu ')
  type(password(), 'x'.repeat(1025))
  expect(screen.getByText('Пароль длиннее 1024 знаков')).toBeTruthy()
  // a long password is a password error: never the one of the address and the login
  expect(screen.queryByText(/Проверьте адрес сервера и логин/)).toBeNull()
  fireEvent.click(setUp())
  expect(vpsSetup).not.toHaveBeenCalled()
  expect(document.activeElement).toBe(password())

  type(password(), SECRET)
  fireEvent.click(setUp())
  expect(vpsSetup).toHaveBeenCalledWith({ address: '[2001:db8::7]:22', user: 'ubuntu', password: SECRET, useKey: false })
})

test('«Настроить» waits for split routing to be off: off and error only', () => {
  for (const phase of ['on', 'degraded', 'blocked', 'starting', 'stopping'] as const) {
    const { bridge, vpsSetup } = vpsBridge()
    renderWith(<VpsPanel />, servicesFor(bridge, off({ phase })))
    expect(setUp().disabled, phase).toBe(true)
    expect(setUp().title, phase).toBe('Выключите деление на время настройки')
    expect(screen.getByText('Выключите деление на время настройки'), phase).toBeTruthy()
    type(address(), '203.0.113.10')
    type(password(), SECRET)
    fireEvent.submit(address().closest('form')!)
    expect(vpsSetup, phase).not.toHaveBeenCalled()
    cleanup()
  }
  for (const phase of ['off', 'error'] as const) {
    const { bridge } = vpsBridge()
    renderWith(<VpsPanel />, servicesFor(bridge, off({ phase })))
    expect(setUp().disabled, phase).toBe(false)
    expect(screen.queryByText('Выключите деление на время настройки'), phase).toBeNull()
    cleanup()
  }
  setLang('en')
  const { bridge } = vpsBridge()
  renderWith(<VpsPanel />, servicesFor(bridge, { ...off({ phase: 'on' }), language: 'en' }))
  expect((screen.getByRole('button', { name: 'Set up' }) as HTMLButtonElement).disabled).toBe(true)
  expect(screen.getByText('Turn split routing off while setting up')).toBeTruthy()
})

test('the progress: six steps, each waiting, in progress, done, skipped or not worked, and the time', async () => {
  vi.useFakeTimers()
  const v = vpsBridge()
  const services = servicesFor(v.bridge, off())
  renderWith(<VpsPanel />, services)
  start()
  // the form is gone, the cancel has the focus
  expect(screen.queryByRole('textbox', { name: 'Адрес сервера' })).toBeNull()
  expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Отменить' }))
  expect(items()).toEqual([
    'Подключение к серверуждёт',
    'Проверка системыждёт',
    'Установка VPN-сервераждёт',
    'Защита сервераждёт',
    'Подписка для телефонаждёт',
    'Добавление в SplitLaneждёт'
  ])
  expect(screen.getByText('Прошло: 0:00')).toBeTruthy()

  await v.emit({ step: 'connect', state: 'start' })
  expect(items()[0]).toBe('Подключение к серверуидёт')
  await v.emit({ step: 'connect', state: 'done' }, { step: 'system', state: 'start' }, { step: 'system', state: 'done' })
  await v.emit({ step: 'install', state: 'start', detail: 'sing-box' })
  await act(async () => {
    vi.advanceTimersByTime(65_000)
  })
  expect(screen.getByText('Прошло: 1:05')).toBeTruthy()
  expect(items().slice(0, 3)).toEqual(['Подключение к серверуготово', 'Проверка системыготово', 'Установка VPN-сервераидёт'])

  await v.emit(
    { step: 'install', state: 'done' },
    { step: 'harden', state: 'start' },
    { step: 'harden', state: 'warn', detail: 'hardening failed: apt-get' },
    { step: 'subscription', state: 'start' },
    { step: 'subscription', state: 'skip' },
    { step: 'add', state: 'start' },
    { step: 'add', state: 'done' }
  )
  await act(async () => {
    vi.advanceTimersByTime(3_000)
  })
  await v.answer({ kind: 'done', servers: 2, subscriptionId: null, warnings: ['harden_failed', 'sub_skipped'] })
  // no subscription, no button
  expect(screen.queryByRole('button', { name: 'Подключить телефон' })).toBeNull()
  expect(items()).toEqual([
    'Подключение к серверуготово',
    'Проверка системыготово',
    'Установка VPN-сервераготово',
    'Защита серверане удалось',
    'Подписка для телефонапропущено',
    'Добавление в SplitLaneготово'
  ])
  // the time stops with the end
  expect(screen.getByText('Прошло: 1:08')).toBeTruthy()
  await act(async () => {
    vi.advanceTimersByTime(10_000)
  })
  expect(screen.getByText('Прошло: 1:08')).toBeTruthy()
  // the subscription of the setup only listens while it runs
  expect(v.listeners.count).toBe(0)
})

test('the result with a subscription: the number of servers, no warning, and the list in «Подключение»', async () => {
  const A = proxyServer('0123456789abcdef', 'SplitLane-VLESS', { source: 'sub-0123456789abcdef' })
  const B = proxyServer('fedcba9876543210', 'SplitLane-Hysteria2', { protocol: 'hysteria2', source: 'sub-0123456789abcdef' })
  const v = vpsBridge()
  const services = servicesFor(v.bridge, off())
  renderWith(<VpsPanel showList />, services)
  start()
  expect(screen.queryByRole('group', { name: 'Серверы' })).toBeNull()
  await v.emit({ step: 'add', state: 'start' })
  const sub = { id: 'sub-0123456789abcdef', name: '203-0-113-10.sslip.io', updatedAt: '2026-11-16T10:00:00Z', count: 2, error: null }
  act(() => services.store.getState().setSnapshot({ ...snap(), state: proxyState([A, B], A.id, [sub], { phase: 'off' }) }))
  await v.answer(DONE)
  expect(screen.getByRole('button', { name: 'Подключить телефон' })).toBeTruthy()
  const status = screen.getByRole('status')
  expect(within(status).getByText('Готово. Добавлено серверов: 2')).toBeTruthy()
  expect(within(status).queryByText(/Телефон подключить не получилось/)).toBeNull()
  expect(within(status).queryByText(/Защиту сервера/)).toBeNull()
  expect(document.activeElement).toBe(status)
  // the new servers, the first one chosen
  const list = screen.getByRole('group', { name: 'Серверы' })
  expect((within(list).getByRole('radio', { name: /SplitLane-VLESS/ }) as HTMLInputElement).checked).toBe(true)
  expect(within(list).getByRole('radio', { name: /SplitLane-Hysteria2/ })).toBeTruthy()
  // nothing to retry or cancel
  expect(screen.queryByRole('button', { name: 'Попробовать снова' })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Отменить' })).toBeNull()
})

test('the result without a subscription says why the phone cannot connect; the wizard shows no list', async () => {
  for (const [warning, part] of [
    ['sub_unreachable', 'подписка на сервере не ответила вовремя'],
    ['sub_port80_busy', 'порт 80 на сервере занят другой программой'],
    ['sub_skipped', 'не удалось поставить или запустить Caddy для подписки']
  ] as const) {
    const v = vpsBridge()
    const listed = proxyState([proxyServer('0123456789abcdef', 'a')], null, [], { phase: 'off' })
    renderWith(<VpsPanel />, servicesFor(v.bridge, { ...snap(), state: listed }))
    start()
    await v.answer({ kind: 'done', servers: 1, subscriptionId: null, warnings: [warning, 'harden_failed'] })
    const status = screen.getByRole('status')
    expect(within(status).getByText('Готово. Добавлено серверов: 1')).toBeTruthy()
    expect(within(status).getByText(/^Телефон подключить не получилось: /).textContent, warning).toContain(part)
    expect(within(status).getByText('Защиту сервера поставить не удалось. VPN работает и без неё')).toBeTruthy()
    expect(screen.queryByRole('group', { name: 'Серверы' })).toBeNull()
    cleanup()
  }
})

test('a key: the file is chosen in the main process, its name shows, its errors show under the button', async () => {
  const vpsPickKey = vi
    .fn<SplitLaneBridge['vpsPickKey']>()
    .mockResolvedValueOnce({ error: 'too_large' })
    .mockResolvedValueOnce({ error: 'not_text' })
    .mockResolvedValueOnce(null)
    .mockResolvedValueOnce({ name: 'id_ed25519' })
  const v = vpsBridge({ vpsPickKey })
  renderWith(<VpsPanel />, servicesFor(v.bridge, off()))
  type(address(), 'vps.example.com:2222')
  fireEvent.click(screen.getByRole('radio', { name: 'Файл ключа' }))
  expect(screen.queryByLabelText('Пароль', { selector: 'input[type="password"]' })).toBeNull()
  fireEvent.click(setUp())
  expect(v.vpsSetup).not.toHaveBeenCalled()
  expect(screen.getByText('Выберите файл ключа')).toBeTruthy()

  const pick = (): HTMLElement => screen.getByRole('button', { name: /^Выбрать (файл ключа|другой файл)…$/ })
  await act(async () => {
    fireEvent.click(pick())
  })
  expect(screen.getByText('Файл больше 16 КБ — это не ключ SSH')).toBeTruthy()
  await act(async () => {
    fireEvent.click(pick())
  })
  expect(screen.getByText('Это не текстовый файл ключа OpenSSH')).toBeTruthy()
  // a closed dialog changes nothing
  await act(async () => {
    fireEvent.click(pick())
  })
  expect(screen.getByText('Это не текстовый файл ключа OpenSSH')).toBeTruthy()
  await act(async () => {
    fireEvent.click(pick())
  })
  expect(screen.queryByText('Это не текстовый файл ключа OpenSSH')).toBeNull()
  expect(screen.queryByText('Выберите файл ключа')).toBeNull()
  expect(screen.getByText('Выбран файл id_ed25519')).toBeTruthy()
  expect(pick().textContent).toBe('Выбрать другой файл…')
  expect(vpsPickKey).toHaveBeenCalledTimes(4)

  type(passphrase(), PHRASE)
  fireEvent.click(setUp())
  expect(v.vpsSetup).toHaveBeenCalledWith({ address: 'vps.example.com:2222', user: 'root', useKey: true, keyPassphrase: PHRASE })
})

test('the secrets stay in the form: never in the store, a notice or the settings', async () => {
  const v = vpsBridge({ vpsPickKey: async () => ({ name: 'id_ed25519' }) })
  const services = servicesFor(v.bridge, off())
  renderWith(<VpsPanel />, services)
  const leaks = (): void => {
    const all = JSON.stringify(services.store.getState())
    expect(all).not.toContain('made-up')
    expect(all).not.toContain(SECRET)
  }
  type(address(), '203.0.113.10')
  type(password(), SECRET)
  fireEvent.click(screen.getByRole('radio', { name: 'Файл ключа' }))
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Выбрать файл ключа…' }))
  })
  type(passphrase(), PHRASE)
  fireEvent.click(screen.getByRole('radio', { name: 'Пароль' }))
  fireEvent.click(setUp())
  leaks()
  await v.emit({ step: 'connect', state: 'start' }, { step: 'connect', state: 'failed', detail: 'ssh_auth' })
  leaks()
  await v.answer({ kind: 'failed', code: 'ssh_auth' })
  leaks()
  expect(v.setSettings).not.toHaveBeenCalled()
  // the password way in sends no passphrase, even one typed for the key
  expect(v.vpsSetup).toHaveBeenCalledWith({ address: '203.0.113.10', user: 'root', password: SECRET, useKey: false })
})

test('«Отменить»: the setup is cancelled, the step it stopped on says so', async () => {
  const v = vpsBridge()
  renderWith(<VpsPanel />, servicesFor(v.bridge, off()))
  start()
  await v.emit({ step: 'connect', state: 'start' }, { step: 'connect', state: 'done' }, { step: 'system', state: 'start' })
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Отменить' }))
  })
  expect(v.vpsCancel).toHaveBeenCalledTimes(1)
  const cancelling = screen.getByRole('button', { name: 'Отменяю…' }) as HTMLButtonElement
  expect(cancelling.disabled).toBe(true)
  await v.answer({ kind: 'cancelled' })
  expect(items().slice(0, 3)).toEqual(['Подключение к серверуготово', 'Проверка системыотменено', 'Установка VPN-сервера'])
  expect(screen.getByRole('status').textContent).toBe('Настройка отменена')
  expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Попробовать снова' }))
})

test('each code of a failure shows its own text, in Russian and in English', async () => {
  for (const lang of ['ru', 'en'] as const) {
    for (const code of VPS_ERROR_CODES) {
      if (code === 'cancelled') continue // the main process ends a cancel as `cancelled`, not as a failure
      setLang(lang)
      const v = vpsBridge()
      renderWith(<VpsPanel />, servicesFor(v.bridge, { ...off(), language: lang }))
      type(screen.getByRole('textbox', { name: lang === 'ru' ? 'Адрес сервера' : 'Server address' }) as HTMLInputElement, '203.0.113.10')
      type(screen.getByLabelText(lang === 'ru' ? 'Пароль' : 'Password', { selector: 'input[type="password"]' }) as HTMLInputElement, SECRET)
      fireEvent.click(screen.getByRole('button', { name: lang === 'ru' ? 'Настроить' : 'Set up' }))
      await v.answer({ kind: 'failed', code })
      expect(screen.getByRole('alert').textContent, `${lang} ${code}`).toBe(vpsErrorText(code, undefined, false))
      expect(screen.getByRole('button', { name: lang === 'ru' ? 'Попробовать снова' : 'Try again' }), code).toBeTruthy()
      cleanup()
    }
  }
  setLang('en')
  expect(vpsErrorText('ssh_auth', undefined, false)).toBe('Wrong username or password')
})

test('a failure marks its step; «Попробовать снова» brings the form back with every field, the focus where to fix', async () => {
  const v = vpsBridge({ vpsPickKey: async () => ({ name: 'id_ed25519' }) })
  const services = servicesFor(v.bridge, off())
  renderWith(<VpsPanel />, services)
  type(address(), 'vps.example.com:2222')
  type(login(), 'deploy')
  type(password(), SECRET)
  fireEvent.click(setUp())
  await v.emit({ step: 'connect', state: 'start' }, { step: 'connect', state: 'failed', detail: 'ssh_auth' })
  await v.answer({ kind: 'failed', code: 'ssh_auth' })
  expect(items()[0]).toBe('Подключение к серверуошибка')
  // the steps that never started say nothing once it is over
  expect(items()[1]).toBe('Проверка системы')
  expect(screen.getByRole('alert').textContent).toBe('Неверный логин или пароль')
  fireEvent.click(screen.getByRole('button', { name: 'Попробовать снова' }))
  expect(address().value).toBe('vps.example.com:2222')
  expect(login().value).toBe('deploy')
  expect(password().value).toBe(SECRET)
  expect(document.activeElement).toBe(password())
  expect(screen.queryByRole('list', { name: 'Ход настройки' })).toBeNull()
  expect(services.store.getState().vps).toBeNull()
  // the key way in keeps its file too, as the main process keeps the key (ruling 18)
  fireEvent.click(screen.getByRole('radio', { name: 'Файл ключа' }))
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Выбрать файл ключа…' }))
  })
  fireEvent.click(setUp())
  await v.answer({ kind: 'failed', code: 'ssh_key', detail: 'passphrase' })
  expect(screen.getByRole('alert').textContent).toBe('Этот файл ключа не подходит: пароль ключа не введён или неверен')
  fireEvent.click(screen.getByRole('button', { name: 'Попробовать снова' }))
  expect(screen.getByText('Выбран файл id_ed25519')).toBeTruthy()
  expect(document.activeElement).toBe(passphrase())
  expect(v.vpsForgetKey).not.toHaveBeenCalled()
  fireEvent.click(setUp())
  expect(v.vpsSetup).toHaveBeenLastCalledWith({ address: 'vps.example.com:2222', user: 'deploy', useKey: true })
})

test('a failure before any step shows no progress', async () => {
  const v = vpsBridge()
  renderWith(<VpsPanel />, servicesFor(v.bridge, off()))
  start()
  await v.answer({ kind: 'failed', code: 'routing_on' })
  expect(screen.queryByRole('list', { name: 'Ход настройки' })).toBeNull()
  expect(screen.getByRole('alert').textContent).toBe('Выключите деление на время настройки')
})

test('leaving the screen forgets the key in the main process, and the password with the form', async () => {
  const v = vpsBridge({ vpsPickKey: async () => ({ name: 'id_ed25519' }) })
  const services = servicesFor(v.bridge, off())
  const view = renderWith(<VpsPanel />, services)
  type(address(), '203.0.113.10')
  type(password(), SECRET)
  fireEvent.click(screen.getByRole('radio', { name: 'Файл ключа' }))
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Выбрать файл ключа…' }))
  })
  expect(v.vpsForgetKey).not.toHaveBeenCalled()
  view.unmount()
  expect(v.vpsForgetKey).toHaveBeenCalledTimes(1)
  renderWith(<VpsPanel />, services)
  expect(address().value).toBe('')
  expect(password().value).toBe('')
  fireEvent.click(screen.getByRole('radio', { name: 'Файл ключа' }))
  expect(screen.queryByText('Выбран файл id_ed25519')).toBeNull()
})

test('a setup goes on when its screen is left: back on the tile it shows, and an end no screen shows is a notice', async () => {
  const v = vpsBridge()
  const services: Services = servicesFor(v.bridge, off())
  const first = renderWith(<VpsPanel />, services)
  start()
  await v.emit({ step: 'connect', state: 'start' })
  first.unmount()
  expect(v.vpsForgetKey).toHaveBeenCalledTimes(1)
  expect(v.vpsCancel).not.toHaveBeenCalled()
  await v.emit({ step: 'connect', state: 'done' }, { step: 'system', state: 'start' })

  const second = renderWith(<VpsPanel />, services)
  expect(items().slice(0, 2)).toEqual(['Подключение к серверуготово', 'Проверка системыидёт'])
  expect(screen.getByRole('button', { name: 'Отменить' })).toBeTruthy()
  second.unmount()

  await v.answer(DONE)
  expect(services.store.getState().notice).toBe('Готово. Добавлено серверов: 2')
  expect(services.store.getState().vps).toBeNull()
  renderWith(<VpsPanel />, services)
  expect(address().value).toBe('')
})

test('an end shown on screen is dropped when the screen is left', async () => {
  const v = vpsBridge()
  const services = servicesFor(v.bridge, off())
  const view = renderWith(<VpsPanel />, services)
  start()
  await v.answer({ kind: 'failed', code: 'timeout' })
  expect(services.store.getState().vps).not.toBeNull()
  view.unmount()
  expect(services.store.getState().vps).toBeNull()
  expect(services.store.getState().notice).toBeNull()
})

test('a thrown bridge call ends the setup as a failed installation', async () => {
  const v = vpsBridge({ vpsSetup: async () => Promise.reject(new Error('link closed')) })
  renderWith(<VpsPanel />, servicesFor(v.bridge, off()))
  await act(async () => {
    start()
  })
  expect(screen.getByRole('alert').textContent).toBe('Установка не удалась')
})

test('the form in English', () => {
  setLang('en')
  const { bridge } = vpsBridge()
  renderWith(<VpsPanel />, servicesFor(bridge, { ...off(), language: 'en' }))
  const hint = 'Your hosting provider emails the address, username and password after you rent the server. It needs Debian 12+ or Ubuntu 22.04+'
  expect(screen.getByText(hint)).toBeTruthy()
  expect(screen.getByRole('textbox', { name: 'Server address' })).toBeTruthy()
  expect((screen.getByRole('textbox', { name: 'Username' }) as HTMLInputElement).value).toBe('root')
  expect(screen.getByRole('radiogroup', { name: 'How to sign in' })).toBeTruthy()
  expect(screen.getByRole('radio', { name: 'Key file' })).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: 'Set up' }))
  expect(screen.getByText('Enter the server address')).toBeTruthy()
  expect(screen.getByText('Enter the password')).toBeTruthy()
})

/** The window goes to the tray (or is minimized): the page is hidden. */
function setHidden(hidden: boolean): void {
  Object.defineProperty(document, 'hidden', { configurable: true, get: () => hidden })
  act(() => {
    document.dispatchEvent(new Event('visibilitychange'))
  })
}

test('a hidden window drops the password, the passphrase and the key, as leaving the screen does (ruling 21)', async () => {
  const v = vpsBridge({ vpsPickKey: async () => ({ name: 'id_ed25519' }) })
  renderWith(<VpsPanel />, servicesFor(v.bridge, off()))
  try {
    type(address(), '203.0.113.10')
    type(login(), 'deploy')
    type(password(), SECRET)
    fireEvent.click(screen.getByRole('radio', { name: 'Файл ключа' }))
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Выбрать файл ключа…' }))
    })
    type(passphrase(), PHRASE)
    // shown again without a hide in between: nothing goes
    setHidden(false)
    expect(v.vpsForgetKey).not.toHaveBeenCalled()
    expect(screen.getByText('Выбран файл id_ed25519')).toBeTruthy()

    setHidden(true)
    expect(v.vpsForgetKey).toHaveBeenCalledTimes(1)
    setHidden(false)
    expect(screen.queryByText('Выбран файл id_ed25519')).toBeNull()
    expect(screen.getByRole('button', { name: 'Выбрать файл ключа…' })).toBeTruthy()
    expect(passphrase().value).toBe('')
    fireEvent.click(screen.getByRole('radio', { name: 'Пароль' }))
    expect(password().value).toBe('')
    // what is no secret stays
    expect(address().value).toBe('203.0.113.10')
    expect(login().value).toBe('deploy')
  } finally {
    setHidden(false)
  }
})

test('a setup goes on while the window is hidden; its progress and result are there when it is back', async () => {
  const v = vpsBridge()
  renderWith(<VpsPanel />, servicesFor(v.bridge, off()))
  try {
    start()
    await v.emit({ step: 'connect', state: 'start' })
    setHidden(true)
    expect(v.vpsForgetKey).toHaveBeenCalledTimes(1)
    expect(v.vpsCancel).not.toHaveBeenCalled()
    await v.emit({ step: 'connect', state: 'done' }, { step: 'system', state: 'start' })
    setHidden(false)
    expect(items().slice(0, 2)).toEqual(['Подключение к серверуготово', 'Проверка системыидёт'])
    await v.answer({ kind: 'failed', code: 'os_unsupported', detail: 'debian 11' })
    expect(screen.getByRole('alert').textContent).toMatch(/^Система сервера не подходит/)
    expect(screen.getByRole('alert').textContent).toContain('Подробности: debian 11')
    // the retry brings the form back without the password: it went with the hide
    fireEvent.click(screen.getByRole('button', { name: 'Попробовать снова' }))
    expect(address().value).toBe('203.0.113.10')
    expect(password().value).toBe('')
  } finally {
    setHidden(false)
  }
})

test('a key picked after the panel went is forgotten at once (ruling 22)', async () => {
  let answer: (r: { name: string }) => void = () => {}
  const v = vpsBridge({ vpsPickKey: () => new Promise((r) => (answer = r)) })
  const view = renderWith(<VpsPanel />, servicesFor(v.bridge, off()))
  fireEvent.click(screen.getByRole('radio', { name: 'Файл ключа' }))
  fireEvent.click(screen.getByRole('button', { name: 'Выбрать файл ключа…' }))
  view.unmount()
  expect(v.vpsForgetKey).toHaveBeenCalledTimes(1)
  await act(async () => {
    answer({ name: 'id_ed25519' })
  })
  expect(v.vpsForgetKey).toHaveBeenCalledTimes(2)
})

test('a polite live line says the step running or failed, not the time', async () => {
  vi.useFakeTimers()
  const v = vpsBridge()
  renderWith(<VpsPanel />, servicesFor(v.bridge, off()))
  start()
  const live = (): Element => document.querySelector('[aria-live="polite"]')!
  expect(live().textContent).toBe('')
  await v.emit({ step: 'connect', state: 'start' })
  expect(live().textContent).toBe('Идёт: Подключение к серверу')
  await act(async () => {
    vi.advanceTimersByTime(5_000)
  })
  expect(live().textContent).toBe('Идёт: Подключение к серверу')
  await v.emit({ step: 'connect', state: 'done' }, { step: 'system', state: 'start' })
  expect(live().textContent).toBe('Идёт: Проверка системы')
  await v.emit({ step: 'system', state: 'failed', detail: 'os_unsupported' })
  expect(live().textContent).toBe('Ошибка: Проверка системы')
  // it is not shown: the list says the same to the eye
  expect(live().className).toMatch(/live/)
})
