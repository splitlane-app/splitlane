import { act, cleanup, fireEvent, screen, within } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'
import type { Snapshot } from '../../shared/ipc'
import type { Activity } from '../../shared/protocol'
import { setLang } from '../i18n/t'
import { boardView, type CardVM } from '../model/board'
import { activity, entry, fakeBridge, proxyServer, proxyState, renderWith, rule, servicesFor, snap } from '../testing'
import { AppCard } from './AppCard'
import { Lane } from './Lane'

afterEach(() => {
  cleanup()
  setLang('ru')
})

const MAIL = 'C:\\Apps\\Mail\\mail.exe'
const BROWSER = 'C:\\Apps\\Browser\\browser.exe'
const GAME_DIR = 'C:\\Games\\Game\\'
const GAME = `${GAME_DIR}game.exe`
const ID1 = '6f1c2a4e-0b7d-4c3a-9e21-5d8f7a1b2c01'
const ID2 = '6f1c2a4e-0b7d-4c3a-9e21-5d8f7a1b2c02'

const board = snap({
  apps: [rule(ID1, 'Mail', MAIL, 'vpn'), rule(ID2, 'Browser', BROWSER, 'direct')]
})

const live: Activity = activity([
  entry(ID1, { ruleId: ID1, name: 'Mail', exe: MAIL, down: 1024, up: 1024 }),
  entry(GAME_DIR, { name: 'Game', exe: GAME, down: 4096 }),
  entry('system', { kind: 'system', down: 3 }),
  entry('unattributed', { kind: 'unattributed' })
])

/** The services with the snapshot and the activity set, and the lanes the board would show. */
function setup(s: Snapshot, a: Activity | null) {
  const { bridge } = fakeBridge()
  const services = servicesFor(bridge, s)
  if (a !== null) services.store.getState().setActivity(a)
  const st = services.store.getState()
  const state = s.state!
  const view = boardView(state.apps, state.defaultLane, st.activity, state.addresses, '')
  return { services, view }
}

const cardNames = (lane: HTMLElement): string[] =>
  Array.from(lane.querySelectorAll<HTMLElement>('[data-card]')).map((c) => c.dataset.name ?? '')

const follows = (a: Node, b: Node): boolean => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0

test('Lane shows pinned cards, the line and unpinned cards in the default lane', () => {
  const { services, view } = setup(board, live)
  renderWith(<Lane vm={view.vpn} onMakeDefault={() => {}} />, services)

  const lane = screen.getByRole('region', { name: 'Через VPN' })
  expect(within(lane).getByText('основная')).toBeTruthy()
  expect(within(lane).queryByText('сделать основной')).toBeNull()
  expect(cardNames(lane)).toEqual(['Mail', 'Game', 'Службы Windows', 'WSL и виртуальные машины'])

  const line = within(lane).getByText('остальные программы')
  const card = (name: string): HTMLElement => lane.querySelector<HTMLElement>(`[data-name="${name}"]`)!
  expect(follows(card('Mail'), line)).toBe(true)
  for (const name of ['Game', 'Службы Windows', 'WSL и виртуальные машины']) {
    expect(follows(line, card(name)), name).toBe(true)
    expect(card(name).dataset.pinned, name).toBe('false')
  }
  expect(card('Mail').dataset.pinned).toBe('true')
  // the pinned card has the pin; the unpinned ones are pinned by the same button, the fixed tile has none
  expect(within(card('Mail')).getByRole('button', { name: 'Открепить' })).toBeTruthy()
  expect(within(card('Game')).getByRole('button', { name: 'Закрепить' })).toBeTruthy()
  expect(within(card('WSL и виртуальные машины')).queryByRole('button', { name: 'Закрепить' })).toBeNull()
  expect(within(lane).queryByText('Перетащите сюда программу или ярлык')).toBeNull()
})

test('a program card names its file under its name, a special tile has no second line', () => {
  const { services, view } = setup(board, live)
  renderWith(<Lane vm={view.vpn} onMakeDefault={() => {}} />, services)
  const lane = screen.getByRole('region', { name: 'Через VPN' })
  const card = (name: string): HTMLElement => lane.querySelector<HTMLElement>(`[data-name="${name}"]`)!
  expect(within(card('Mail')).getByText('mail.exe')).toBeTruthy()
  expect(within(card('Game')).getByText('game.exe')).toBeTruthy()
  for (const tile of ['Службы Windows', 'WSL и виртуальные машины']) {
    expect(card(tile).querySelector('[data-card-file]'), tile).toBeNull()
  }
})

test('a program of a built-in rule sits in its own lane with a note, without a pin or a menu', () => {
  const TS_DIR = 'C:\\Program Files\\Tailscale\\'
  const a = activity([
    entry(TS_DIR, { name: 'Tailscale service', exe: `${TS_DIR}tailscaled.exe`, down: 30 }),
    entry(GAME_DIR, { name: 'Game', exe: GAME, down: 4096 }),
    entry('unattributed', { kind: 'unattributed' })
  ])
  const { services, view } = setup(board, a)
  renderWith(
    <>
      <Lane vm={view.vpn} onMakeDefault={() => {}} />
      <Lane vm={view.direct} onMakeDefault={() => {}} />
    </>,
    services
  )
  expect(cardNames(screen.getByRole('region', { name: 'Через VPN' }))).toEqual([
    'Mail',
    'Game',
    'Службы Windows',
    'WSL и виртуальные машины'
  ])
  const direct = screen.getByRole('region', { name: 'Напрямую' })
  expect(cardNames(direct)).toEqual(['Browser', 'Tailscale service'])
  const card = direct.querySelector<HTMLElement>('[data-name="Tailscale service"]')!
  expect(card.dataset.builtin).toBe('true')
  expect(card.dataset.pinned).toBe('false')
  expect(within(card).getByText('встроенное правило · tailscaled.exe')).toBeTruthy()
  expect(within(card).getByRole('button', { name: 'Tailscale service, Напрямую, встроенное правило' })).toBeTruthy()
  expect(within(card).queryByRole('button', { name: /Закрепить|Открепить/ })).toBeNull()
  fireEvent.contextMenu(card)
  expect(screen.queryByRole('menu')).toBeNull()
  // above the drop zone, like the pinned cards
  expect(follows(card, within(direct).getByText('Перетащите сюда программу или ярлык'))).toBe(true)
})

test('Lane explains the unpinned programs while routing is off', () => {
  const { services, view } = setup(snap({ ...board.state!, phase: 'off' }), null)
  renderWith(<Lane vm={view.vpn} onMakeDefault={() => {}} />, services)

  const lane = screen.getByRole('region', { name: 'Через VPN' })
  expect(cardNames(lane)).toEqual(['Mail'])
  expect(within(lane).getByText('остальные программы')).toBeTruthy()
  expect(within(lane).getByText('Незакреплённые программы появятся здесь после включения деления')).toBeTruthy()
})

test('Lane shows the drop hint in the other lane', () => {
  const { services, view } = setup(board, live)
  const made: string[] = []
  renderWith(<Lane vm={view.direct} onMakeDefault={() => made.push('direct')} />, services)

  const lane = screen.getByRole('region', { name: 'Напрямую' })
  expect(cardNames(lane)).toEqual(['Browser'])
  expect(within(lane).getByText('Перетащите сюда программу или ярлык')).toBeTruthy()
  expect(within(lane).queryByText('остальные программы')).toBeNull()
  expect(within(lane).queryByText('основная')).toBeNull()

  fireEvent.click(within(lane).getByRole('button', { name: 'сделать основной' }))
  expect(made).toEqual(['direct'])
})

test('AppCard shows «тихо» without traffic', () => {
  const { services } = setup(board, live)
  const quiet: CardVM = {
    key: ID2,
    ruleId: ID2,
    kind: 'app',
    name: 'Browser',
    exe: BROWSER,
    lane: 'direct',
    pinned: true,
    down: 0,
    up: 0,
    connections: 0,
    movable: true,
    builtin: false
  }
  const opened: string[] = []
  const { unmount } = renderWith(<AppCard card={quiet} onOpen={(key) => opened.push(key)} />, services)
  expect(screen.getByText('тихо')).toBeTruthy()
  fireEvent.click(screen.getByText('Browser'))
  expect(opened).toEqual([ID2])
  unmount()

  renderWith(<AppCard card={{ ...quiet, down: 1536, up: 512 }} onOpen={() => {}} />, services)
  expect(screen.queryByText('тихо')).toBeNull()
  expect(screen.getByText('2,0 КБ/с')).toBeTruthy()
})

test('the direct address is masked until clicked', () => {
  const { services, view } = setup(board, live)
  renderWith(<Lane vm={view.direct} onMakeDefault={() => {}} />, services)

  const lane = screen.getByRole('region', { name: 'Напрямую' })
  expect(within(lane).getByText(/Сайты видят ваш настоящий адрес/)).toBeTruthy()
  expect(within(lane).queryByText('203.0.113.9')).toBeNull()
  const masked = within(lane).getByRole('button', { name: /203\.0\.•\.•/ })

  fireEvent.click(masked)
  expect(within(lane).getByText('203.0.113.9')).toBeTruthy()
  expect(within(lane).queryByText('203.0.•.•')).toBeNull()
})

test('the VPN lane names the server and its address', () => {
  const { services, view } = setup(board, live)
  renderWith(<Lane vm={view.vpn} onMakeDefault={() => {}} />, services)
  const lane = screen.getByRole('region', { name: 'Через VPN' })
  const sees = within(lane).getByText(/^Сайты видят адрес сервера node-a/)
  expect(sees.textContent).toBe('Сайты видят адрес сервера node-a · 198.51.100.7')
})

test('a click on the pin of an unpinned card pins it in the default lane', async () => {
  const sent: unknown[] = []
  const { bridge } = fakeBridge({
    command: async (c) => {
      sent.push(c)
      return { ok: true }
    }
  })
  const services = servicesFor(bridge, board)
  act(() => services.store.getState().setActivity(live))
  const state = board.state!
  const view = boardView(state.apps, state.defaultLane, services.store.getState().activity, state.addresses, '')
  renderWith(<Lane vm={view.vpn} onMakeDefault={() => {}} />, services)

  const lane = screen.getByRole('region', { name: 'Через VPN' })
  const game = lane.querySelector<HTMLElement>('[data-name="Game"]')!
  await act(async () => {
    fireEvent.click(within(game).getByRole('button', { name: 'Закрепить' }))
  })
  expect(sent).toHaveLength(1)
  const c = sent[0] as { method: string; apps: { exe: string; lane: string }[] }
  expect(c.method).toBe('lanes.set')
  expect(c.apps.map((r) => [r.exe, r.lane])).toEqual([
    [MAIL, 'vpn'],
    [BROWSER, 'direct'],
    [GAME, 'vpn']
  ])
})


test('files dropped from Explorer are pinned in that lane, the rest named in a notice', async () => {
  const asked: File[][] = []
  const { bridge, commands } = fakeBridge({
    resolveDropped: async (files) => {
      asked.push(files)
      return [
        { kind: 'program', program: { exe: GAME, name: 'Game' } },
        { kind: 'skipped', name: 'notes', reason: 'notProgram' },
        { kind: 'skipped', name: 'Документы', reason: 'notProgram' },
        { kind: 'skipped', name: 'Старая игра', reason: 'game' }
      ]
    }
  })
  const services = servicesFor(bridge, board)
  const state = board.state!
  const view = boardView(state.apps, state.defaultLane, null, state.addresses, '')
  renderWith(<Lane vm={view.direct} onMakeDefault={() => {}} />, services)

  const lane = screen.getByRole('region', { name: 'Напрямую' })
  const files = [
    new File([''], 'Game.lnk'),
    new File([''], 'notes.txt'),
    new File([''], 'Документы'),
    new File([''], 'Старая игра.url')
  ]
  const dataTransfer = { types: ['Files'], files, dropEffect: 'none' }
  // a drag of text is not for the lane
  const text = { types: ['text/plain'], files: [], dropEffect: 'none' }
  fireEvent.dragOver(lane, { dataTransfer: text })
  expect(lane.dataset.filesOver).toBe('false')
  expect(text.dropEffect).toBe('none')

  fireEvent.dragOver(within(lane).getByText('Browser'), { dataTransfer })
  expect(lane.dataset.filesOver).toBe('true')
  expect(dataTransfer.dropEffect).toBe('copy')
  await act(async () => {
    fireEvent.drop(lane, { dataTransfer })
  })
  expect(lane.dataset.filesOver).toBe('false')
  expect(asked).toEqual([files])
  expect(commands).toHaveLength(1)
  const c = commands[0] as { method: string; apps: { name: string; exe: string; lane: string }[] }
  expect(c.method).toBe('lanes.set')
  expect(c.apps.map((r) => [r.name, r.exe, r.lane])).toEqual([
    ['Mail', MAIL, 'vpn'],
    ['Browser', BROWSER, 'direct'],
    ['Game', GAME, 'direct']
  ])
  expect(services.store.getState().notice).toBe(
    'Не программа: notes, Документы; Старая игра: это ярлык игры — запустите игру, и она появится в «Сейчас в сети»'
  )
})

test('the VPN lane names the server of the proxy, and says VPN without a name', () => {
  const server = proxyServer('0123456789abcdef', 'edge-a.example.com')
  const { services, view } = setup({ ...board, state: proxyState([server], server.id, [], { apps: board.state!.apps }) }, null)
  renderWith(<Lane vm={view.vpn} onMakeDefault={() => {}} />, services)
  expect(screen.getByText(/^Сайты видят адрес сервера/).textContent).toBe('Сайты видят адрес сервера edge-a.example.com · 198.51.100.7')
  act(() => services.store.getState().setSnapshot({ ...board, state: proxyState([server], null, [], { apps: board.state!.apps }) }))
  expect(screen.getByText(/^Сайты видят адрес сервера/).textContent).toBe('Сайты видят адрес сервера VPN · 198.51.100.7')
})

test('the VPN lane names the exit node remembered while the list is empty, in either language', () => {
  const off = snap({ apps: board.state!.apps, phase: 'off' })
  const st = off.state!
  const remembered: Snapshot = {
    ...off,
    state: { ...st, tailscale: { ...st.tailscale, exitNodes: [], path: null } },
    settings: { ...off.settings, lastExitNode: { id: '100.64.0.2', name: 'node-a' } }
  }
  const { services, view } = setup(remembered, null)
  renderWith(<Lane vm={view.vpn} onMakeDefault={() => {}} />, services)
  expect(screen.getByText(/^Сайты видят адрес сервера/).textContent).toBe('Сайты видят адрес сервера node-a · 198.51.100.7')
  // the board renders again in a new language (its view takes the language); here the lane is rendered anew
  const inEnglish = (s: Snapshot): void => {
    cleanup()
    act(() => services.store.getState().setSnapshot({ ...s, language: 'en' }))
    renderWith(<Lane vm={view.vpn} onMakeDefault={() => {}} />, services)
  }
  inEnglish(remembered)
  expect(screen.getByText(/^Websites see/).textContent).toBe('Websites see the address of node-a · 198.51.100.7')
  inEnglish({ ...remembered, settings: { ...remembered.settings, lastExitNode: null } })
  expect(screen.getByText(/^Websites see/).textContent).toBe('Websites see the address of the VPN server · 198.51.100.7')
})
