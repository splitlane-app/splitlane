import { act, cleanup, fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import type { Snapshot } from '../../shared/ipc'
import type { InstallState } from '../../shared/ipc'
import { fakeBridge, renderWith, servicesFor, snap } from '../testing'
import { SetupScreen } from './SetupScreen'

afterEach(cleanup)

const setup = (install: InstallState): Snapshot => ({
  ...snap({}, { kind: 'setup-needed' }),
  state: null,
  install
})

test('SetupScreen installs on the button and shows the progress', () => {
  const installHelper = vi.fn(async () => {})
  const { bridge } = fakeBridge({ installHelper })
  const services = servicesFor(bridge, setup({ kind: 'idle' }))
  renderWith(<SetupScreen update={false} />, services)
  expect(screen.getByRole('heading', { name: 'Первая настройка' })).toBeTruthy()
  expect(
    screen.getByText(
      'Чтобы разделять трафик программ, SplitLane нужна фоновая служба с правами администратора. Она устанавливается один раз: Windows попросит подтвердить. Потом окно работает без прав администратора.'
    )
  ).toBeTruthy()
  fireEvent.click(screen.getByRole('button', { name: 'Установить службу' }))
  expect(installHelper).toHaveBeenCalledTimes(1)

  act(() => services.store.getState().setSnapshot(setup({ kind: 'running' })))
  expect(screen.getByText('Подтвердите запрос Windows…')).toBeTruthy()
  expect(screen.queryByRole('button')).toBeNull()

  act(() => services.store.getState().setSnapshot(setup({ kind: 'done', outcome: { kind: 'ok' } })))
  expect(screen.getByText('Готово, служебный процесс запускается…')).toBeTruthy()
})

test('SetupScreen offers the update of an older helper', () => {
  const { bridge } = fakeBridge()
  renderWith(<SetupScreen update />, servicesFor(bridge, setup({ kind: 'idle' })))
  expect(screen.getByRole('heading', { name: 'Обновите служебный процесс' })).toBeTruthy()
  expect(screen.getByRole('button', { name: 'Обновить службу' })).toBeTruthy()
})

test('SetupScreen shows cancelled and refused outcomes', () => {
  const { bridge } = fakeBridge()
  const services = servicesFor(bridge, setup({ kind: 'done', outcome: { kind: 'cancelled' } }))
  renderWith(<SetupScreen update={false} />, services)
  expect(screen.getByText('Настройка отменена')).toBeTruthy()
  expect(screen.getByRole('button', { name: 'Установить службу' })).toBeTruthy()

  act(() =>
    services.store
      .getState()
      .setSnapshot(setup({ kind: 'done', outcome: { kind: 'refused', reason: 'installed for another user' } }))
  )
  expect(screen.getByText('installed for another user')).toBeTruthy()
  expect(screen.queryByText('Настройка отменена')).toBeNull()
  expect(screen.getByRole('button', { name: 'Установить службу' })).toBeTruthy()

  act(() =>
    services.store.getState().setSnapshot(setup({ kind: 'done', outcome: { kind: 'failed', detail: 'exit code 5' } }))
  )
  expect(screen.getByText('Не удалось выполнить настройку')).toBeTruthy()
  expect(screen.getByText('exit code 5')).toBeTruthy()
})
