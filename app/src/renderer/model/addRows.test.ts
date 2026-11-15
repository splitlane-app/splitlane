import { expect, test } from 'vitest'
import { activity, entry, rule } from '../testing'
import { addRowDrop, installedRows, onlineRows, searchRows } from './addRows'
import { boardView } from './board'

const MAIL = 'C:\\Apps\\Mail\\mail.exe'
const GAME_DIR = 'C:\\Games\\Game\\'
const GAME = `${GAME_DIR}game.exe`
const TS_DIR = 'C:\\Program Files\\Tailscale\\'
const ADDRESSES = { vpn: '198.51.100.7', direct: '203.0.113.9' }
const ID1 = '6f1c2a4e-0b7d-4c3a-9e21-5d8f7a1b2c01'

test('onlineRows offers pin and the other lane for programs on the default lane', () => {
  const a = activity([
    entry(GAME_DIR, { name: 'Game', exe: GAME, down: 10 }),
    entry('system', { kind: 'system' }),
    entry('unattributed', { kind: 'unattributed' })
  ])
  const vpnMain = onlineRows(boardView([], 'vpn', a, ADDRESSES, ''))
  expect(vpnMain).toEqual([
    {
      key: GAME_DIR,
      name: 'Game',
      exe: GAME,
      currentLane: 'vpn',
      pinned: false,
      builtin: false,
      actions: ['pin', 'direct']
    }
  ])
  const directMain = onlineRows(boardView([], 'direct', a, ADDRESSES, ''))
  expect(directMain[0].currentLane).toBe('direct')
  expect(directMain[0].actions).toEqual(['pin', 'vpn'])
  expect(onlineRows(boardView([], 'vpn', null, ADDRESSES, ''))).toEqual([])
})

test('onlineRows does not offer the programs of a built-in rule', () => {
  const a = activity([
    entry(TS_DIR, { name: 'Tailscale service', exe: `${TS_DIR}tailscaled.exe`, down: 30 }),
    entry(GAME_DIR, { name: 'Game', exe: GAME, down: 10 }),
    entry('unattributed', { kind: 'unattributed' })
  ])
  expect(onlineRows(boardView([], 'vpn', a, ADDRESSES, '')).map((r) => r.key)).toEqual([GAME_DIR])
})

test('installedRows marks pinned programs', () => {
  const rows = installedRows(
    [
      { exe: MAIL.toUpperCase(), name: 'Mail' },
      { exe: GAME, name: 'Game' }
    ],
    { apps: [rule(ID1, 'Mail', MAIL, 'direct')], defaultLane: 'vpn' }
  )
  expect(rows[0]).toMatchObject({ currentLane: 'direct', pinned: true, builtin: false, actions: ['vpn', 'direct'] })
  expect(rows[1]).toMatchObject({ key: GAME, name: 'Game', exe: GAME, currentLane: null, pinned: false })
  expect(rows[1].actions).toEqual(['vpn', 'direct'])
})

test('installedRows offers no lane to a program of a built-in rule', () => {
  const [row] = installedRows([{ exe: `${TS_DIR}tailscale-ipn.exe`, name: 'Tailscale' }], { apps: [], defaultLane: 'vpn' })
  expect(row).toEqual({
    key: `${TS_DIR}tailscale-ipn.exe`,
    name: 'Tailscale',
    exe: `${TS_DIR}tailscale-ipn.exe`,
    currentLane: 'direct',
    pinned: false,
    builtin: true,
    actions: []
  })
})

test('searchRows keeps the rows whose name, path or file name has the search, ignoring case', () => {
  const rows = installedRows(
    [
      { exe: MAIL, name: 'Почта' },
      { exe: GAME, name: 'Game' },
      { exe: 'D:\\Tools\\curl-vpn.exe', name: 'The curl executable' }
    ],
    { apps: [], defaultLane: 'vpn' }
  )
  const keys = (search: string): string[] => searchRows(rows, search).map((r) => r.exe)
  expect(keys('ПОЧ')).toEqual([MAIL])
  expect(keys('c:\\games')).toEqual([GAME])
  expect(keys('CURL-VPN')).toEqual(['D:\\Tools\\curl-vpn.exe'])
  expect(keys('.exe')).toHaveLength(3)
  expect(searchRows(rows, '  ')).toBe(rows)
  expect(keys('нет такой')).toEqual([])
})

test('addRowDrop takes only the places above the line', () => {
  expect(addRowDrop({ lane: 'direct', area: 'pinned' })).toBe('direct')
  expect(addRowDrop({ lane: 'vpn', area: 'unpinned' })).toBeNull()
})
