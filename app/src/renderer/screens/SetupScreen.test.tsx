import { act, cleanup, fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import type { Snapshot } from '../../shared/ipc'
import type { InstallState } from '../../shared/ipc'
import { DEFAULT_SETTINGS } from '../../shared/settings'
import { fakeBridge, renderWith, servicesFor, snap } from '../testing'
import { SetupScreen } from './SetupScreen'

afterEach(cleanup)

const setup = (install: InstallState): Snapshot => ({
  ...snap({}, { kind: 'setup-needed' }),
  state: null,
  install
})

test('SetupScreen installs on the button and shows the progress', async () => {
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
  fireEvent.click(await screen.findByRole('button', { name: 'Установить службу' }))
  expect(installHelper).toHaveBeenCalledTimes(1)

  act(() => services.store.getState().setSnapshot(setup({ kind: 'running' })))
  expect(screen.getByText('Подтвердите запрос Windows…')).toBeTruthy()
  expect(screen.queryByRole('button')).toBeNull()

  act(() => services.store.getState().setSnapshot(setup({ kind: 'done', outcome: { kind: 'ok' } })))
  expect(screen.getByText('Готово, служебный процесс запускается…')).toBeTruthy()
})

test('SetupScreen shows the install hint when not installed', async () => {
  const { bridge } = fakeBridge({ appInfo: async () => ({ version: '0.2.0', installed: false }) })
  renderWith(<SetupScreen update={false} />, servicesFor(bridge, setup({ kind: 'idle' })))
  expect(
    await screen.findByText('Эта копия SplitLane не установлена. Установите SplitLane установщиком: он поставит и фоновую службу.')
  ).toBeTruthy()
  expect(screen.queryByRole('button', { name: 'Установить службу' })).toBeNull()
})

test('SetupScreen shows neither button nor hint until appInfo answers', async () => {
  let answer: (i: { version: string; installed: boolean }) => void = () => {}
  const { bridge } = fakeBridge({ appInfo: () => new Promise((r) => (answer = r)) })
  renderWith(<SetupScreen update={false} />, servicesFor(bridge, setup({ kind: 'idle' })))
  expect(screen.getByRole('heading', { name: 'Первая настройка' })).toBeTruthy()
  expect(screen.queryByRole('button')).toBeNull()
  expect(screen.queryByText(/не установлена/)).toBeNull()
  await act(async () => answer({ version: '0.2.0', installed: true }))
  expect(screen.getByRole('button', { name: 'Установить службу' })).toBeTruthy()
})

test('SetupScreen treats a failed appInfo as not installed', async () => {
  const { bridge } = fakeBridge({
    appInfo: async () => {
      throw new Error('no bridge')
    }
  })
  renderWith(<SetupScreen update={false} />, servicesFor(bridge, setup({ kind: 'idle' })))
  expect(await screen.findByText(/не установлена/)).toBeTruthy()
  expect(screen.queryByRole('button')).toBeNull()
})

test('SetupScreen offers the update of an older helper', async () => {
  const { bridge } = fakeBridge()
  renderWith(<SetupScreen update />, servicesFor(bridge, setup({ kind: 'idle' })))
  expect(screen.getByRole('heading', { name: 'Обновите служебный процесс' })).toBeTruthy()
  expect(await screen.findByRole('button', { name: 'Обновить службу' })).toBeTruthy()
})

test('SetupScreen shows the update banner', async () => {
  // after an install refusal the window may open here: the result of the update shows on this screen too
  const dismissUpdate = vi.fn(async () => {})
  const { bridge } = fakeBridge({ dismissUpdate })
  const notesUrl = 'https://github.com/splitlane-app/splitlane/releases/tag/v1.2.0'
  const services = servicesFor(bridge, {
    ...setup({ kind: 'idle' }),
    update: { kind: 'installRefused', version: '1.2.0', notesUrl, reason: 'the tunnel is on', canRetry: true }
  })
  renderWith(<SetupScreen update={false} />, services)
  expect(screen.getByText('Обновление до 1.2.0 не установлено: the tunnel is on')).toBeTruthy()
  expect(screen.getByRole('button', { name: 'Повторить' })).toBeTruthy()
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Закрыть' }))
  })
  expect(dismissUpdate).toHaveBeenCalledTimes(1)
  expect(await screen.findByRole('button', { name: 'Установить службу' })).toBeTruthy()
})

test('SetupScreen shows cancelled and refused outcomes', async () => {
  const { bridge } = fakeBridge()
  const services = servicesFor(bridge, setup({ kind: 'done', outcome: { kind: 'cancelled' } }))
  renderWith(<SetupScreen update={false} />, services)
  expect(screen.getByText('Настройка отменена')).toBeTruthy()
  expect(await screen.findByRole('button', { name: 'Установить службу' })).toBeTruthy()

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

test('SetupScreen has the language selector, which saves the setting', () => {
  const setSettings = vi.fn(async () => DEFAULT_SETTINGS)
  const { bridge } = fakeBridge({ setSettings })
  renderWith(<SetupScreen update={false} />, servicesFor(bridge, setup({ kind: 'idle' })))
  fireEvent.change(screen.getByRole('combobox', { name: 'Язык' }), { target: { value: 'en' } })
  expect(setSettings).toHaveBeenLastCalledWith({ language: 'en' })
})
