import { cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { fakeBridge, proxyState, renderWith, servicesFor, snap } from '../testing'
import { Subscriptions } from './Subscriptions'

// the module is replaced to see exactly what the code is made of; a 21x21 checker stands for the matrix
const made = vi.hoisted(() => ({ factory: vi.fn(), addData: vi.fn(), make: vi.fn() }))
vi.mock('qrcode-generator', () => ({
  default: (...args: unknown[]) => {
    made.factory(...args)
    return {
      addData: made.addData,
      make: made.make,
      getModuleCount: () => 21,
      isDark: (r: number, c: number) => (r + c) % 2 === 0
    }
  }
}))

afterEach(cleanup)
beforeEach(() => vi.clearAllMocks())

const ADDRESS = 'https://203-0-113-10.sslip.io:8443/abc/sub'
const SUB = { id: 'sub-0123456789abcdef', name: 'sub.example.com', updatedAt: '', count: 1, error: null }

function setup(reveal: ReturnType<typeof fakeBridge>['bridge']['proxyReveal']) {
  const { bridge } = fakeBridge({ proxyReveal: reveal })
  const s = proxyState([], null, [SUB], { phase: 'off' })
  const services = servicesFor(bridge, { ...snap(), state: s })
  const view = renderWith(<Subscriptions state={s} />, services)
  return { view }
}

const button = (): HTMLElement => screen.getByRole('button', { name: 'Подключить телефон' })

test('the button asks for the subscription by its id and the dialog shows the code and the caption', async () => {
  const reveal = vi.fn(async (_id: string) => ({ address: ADDRESS }))
  setup(reveal)
  fireEvent.click(button())
  const dialog = await screen.findByRole('dialog', { name: 'Подключить телефон' })
  expect(reveal).toHaveBeenCalledWith(SUB.id)
  expect(dialog.getAttribute('aria-modal')).toBe('true')
  expect(dialog.querySelector('svg')).not.toBeNull()
  expect(dialog.textContent).toContain('Отсканируйте в Hiddify или v2rayNG. Это секрет')
  // the code is made of exactly the address (type 0 = auto, level M); the address is not text
  expect(made.factory).toHaveBeenCalledWith(0, 'M')
  expect(made.addData).toHaveBeenCalledWith(ADDRESS)
  expect(document.body.textContent).not.toContain('sslip.io')
})

test('Esc closes it, focus goes back to the button and the address is gone from the DOM', async () => {
  setup(async () => ({ address: ADDRESS }))
  button().focus()
  fireEvent.click(button())
  const dialog = await screen.findByRole('dialog')
  expect(dialog.contains(document.activeElement)).toBe(true)
  fireEvent.keyDown(dialog, { key: 'Escape' })
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(button())
  expect(document.body.innerHTML).not.toContain('sslip.io')
})

test('«Закрыть» closes it, and Tab stays inside', async () => {
  setup(async () => ({ address: ADDRESS }))
  fireEvent.click(button())
  const dialog = await screen.findByRole('dialog')
  const close = screen.getByRole('button', { name: 'Закрыть' })
  close.focus()
  fireEvent.keyDown(close, { key: 'Tab' })
  expect(dialog.contains(document.activeElement)).toBe(true)
  fireEvent.click(close)
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(document.activeElement).toBe(button())
})

test('an error shows a short line by the button, not a dialog', async () => {
  setup(async () => ({ error: { code: 'unknown_server', message: 'x' } }))
  fireEvent.click(button())
  expect((await screen.findByRole('alert')).textContent).toBe('Подписка не найдена')
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(made.addData).not.toHaveBeenCalled()
})

test('another error, or a failed call, says the address could not be got', async () => {
  setup(async () => {
    throw new Error('boom')
  })
  fireEvent.click(button())
  await waitFor(() => expect(screen.getByRole('alert').textContent).toBe('Не удалось получить адрес подписки'))
  expect(screen.queryByRole('dialog')).toBeNull()
})
