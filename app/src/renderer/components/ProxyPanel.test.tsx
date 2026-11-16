import { act, cleanup, fireEvent, screen, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import type { ProxyOutcome, Snapshot } from '../../shared/ipc'
import type { ProxyAddResult } from '../../shared/protocol'
import { DEFAULT_SETTINGS } from '../../shared/settings'
import { setLang } from '../i18n/t'
import { fakeBridge, proxyServer, proxyState, renderWith, servicesFor, snap } from '../testing'
import { ProxyPanel } from './ProxyPanel'

afterEach(() => {
  cleanup()
  setLang('ru')
})

const A = proxyServer('0123456789abcdef', 'edge-a.example.com')
const B = proxyServer('fedcba9876543210', 'edge-b.example.com', { protocol: 'shadowsocks' })
// made-up links: nothing of them is a real server or a real key
const TEXT = [
  'vless://00000000-0000-4000-8000-000000000001@edge-a.example.com:443?security=reality&pbk=made-up-key#edge-a',
  'ss://bWFkZS11cDptYWRlLXVw@192.0.2.10:8388#edge-b',
  'vless://00000000-0000-4000-8000-000000000002@198.51.100.3:443?type=xhttp#xhttp'
].join('\n')

const withState = (s = proxyState([], null, [], { phase: 'off' })): Snapshot => ({ ...snap(), state: s })

const result = (over: Partial<ProxyAddResult> = {}): ProxyOutcome => ({
  kind: 'done',
  result: { added: [], existing: 0, servers: [], subscription: null, rejected: [], rejectedMore: 0, ...over }
})

/** A bridge call that waits for the test to answer it. */
function deferred(): { call: () => Promise<ProxyOutcome>; answer(o: ProxyOutcome): Promise<void> } {
  let resolve: (o: ProxyOutcome) => void = () => {}
  return {
    call: () => new Promise<ProxyOutcome>((r) => (resolve = r)),
    answer: (o) =>
      act(async () => {
        resolve(o)
      })
  }
}

const field = (): HTMLTextAreaElement => screen.getByRole('textbox', { name: 'Ссылки или адрес подписки' }) as HTMLTextAreaElement

test('the key field: Add waits for a text, says it is adding, then shows the summary and clears the field', async () => {
  const d = deferred()
  const proxyAdd = vi.fn((_text: string) => d.call())
  const setSettings = vi.fn(async () => DEFAULT_SETTINGS)
  const { bridge, commands } = fakeBridge({ proxyAdd, setSettings })
  const services = servicesFor(bridge, withState())
  renderWith(<ProxyPanel mode="key" />, services)

  expect(screen.getByText('Ссылки по одной в строке и текст файла WireGuard, или один адрес подписки https://')).toBeTruthy()
  const add = (): HTMLButtonElement => screen.getByRole('button', { name: /^(Добавить|Добавляю…)$/ }) as HTMLButtonElement
  expect(add().disabled).toBe(true)
  fireEvent.change(field(), { target: { value: '  \n ' } })
  expect(add().disabled).toBe(true)
  fireEvent.change(field(), { target: { value: TEXT } })
  expect(add().disabled).toBe(false)

  fireEvent.click(add())
  expect(proxyAdd).toHaveBeenCalledWith(TEXT)
  // a subscription download can take a while: the button says so and cannot be pressed twice
  expect(add().textContent).toBe('Добавляю…')
  expect(add().disabled).toBe(true)
  expect(field().readOnly).toBe(true)
  // the secret text is only in the component: neither in the store nor in a notice
  expect(JSON.stringify(services.store.getState())).not.toContain('made-up-key')

  await d.answer(
    result({
      added: [A.id, B.id],
      existing: 1,
      rejected: [
        { line: 3, code: 'unsupported_transport' },
        { line: 0, code: 'too_many' }
      ],
      rejectedMore: 4
    })
  )
  const status = screen.getByRole('status')
  expect([...status.querySelectorAll('p, li')].map((e) => e.textContent)).toEqual([
    'Добавлено серверов: 2',
    'Уже были в списке: 1',
    'Строка 3: Этот транспорт SplitLane пока не поддерживает',
    'Слишком много серверов или подписок: можно до 500 серверов и 20 подписок',
    'И ещё отклонено строк: 4'
  ])
  expect(within(status).getAllByRole('listitem')).toHaveLength(2)
  expect(field().value).toBe('')
  expect(field().readOnly).toBe(false)
  expect(add().textContent).toBe('Добавить')
  expect(services.store.getState().notice).toBeNull()
  // the text went only to proxyAdd: no setting, no command and no notice ever carried any of it
  expect(proxyAdd).toHaveBeenCalledTimes(1)
  expect(JSON.stringify(setSettings.mock.calls)).not.toContain('made-up')
  expect(JSON.stringify(commands)).not.toContain('made-up')
  expect(JSON.stringify(services.store.getState())).not.toContain('made-up')
})

test('an answer that comes after the panel went is a notice with the numbers only', async () => {
  const d = deferred()
  const { bridge } = fakeBridge({ proxyAdd: () => d.call() })
  const services = servicesFor(bridge, withState())
  const view = renderWith(<ProxyPanel mode="key" />, services)
  fireEvent.change(field(), { target: { value: TEXT } })
  fireEvent.click(screen.getByRole('button', { name: 'Добавить' }))
  view.unmount()
  await d.answer(
    result({ added: [A.id, B.id], existing: 1, rejected: [{ line: 3, code: 'unsupported_transport' }], rejectedMore: 2 })
  )
  const notice = services.store.getState().notice
  expect(notice).toBe('Добавлено серверов: 2 · Отклонено строк: 3')
  expect(notice).not.toMatch(/made-up|Строка|транспорт/)

  // a failure: its text
  const failed = deferred()
  const again = servicesFor(fakeBridge({ proxyAddFile: () => failed.call() }).bridge, withState())
  const fileView = renderWith(<ProxyPanel mode="file" />, again)
  fireEvent.click(screen.getByRole('button', { name: 'Выбрать файл .conf…' }))
  fileView.unmount()
  await failed.answer({ kind: 'failed', error: { code: 'internal', message: 'file unreadable' } })
  expect(again.store.getState().notice).toBe('Нет связи со служебным процессом или внутренняя ошибка')

  // a closed dialog: nothing
  const closed = deferred()
  const third = servicesFor(fakeBridge({ proxyAddFile: () => closed.call() }).bridge, withState())
  const thirdView = renderWith(<ProxyPanel mode="file" />, third)
  fireEvent.click(screen.getByRole('button', { name: 'Выбрать файл .conf…' }))
  thirdView.unmount()
  await closed.answer({ kind: 'cancelled' })
  expect(third.store.getState().notice).toBeNull()
})

test('focus lost while it ran goes back to the field or the file button; focus moved elsewhere stays', async () => {
  const d = deferred()
  const { bridge } = fakeBridge({ proxyAdd: () => d.call(), proxyAddFile: () => d.call() })
  const services = servicesFor(bridge, withState())
  const view = renderWith(<ProxyPanel mode="key" />, services)
  fireEvent.change(field(), { target: { value: 'not a link' } })
  fireEvent.click(screen.getByRole('button', { name: 'Добавить' }))
  // the button is disabled while it runs, and the browser drops its focus: the page has it
  expect(document.activeElement).toBe(document.body)
  await d.answer(result({ rejected: [{ line: 1, code: 'bad_link' }] }))
  expect(document.activeElement).toBe(field())

  // the user went on to something else meanwhile: it keeps the focus
  fireEvent.click(screen.getByRole('button', { name: 'Добавить' }))
  const elsewhere = document.createElement('button')
  document.body.append(elsewhere)
  elsewhere.focus()
  await d.answer(result({ rejected: [{ line: 1, code: 'bad_link' }] }))
  expect(document.activeElement).toBe(elsewhere)
  elsewhere.remove()
  view.unmount()

  renderWith(<ProxyPanel mode="file" />, services)
  const choose = screen.getByRole('button', { name: 'Выбрать файл .conf…' })
  fireEvent.click(choose)
  expect(document.activeElement).toBe(document.body)
  await d.answer({ kind: 'cancelled' })
  expect(document.activeElement).toBe(choose)
})

test('busy for links or a file is the general text; for a subscription address, another download', async () => {
  const busy: ProxyOutcome = { kind: 'failed', error: { code: 'busy', message: 'busy' } }
  const { bridge } = fakeBridge({ proxyAdd: async () => busy, proxyAddFile: async () => busy })
  renderWith(<ProxyPanel mode="key" />, servicesFor(bridge, withState()))
  const send = async (text: string): Promise<string | null> => {
    fireEvent.change(field(), { target: { value: text } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Добавить' }))
    })
    return screen.getByRole('alert').textContent
  }
  expect(await send(TEXT)).toBe('Служебный процесс занят. Повторите через несколько секунд')
  expect(await send('https://sub.example.com/list')).toBe('Уже идёт загрузка подписки')
  cleanup()
  renderWith(<ProxyPanel mode="file" />, servicesFor(bridge, withState()))
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Выбрать файл .conf…' }))
  })
  expect(screen.getByRole('alert').textContent).toBe('Служебный процесс занят. Повторите через несколько секунд')
})

test('a text with nothing taken stays in the field, and an unknown code reads as a line not recognised', async () => {
  const proxyAdd = vi.fn(async () => result({ rejected: [{ line: 1, code: 'from_a_newer_helper' }] }))
  const { bridge } = fakeBridge({ proxyAdd })
  renderWith(<ProxyPanel mode="key" />, servicesFor(bridge, withState()))
  fireEvent.change(field(), { target: { value: 'not a link' } })
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Добавить' }))
  })
  const status = screen.getByRole('status')
  expect(within(status).getByText('Добавлено серверов: 0')).toBeTruthy()
  expect(within(status).getByText('Строка 1: Строка не распознана')).toBeTruthy()
  expect(within(status).queryByText(/Уже были в списке/)).toBeNull()
  expect(within(status).queryByText(/И ещё отклонено/)).toBeNull()
  expect(field().value).toBe('not a link')

  // the servers already there also clear it: the text did its job
  proxyAdd.mockResolvedValueOnce(result({ existing: 2 }))
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Добавить' }))
  })
  expect(screen.getByText('Уже были в списке: 2')).toBeTruthy()
  expect(field().value).toBe('')
})

test('a failed add shows its error as an alert in the same place; a thrown call is a lost link', async () => {
  const proxyAdd = vi.fn(
    async (): Promise<ProxyOutcome> => ({ kind: 'failed', error: { code: 'busy', message: 'another subscription is being downloaded' } })
  )
  const { bridge } = fakeBridge({ proxyAdd })
  renderWith(<ProxyPanel mode="key" />, servicesFor(bridge, withState()))
  fireEvent.change(field(), { target: { value: 'https://sub.example.com/list' } })
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Добавить' }))
  })
  expect(screen.getByRole('alert').textContent).toBe('Уже идёт загрузка подписки')
  expect(screen.queryByRole('status')).toBeNull()
  // a failure keeps the text: the user may try again
  expect(field().value).toBe('https://sub.example.com/list')

  proxyAdd.mockRejectedValueOnce(new Error('ipc gone'))
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Добавить' }))
  })
  expect(screen.getByRole('alert').textContent).toBe('Нет связи со служебным процессом или внутренняя ошибка')
  expect(screen.queryByText(/ipc gone/)).toBeNull()
})

test('the file button: a closed dialog shows nothing, a file read shows the summary', async () => {
  const d = deferred()
  const proxyAddFile = vi.fn(() => d.call())
  const proxyAdd = vi.fn(async (): Promise<ProxyOutcome> => ({ kind: 'cancelled' }))
  const { bridge } = fakeBridge({ proxyAddFile, proxyAdd })
  renderWith(<ProxyPanel mode="file" />, servicesFor(bridge, withState()))
  expect(screen.queryByRole('textbox')).toBeNull()
  const choose = (): HTMLButtonElement => screen.getByRole('button', { name: 'Выбрать файл .conf…' }) as HTMLButtonElement

  fireEvent.click(choose())
  expect(proxyAddFile).toHaveBeenCalledTimes(1)
  expect(choose().disabled).toBe(true)
  await d.answer({ kind: 'cancelled' })
  expect(choose().disabled).toBe(false)
  expect(screen.queryByRole('status')).toBeNull()
  expect(screen.queryByRole('alert')).toBeNull()

  fireEvent.click(choose())
  await d.answer(result({ added: [A.id] }))
  expect(screen.getByRole('status').textContent).toBe('Добавлено серверов: 1')

  // a file too large comes as the rejection of the whole text
  fireEvent.click(choose())
  await d.answer(result({ rejected: [{ line: 0, code: 'too_long' }] }))
  expect(within(screen.getByRole('status')).getByText('Текст или файл больше 64 КБ')).toBeTruthy()
  expect(proxyAdd).not.toHaveBeenCalled()
})

test('the server list is under the panel once there are servers', () => {
  const { bridge } = fakeBridge()
  const services = servicesFor(bridge, withState())
  renderWith(<ProxyPanel mode="key" />, services)
  expect(screen.queryByRole('group', { name: 'Серверы' })).toBeNull()
  act(() => services.store.getState().setSnapshot(withState(proxyState([A, B], A.id, [], { phase: 'off' }))))
  const group = screen.getByRole('group', { name: 'Серверы' })
  expect(within(group).getAllByRole('radio')).toHaveLength(2)
  // no remove buttons unless asked for
  expect(screen.queryByRole('button', { name: /^Удалить/ })).toBeNull()
})

test('the panel in English', async () => {
  setLang('en')
  const proxyAdd = vi.fn(async () => result({ added: [A.id], existing: 3, rejected: [{ line: 2, code: 'plugin' }], rejectedMore: 1 }))
  const { bridge } = fakeBridge({ proxyAdd })
  renderWith(<ProxyPanel mode="key" />, servicesFor(bridge, { ...withState(), language: 'en' }))
  fireEvent.change(screen.getByRole('textbox', { name: 'Links or a subscription address' }), { target: { value: TEXT } })
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Add' }))
  })
  expect([...screen.getByRole('status').querySelectorAll('p, li')].map((e) => e.textContent)).toEqual([
    'Servers added: 1',
    'Already in the list: 3',
    "Line 2: Shadowsocks links with a plugin aren't supported yet",
    'More lines rejected: 1'
  ])
})
