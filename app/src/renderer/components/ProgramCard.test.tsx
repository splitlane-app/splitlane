import { act, cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import type { Activity, ActivityDetail } from '../../shared/protocol'
import { activity, entry, fakeBridge, renderWith, rule, servicesFor, snap } from '../testing'
import { ProgramCard } from './ProgramCard'

afterEach(cleanup)

const MAIL = 'C:\\Apps\\Mail\\mail.exe'
const ID1 = '6f1c2a4e-0b7d-4c3a-9e21-5d8f7a1b2c01'

const detail: ActivityDetail = {
  key: ID1,
  exes: ['C:\\Apps\\Mail\\mail.exe', 'C:\\Apps\\Mail\\helper.exe'],
  connections: [
    { host: 'mail.example.org', lane: 'vpn', down: 2048, up: 0 },
    { host: '203.0.113.5', lane: 'direct', down: 0, up: 0 }
  ]
}

function setup(withDetail: boolean) {
  const setActivityDetail = vi.fn(async () => {})
  const fake = fakeBridge({ setActivityDetail })
  const services = servicesFor(fake.bridge, snap({ apps: [rule(ID1, 'Mail', MAIL, 'vpn')] }))
  const a: Activity = {
    ...activity([entry(ID1, { ruleId: ID1, name: 'Mail', exe: MAIL, down: 100, connections: 2 })]),
    ...(withDetail ? { detail } : {})
  }
  services.store.getState().setActivity(a)
  services.store.getState().openCard(ID1)
  return { services, setActivityDetail, commands: fake.commands }
}

test('ProgramCard subscribes to detail on open and unsubscribes on close', () => {
  const { services, setActivityDetail } = setup(true)
  renderWith(<ProgramCard />, services)
  expect(setActivityDetail).toHaveBeenCalledWith(ID1)
  const dialog = screen.getByRole('dialog', { name: 'Mail' })
  expect(within(dialog).getByText('закреплена · через VPN')).toBeTruthy()

  fireEvent.click(screen.getByRole('button', { name: 'Закрыть' }))
  expect(services.store.getState().ui.cardKey).toBeNull()
  expect(setActivityDetail).toHaveBeenLastCalledWith(null)
  expect(screen.queryByRole('dialog')).toBeNull()
})

test('ProgramCard toggles the whole folder', async () => {
  const { services, commands } = setup(true)
  renderWith(<ProgramCard />, services)
  const sw = screen.getByRole('switch', { name: 'Вся папка программы' })
  expect(sw.getAttribute('aria-checked')).toBe('true')
  await act(async () => {
    fireEvent.click(sw)
  })
  await waitFor(() => expect(commands.length).toBe(1))
  const sent = commands[0]
  expect(sent.method).toBe('lanes.set')
  if (sent.method === 'lanes.set') expect(sent.apps[0].matcher.kind).toBe('file')
})

test('ProgramCard lists connections with lanes and speeds', () => {
  const { services } = setup(true)
  renderWith(<ProgramCard />, services)
  const dialog = screen.getByRole('dialog', { name: 'Mail' })
  expect(within(dialog).getByText('C:\\Apps\\Mail\\helper.exe')).toBeTruthy()
  const rows = within(dialog).getAllByRole('row').slice(1)
  expect(rows).toHaveLength(2)
  expect(within(rows[0]).getByText('mail.example.org')).toBeTruthy()
  expect(within(rows[0]).getByText('Через VPN')).toBeTruthy()
  expect(within(rows[0]).getByText('2,0 КБ/с')).toBeTruthy()
  expect(within(rows[1]).getByText('Напрямую')).toBeTruthy()
  expect(within(rows[1]).getByText('тихо')).toBeTruthy()
})

test('ProgramCard says there are no connections without detail', () => {
  const { services } = setup(false)
  renderWith(<ProgramCard />, services)
  expect(screen.getByText('Нет соединений')).toBeTruthy()
})
