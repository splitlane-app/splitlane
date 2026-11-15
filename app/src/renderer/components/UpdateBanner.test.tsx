import { act, cleanup, fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import type { UpdateState } from '../../shared/update'
import { fakeBridge, renderWith, servicesFor, snap } from '../testing'
import { UpdateBanner } from './UpdateBanner'

afterEach(cleanup)

const N = 'https://github.com/splitlane-app/splitlane/releases/tag/v1.2.0'

function withUpdate(update: UpdateState) {
  return { ...snap({ phase: 'on' }), update }
}

const click = async (name: string): Promise<void> => {
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name }))
  })
}

test('UpdateBanner shows nothing without an offer', () => {
  const { bridge } = fakeBridge()
  const { container } = renderWith(<UpdateBanner />, servicesFor(bridge, withUpdate({ kind: 'idle' })))
  expect(container.textContent).toBe('')
})

test('UpdateBanner buttons call the bridge', async () => {
  const calls: string[] = []
  const rec = (name: string) => async (): Promise<void> => {
    calls.push(name)
  }
  const { bridge } = fakeBridge({
    installUpdate: rec('install'),
    cancelUpdate: rec('cancel'),
    updateLater: rec('later'),
    dismissUpdate: rec('dismiss'),
    openUpdateNotes: async () => {
      calls.push('notes')
      return true
    }
  })
  const services = servicesFor(bridge, withUpdate({ kind: 'available', version: '1.2.0', notesUrl: N, hidden: false }))
  const show = (u: UpdateState): void => act(() => services.store.getState().setSnapshot(withUpdate(u)))
  renderWith(<UpdateBanner />, services)

  expect(screen.getByText('Доступна версия 1.2.0')).toBeTruthy()
  await click('Обновить')
  await click('Что нового')
  await click('Позже')
  expect(calls).toEqual(['install', 'notes', 'later'])

  show({ kind: 'downloading', version: '1.2.0', notesUrl: N, percent: 5 })
  await click('Отменить')
  expect(calls.at(-1)).toBe('cancel')

  show({ kind: 'downloadFailed', version: '1.2.0', notesUrl: N, reason: 'HTTP 500' })
  await click('Повторить')
  expect(calls.at(-1)).toBe('install')

  expect(calls).toHaveLength(5)

  // a result can be closed
  for (const u of [
    { kind: 'cancelled', version: '1.2.0', notesUrl: N },
    { kind: 'downloadFailed', version: '1.2.0', notesUrl: N, reason: 'HTTP 500' },
    { kind: 'installRefused', version: '1.2.0', notesUrl: N, reason: '', canRetry: true }
  ] satisfies UpdateState[]) {
    show(u)
    await click('Закрыть')
    expect(calls.at(-1), u.kind).toBe('dismiss')
  }
  expect(calls).toHaveLength(8)

  show({ kind: 'available', version: '1.2.0', notesUrl: N, hidden: true })
  expect(screen.queryByRole('button')).toBeNull()
})

test("a refusal of the window's own version offers only Закрыть", async () => {
  const installUpdate = vi.fn(async () => {})
  const dismissUpdate = vi.fn(async () => {})
  const { bridge } = fakeBridge({ installUpdate, dismissUpdate })
  const refused: UpdateState = {
    kind: 'installRefused',
    version: '1.2.0',
    notesUrl: N,
    reason: 'the task cannot be registered',
    canRetry: false
  }
  renderWith(<UpdateBanner />, servicesFor(bridge, withUpdate(refused)))
  expect(screen.getByText('Обновление до 1.2.0 не установлено: the task cannot be registered')).toBeTruthy()
  expect(screen.getAllByRole('button').map((b) => b.textContent)).toEqual(['Закрыть'])
  await click('Закрыть')
  expect(dismissUpdate).toHaveBeenCalledTimes(1)
  expect(installUpdate).not.toHaveBeenCalled()
})

test('UpdateBanner survives a rejecting bridge', async () => {
  const installUpdate = vi.fn(async () => {
    throw new Error('x')
  })
  const { bridge } = fakeBridge({ installUpdate })
  const services = servicesFor(bridge, withUpdate({ kind: 'available', version: '1.2.0', notesUrl: N, hidden: false }))
  renderWith(<UpdateBanner />, services)
  await click('Обновить')
  expect(installUpdate).toHaveBeenCalledTimes(1)
  expect(screen.getByText('Доступна версия 1.2.0')).toBeTruthy()
})

test('Cancel stays pressable while the install request is still pending', async () => {
  const cancelUpdate = vi.fn(async () => {})
  const { bridge } = fakeBridge({ installUpdate: () => new Promise<void>(() => {}), cancelUpdate })
  const services = servicesFor(bridge, withUpdate({ kind: 'available', version: '1.2.0', notesUrl: N, hidden: false }))
  renderWith(<UpdateBanner />, services)
  await click('Обновить')
  act(() => services.store.getState().setSnapshot(withUpdate({ kind: 'downloading', version: '1.2.0', notesUrl: N, percent: 1 })))
  const cancel = screen.getByRole('button', { name: 'Отменить' }) as HTMLButtonElement
  expect(cancel.disabled).toBe(false)
  await click('Отменить')
  expect(cancelUpdate).toHaveBeenCalledTimes(1)
})

test('the success notice has a close icon, no Закрыть button, and hides itself after 10 seconds', async () => {
  vi.useFakeTimers()
  try {
    const dismissUpdate = vi.fn(async () => {})
    const { bridge } = fakeBridge({ dismissUpdate })
    const services = servicesFor(bridge, withUpdate({ kind: 'installed', version: '1.2.0' }))
    renderWith(<UpdateBanner />, services)
    expect(screen.getByText('SplitLane обновлён до 1.2.0')).toBeTruthy()
    // one button: the icon, named Закрыть
    const buttons = screen.getAllByRole('button')
    expect(buttons).toHaveLength(1)
    expect(buttons[0]!.getAttribute('aria-label')).toBe('Закрыть')
    expect(buttons[0]!.textContent).toBe('')

    await act(async () => {
      vi.advanceTimersByTime(9_999)
    })
    expect(dismissUpdate).not.toHaveBeenCalled()
    await act(async () => {
      vi.advanceTimersByTime(1)
    })
    expect(dismissUpdate).toHaveBeenCalledTimes(1)

    // the icon closes it at once
    await act(async () => {
      fireEvent.click(buttons[0]!)
    })
    expect(dismissUpdate).toHaveBeenCalledTimes(2)
  } finally {
    vi.useRealTimers()
  }
})

test('the timer of the success notice is dropped when the state changes', async () => {
  vi.useFakeTimers()
  try {
    const dismissUpdate = vi.fn(async () => {})
    const { bridge } = fakeBridge({ dismissUpdate })
    const services = servicesFor(bridge, withUpdate({ kind: 'installed', version: '1.2.0' }))
    renderWith(<UpdateBanner />, services)
    act(() => services.store.getState().setSnapshot(withUpdate({ kind: 'idle' })))
    await act(async () => {
      vi.advanceTimersByTime(20_000)
    })
    expect(dismissUpdate).not.toHaveBeenCalled()
  } finally {
    vi.useRealTimers()
  }
})
