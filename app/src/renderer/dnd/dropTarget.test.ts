import { expect, test } from 'vitest'
import type { Lane } from '../../shared/protocol'
import type { CardVM } from '../model/board'
import type { Layout } from '../model/layout'
import { rule } from '../testing'
import { changeOf, dropOp, parseZone, zoneId, type DropOp, type DropZone } from './dropTarget'

const MAIL = 'C:\\Apps\\Mail\\mail.exe'
const BROWSER = 'C:\\Apps\\Browser\\browser.exe'
const GAME = 'C:\\Games\\Game\\game.exe'
const ID1 = '6f1c2a4e-0b7d-4c3a-9e21-5d8f7a1b2c01'
const ID2 = '6f1c2a4e-0b7d-4c3a-9e21-5d8f7a1b2c02'
const ID3 = '6f1c2a4e-0b7d-4c3a-9e21-5d8f7a1b2c03'
const NEW_ID = '6f1c2a4e-0b7d-4c3a-9e21-5d8f7a1b2c09'

function card(over: Partial<CardVM>): CardVM {
  return {
    key: GAME,
    ruleId: null,
    kind: 'app',
    name: 'Game',
    exe: GAME,
    lane: 'vpn',
    pinned: false,
    down: 0,
    up: 0,
    connections: 0,
    movable: true,
    ...over
  }
}

/** The cards of the board, as boardView makes them with defaultLane the default lane. */
function cards(defaultLane: Lane): Record<string, CardVM> {
  return {
    pinnedVpn: card({ key: ID1, ruleId: ID1, name: 'Mail', exe: MAIL, lane: 'vpn', pinned: true }),
    pinnedDirect: card({ key: ID2, ruleId: ID2, name: 'Browser', exe: BROWSER, lane: 'direct', pinned: true }),
    pinnedSystem: card({
      key: ID3,
      ruleId: ID3,
      kind: 'system',
      name: 'Службы Windows',
      exe: '',
      lane: 'direct',
      pinned: true
    }),
    unpinned: card({ lane: defaultLane }),
    systemTile: card({ key: 'system', kind: 'system', name: 'Службы Windows', exe: '', lane: defaultLane }),
    unattributed: card({
      key: 'unattributed',
      kind: 'unattributed',
      name: 'WSL и виртуальные машины',
      exe: '',
      lane: defaultLane,
      movable: false
    })
  }
}

const ZONES: DropZone[] = [
  { lane: 'vpn', area: 'pinned' },
  { lane: 'vpn', area: 'unpinned' },
  { lane: 'direct', area: 'pinned' },
  { lane: 'direct', area: 'unpinned' }
]

const toVpn: DropOp = { op: 'move', lane: 'vpn' }
const toDirect: DropOp = { op: 'move', lane: 'direct' }
const PIN: DropOp = { op: 'pin' }
const UNPIN: DropOp = { op: 'unpin' }

// Spec 9.2, by the zones above in order: vpn above the line, vpn below it, direct above, direct below. Only the
// default lane has the place below the line; a card dropped where it is already changes nothing.
const TABLE: Record<Lane, Record<string, DropOp[]>> = {
  vpn: {
    pinnedVpn: [null, UNPIN, toDirect, null],
    pinnedDirect: [toVpn, UNPIN, null, null],
    pinnedSystem: [toVpn, UNPIN, null, null],
    unpinned: [PIN, null, toDirect, null],
    systemTile: [PIN, null, toDirect, null],
    unattributed: [null, null, null, null]
  },
  direct: {
    pinnedVpn: [null, null, toDirect, UNPIN],
    pinnedDirect: [toVpn, null, null, UNPIN],
    pinnedSystem: [toVpn, null, null, UNPIN],
    unpinned: [toVpn, null, PIN, null],
    systemTile: [toVpn, null, PIN, null],
    unattributed: [null, null, null, null]
  }
}

test('dropOp follows the table of 9.2', () => {
  let checked = 0
  for (const defaultLane of ['vpn', 'direct'] as const) {
    const board = cards(defaultLane)
    for (const [name, c] of Object.entries(board)) {
      ZONES.forEach((zone, i) => {
        expect(dropOp(c, zone, defaultLane), `${name} on ${zoneId(zone)}, default ${defaultLane}`).toEqual(
          TABLE[defaultLane][name][i]
        )
        checked++
      })
    }
  }
  expect(checked).toBe(2 * 6 * 4)
})

test('a zone id names the lane and the area', () => {
  for (const zone of ZONES) expect(parseZone(zoneId(zone))).toEqual(zone)
  expect(zoneId({ lane: 'direct', area: 'unpinned' })).toBe('direct:unpinned')
  for (const id of ['', 'vpn', 'vpn:', 'tor:pinned', 'vpn:pinned:x', 'direct:below']) {
    expect(parseZone(id), id).toBeNull()
  }
})

test('changeOf turns a drop into the layout operation', () => {
  const l: Layout = {
    apps: [rule(ID1, 'Mail', MAIL, 'vpn'), rule(ID2, 'Browser', BROWSER, 'direct')],
    defaultLane: 'vpn'
  }
  const board = cards('vpn')
  const newId = (): string => NEW_ID
  const lanes = (x: Layout): [string, string][] => x.apps.map((r) => [r.exe, r.lane])

  expect(lanes(changeOf(board.pinnedVpn, { op: 'move', lane: 'direct' })(l, newId))).toEqual([
    [MAIL, 'direct'],
    [BROWSER, 'direct']
  ])
  expect(lanes(changeOf(board.unpinned, { op: 'pin' })(l, newId))).toEqual([
    [MAIL, 'vpn'],
    [BROWSER, 'direct'],
    [GAME, 'vpn']
  ])
  expect(lanes(changeOf(board.unpinned, { op: 'move', lane: 'direct' })(l, newId))).toEqual([
    [MAIL, 'vpn'],
    [BROWSER, 'direct'],
    [GAME, 'direct']
  ])
  expect(lanes(changeOf(board.pinnedDirect, { op: 'unpin' })(l, newId))).toEqual([[MAIL, 'vpn']])
  // nothing to unpin on a card of no rule
  expect(changeOf(board.unpinned, { op: 'unpin' })(l, newId)).toBe(l)
})
