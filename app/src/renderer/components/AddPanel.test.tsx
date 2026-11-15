import { DragDropProvider } from '@dnd-kit/react'
import { act, cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
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

test('AddPanel rows name the program file, a protected program offers no lane', async () => {
  const TS_IPN = 'C:\\Program Files\\Tailscale\\tailscale-ipn.exe'
  setup({
    installedPrograms: async () => [
      { exe: MAIL, name: 'Mail' },
      { exe: TS_IPN, name: 'Tailscale' }
    ]
  })
  const online = document.querySelector<HTMLElement>(`[data-add-row="${CSS.escape(GAME_DIR)}"]`)!
  expect(within(online).getByText('game.exe')).toBeTruthy()
  expect(within(online).getByText('сейчас: Через VPN')).toBeTruthy()

  fireEvent.click(screen.getByRole('tab', { name: 'Установленные' }))
  await screen.findByText('Tailscale')
  const mail = document.querySelector<HTMLElement>(`[data-add-row="${CSS.escape(MAIL)}"]`)!
  expect(within(mail).getByText('mail.exe')).toBeTruthy()
  const ts = document.querySelector<HTMLElement>(`[data-add-row="${CSS.escape(TS_IPN)}"]`)!
  expect(within(ts).getByText('tailscale-ipn.exe')).toBeTruthy()
  expect(within(ts).getByText('встроенное правило: Напрямую')).toBeTruthy()
  // no lane buttons (the drag handle of the row has the role of a button too)
  expect(ts.querySelectorAll('button')).toHaveLength(0)
  expect(within(mail).getAllByRole('button', { name: /: Mail$/ })).toHaveLength(2)
})

test('AddPanel lists follow the search of the header', async () => {
  const NOTES = 'C:\\Apps\\Notes\\notes.exe'
  const { services } = setup({
    installedPrograms: async () => [
      { exe: MAIL, name: 'Mail' },
      { exe: NOTES, name: 'Заметки' }
    ]
  })
  const search = (q: string): void => act(() => services.store.getState().setSearch(q))

  search('GAME.exe')
  expect(screen.getByText('Game')).toBeTruthy()
  search('заметки')
  expect(screen.queryByText('Game')).toBeNull()
  expect(screen.getByText('Ничего не найдено')).toBeTruthy()

  fireEvent.click(screen.getByRole('tab', { name: 'Установленные' }))
  await screen.findByText('Заметки')
  expect(screen.queryByText('Mail')).toBeNull()
  search('c:\\apps\\mail')
  expect(screen.getByText('Mail')).toBeTruthy()
  expect(screen.queryByText('Заметки')).toBeNull()
  search('нет такой')
  expect(screen.getByText('Ничего не найдено')).toBeTruthy()
  search('')
  expect(screen.getByText('Mail')).toBeTruthy()
  expect(screen.getByText('Заметки')).toBeTruthy()
})

test('AddPanel shows the picked file with both lane buttons, and closes with Escape', async () => {
  const { services } = setup({
    pickProgram: async () => ({ kind: 'program', program: { exe: 'C:\\Tools\\tool.exe', name: 'Tool' } })
  })
  fireEvent.click(screen.getByRole('button', { name: 'Выбрать файл…' }))
  await screen.findByRole('button', { name: 'Через VPN: Tool' })
  expect(screen.getByRole('button', { name: 'Напрямую: Tool' })).toBeTruthy()
  fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
  expect(services.store.getState().ui.addOpen).toBe(false)
})

test('AddPanel names a picked file that is not a program', async () => {
  const { services } = setup({ pickProgram: async () => ({ kind: 'skipped', name: 'tool.exe', reason: 'notProgram' }) })
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Выбрать файл…' }))
  })
  expect(services.store.getState().notice).toBe('Не программа: tool.exe')
  expect(screen.queryByText('Выбранный файл')).toBeNull()
})

test('AddPanel explains a picked game shortcut whose game is not found', async () => {
  const { services } = setup({ pickProgram: async () => ({ kind: 'skipped', name: 'Старая игра', reason: 'game' }) })
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: 'Выбрать файл…' }))
  })
  expect(services.store.getState().notice).toBe(
    'Старая игра: это ярлык игры — запустите игру, и она появится в «Сейчас в сети»'
  )
  expect(screen.queryByText('Выбранный файл')).toBeNull()
})

test('AddPanel gives the picked row and the list row of one program different drag ids', async () => {
  setup({
    installedPrograms: async () => [{ exe: MAIL, name: 'Mail' }],
    pickProgram: async () => ({ kind: 'program', program: { exe: MAIL, name: 'Mail' } })
  })
  fireEvent.click(screen.getByRole('button', { name: 'Выбрать файл…' }))
  await screen.findByRole('button', { name: 'Через VPN: Mail' })
  fireEvent.click(screen.getByRole('tab', { name: 'Установленные' }))
  await waitFor(() => expect(document.querySelectorAll('[data-add-row]').length).toBe(2))
  const ids = Array.from(document.querySelectorAll<HTMLElement>('[data-add-row]')).map((e) => e.dataset.dragId)
  expect(new Set(ids).size).toBe(2)
})

test('the drag handle of a row names its role in the language of the window', () => {
  setup()
  const handle = document.querySelector('[data-add-row] [aria-roledescription]')
  expect(handle?.getAttribute('aria-roledescription')).toBe('карточка программы')
})
