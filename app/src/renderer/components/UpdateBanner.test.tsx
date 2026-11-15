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

  show({ kind: 'installed', version: '1.2.0' })
  await click('Закрыть')
  expect(calls.at(-1)).toBe('dismiss')
  expect(calls).toHaveLength(6)

  show({ kind: 'available', version: '1.2.0', notesUrl: N, hidden: true })
  expect(screen.queryByRole('button')).toBeNull()
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
