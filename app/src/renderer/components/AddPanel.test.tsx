import { DragDropProvider } from '@dnd-kit/react'
import { act, cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import type { Program } from '../../shared/programs'
import { activity, entry, fakeBridge, renderWith, rule, servicesFor, snap } from '../testing'
import { AddPanel } from './AddPanel'

afterEach(cleanup)

const MAIL = 'C:\\Apps\\Mail\\mail.exe'
const GAME_DIR = 'C:\\Games\\Game\\'
const GAME = `${GAME_DIR}game.exe`
const ID1 = '6f1c2a4e-0b7d-4c3a-9e21-5d8f7a1b2c01'

function setup(over: Parameters<typeof fakeBridge>[0] = {}, phase: 'on' | 'off' = 'on') {
  const fake = fakeBridge(over)
  const services = servicesFor(fake.bridge, snap({ phase, apps: [rule(ID1, 'Mail', MAIL, 'direct')] }))
  services.store.getState().setActivity(activity([entry(GAME_DIR, { name: 'Game', exe: GAME, down: 5 })]))
  services.store.getState().openAdd(true)
  renderWith(
    <DragDropProvider>
      <AddPanel />
    </DragDropProvider>,
    services
  )
  return { services, commands: fake.commands }
}

test('AddPanel online tab pins a program or sends it to the other lane', async () => {
  const { commands } = setup()
  expect(screen.getByText('Game')).toBeTruthy()
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Напрямую: Game' }))
  })
  await waitFor(() => expect(commands.length).toBe(1))
  const sent = commands[0]
  if (sent.method !== 'lanes.set') throw new Error('lanes.set expected')
  expect(sent.apps.map((a) => [a.exe, a.lane])).toEqual([
    [MAIL, 'direct'],
    [GAME, 'direct']
  ])
})

test('AddPanel online tab is empty with an explanation while routing is off', () => {
  setup({}, 'off')
  expect(screen.getByText(/Включите деление/)).toBeTruthy()
})

test('AddPanel asks for the installed programs once and marks pinned ones', async () => {
  const programs: Program[] = [
    { exe: MAIL, name: 'Mail' },
    { exe: 'C:\\Apps\\Notes\\notes.exe', name: 'Notes' }
  ]
  const installedPrograms = vi.fn(async () => programs)
  setup({ installedPrograms })
  fireEvent.click(screen.getByRole('tab', { name: 'Установленные' }))
  expect(screen.getByText('Ищем установленные программы…')).toBeTruthy()
  await screen.findByText('Notes')
  expect(screen.getByText('закреплена: Напрямую')).toBeTruthy()
  expect((screen.getByRole('button', { name: 'Напрямую: Mail' }) as HTMLButtonElement).disabled).toBe(true)
  fireEvent.click(screen.getByRole('tab', { name: 'Сейчас в сети' }))
  fireEvent.click(screen.getByRole('tab', { name: 'Установленные' }))
  expect(installedPrograms).toHaveBeenCalledTimes(1)
})

test('AddPanel shows the picked file with both lane buttons, and closes with Escape', async () => {
  const { services } = setup({ pickProgram: async () => ({ exe: 'C:\\Tools\\tool.exe', name: 'Tool' }) })
  fireEvent.click(screen.getByRole('button', { name: 'Выбрать файл…' }))
  await screen.findByRole('button', { name: 'Через VPN: Tool' })
  expect(screen.getByRole('button', { name: 'Напрямую: Tool' })).toBeTruthy()
  fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
  expect(services.store.getState().ui.addOpen).toBe(false)
})

test('AddPanel gives the picked row and the list row of one program different drag ids', async () => {
  setup({
    installedPrograms: async () => [{ exe: MAIL, name: 'Mail' }],
    pickProgram: async () => ({ exe: MAIL, name: 'Mail' })
  })
  fireEvent.click(screen.getByRole('button', { name: 'Выбрать файл…' }))
  await screen.findByRole('button', { name: 'Через VPN: Mail' })
  fireEvent.click(screen.getByRole('tab', { name: 'Установленные' }))
  await waitFor(() => expect(document.querySelectorAll('[data-add-row]').length).toBe(2))
  const ids = Array.from(document.querySelectorAll<HTMLElement>('[data-add-row]')).map((e) => e.dataset.dragId)
  expect(new Set(ids).size).toBe(2)
})
