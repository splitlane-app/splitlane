import { act, cleanup, fireEvent, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { fakeBridge, renderWith, servicesFor, snap } from '../testing'
import { UpdateAppScreen } from './UpdateAppScreen'

afterEach(cleanup)

test('the update-app screen shows the update offer and the check button', async () => {
  const checkUpdates = vi.fn(async () => {})
  const installUpdate = vi.fn(async () => {})
  const { bridge } = fakeBridge({ checkUpdates, installUpdate })
  const notes = 'https://github.com/splitlane-app/splitlane/releases/tag/v1.2.0'
  const services = servicesFor(bridge, { ...snap({ phase: 'on' }), update: { kind: 'available', version: '1.2.0', notesUrl: notes, hidden: false } })
  renderWith(<UpdateAppScreen />, services)
  expect(screen.getByText('Доступна версия 1.2.0')).toBeTruthy()
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Обновить' }))
  })
  expect(installUpdate).toHaveBeenCalledTimes(1)
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Проверить обновления' }))
  })
  expect(checkUpdates).toHaveBeenCalledTimes(1)
})
