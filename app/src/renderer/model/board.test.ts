import { describe, expect, it } from 'vitest'
import type { Activity, ActivityApp, AppRule, Lane, Traffic } from '../../shared/protocol'
import { t } from '../i18n/t'
import { boardView } from './board'

const GAME = String.raw`C:\Games\Game\game.exe`
const BROWSER = String.raw`C:\Apps\Browser\browser.exe`
const MAIL = String.raw`C:\Apps\Mail\mail.exe`
// unpinned keys are program folders with a trailing backslash
const EDITOR_DIR = 'D:\\Tools\\Editor\\'
const EDITOR = `${EDITOR_DIR}editor.exe`
const ALPHA_DIR = 'C:\\Tools\\Alpha\\'
const BETA_DIR = 'C:\\Tools\\Beta\\'
const GAMMA_DIR = 'C:\\Tools\\Gamma\\'

const addresses = { vpn: '198.51.100.7', direct: '203.0.113.9' }
const quiet: Traffic = { down: 0, up: 0 }

function rule(id: string, name: string, exe: string, lane: Lane, over: Partial<AppRule> = {}): AppRule {
  return { id, name, exe, matcher: { kind: 'folder', value: '' }, lane, ...over }
}

function entry(key: string, over: Partial<ActivityApp> = {}): ActivityApp {
  return { key, kind: 'app', name: '', exe: '', ruleId: null, lane: 'vpn', down: 0, up: 0, connections: 0, ...over }
}

const systemTile = (over: Partial<ActivityApp> = {}): ActivityApp => entry('system', { kind: 'system', ...over })
const unattributedTile = (over: Partial<ActivityApp> = {}): ActivityApp =>
  entry('unattributed', { kind: 'unattributed', ...over })

function activity(apps: ActivityApp[], lanes: Activity['lanes'] = { vpn: quiet, direct: quiet }): Activity {
  return { lanes, apps }
}

describe('boardView', () => {
  it('pinned cards come from rules with speeds by ruleId', () => {
    const apps = [rule('r1', 'Игра', GAME, 'vpn'), rule('r2', 'Браузер', BROWSER, 'direct'), rule('r3', 'Почта', MAIL, 'vpn')]
    const act = activity(
      [
        entry('r2', { ruleId: 'r2', name: 'Браузер', exe: BROWSER, lane: 'direct', down: 50, up: 5, connections: 1 }),
        entry('r1', { ruleId: 'r1', name: 'Игра', exe: GAME, down: 1000, up: 24, connections: 3 }),
        systemTile(),
        unattributedTile()
      ],
      { vpn: { down: 1100, up: 30 }, direct: { down: 60, up: 6 } }
    )
    const board = boardView(apps, 'vpn', act, addresses, '')

    expect(board.vpn.pinned).toEqual([
      {
        key: 'r1',
        ruleId: 'r1',
        kind: 'app',
        name: 'Игра',
        exe: GAME,
        lane: 'vpn',
        pinned: true,
        down: 1000,
        up: 24,
        connections: 3,
        movable: true
      },
      {
        key: 'r3',
        ruleId: 'r3',
        kind: 'app',
        name: 'Почта',
        exe: MAIL,
        lane: 'vpn',
        pinned: true,
        down: 0,
        up: 0,
        connections: 0,
        movable: true
      }
    ])
    expect(board.direct.pinned).toEqual([
      {
        key: 'r2',
        ruleId: 'r2',
        kind: 'app',
        name: 'Браузер',
        exe: BROWSER,
        lane: 'direct',
        pinned: true,
        down: 50,
        up: 5,
        connections: 1,
        movable: true
      }
    ])
    expect(board.vpn).toMatchObject({ lane: 'vpn', isDefault: true, down: 1100, up: 30, address: addresses.vpn })
    expect(board.direct).toMatchObject({
      lane: 'direct',
      isDefault: false,
      down: 60,
      up: 6,
      address: addresses.direct,
      unpinned: []
    })
  })

  it('unpinned programs and special tiles sit in the default lane', () => {
    const act = activity([
      unattributedTile({ down: 900, connections: 4 }),
      entry(BETA_DIR, { name: 'Beta', exe: `${BETA_DIR}beta.exe`, lane: 'direct', down: 500, connections: 2 }),
      entry(GAMMA_DIR, { name: 'Gamma', exe: `${GAMMA_DIR}gamma.exe`, lane: 'direct', down: 10 }),
      entry(ALPHA_DIR, { name: 'Alpha', exe: `${ALPHA_DIR}alpha.exe`, lane: 'direct', down: 4, up: 6 }),
      systemTile({ down: 5, connections: 1 })
    ])
    const board = boardView([], 'direct', act, addresses, '')

    expect(board.vpn.unpinned).toEqual([])
    expect(board.direct.unpinned.map((c) => c.key)).toEqual([BETA_DIR, ALPHA_DIR, GAMMA_DIR, 'system', 'unattributed'])
    expect(board.direct.unpinned[0]).toEqual({
      key: BETA_DIR,
      ruleId: null,
      kind: 'app',
      name: 'Beta',
      exe: `${BETA_DIR}beta.exe`,
      lane: 'direct',
      pinned: false,
      down: 500,
      up: 0,
      connections: 2,
      movable: true
    })
    expect(board.direct.unpinned[3]).toEqual({
      key: 'system',
      ruleId: null,
      kind: 'system',
      name: t('tile.system'),
      exe: '',
      lane: 'direct',
      pinned: false,
      down: 5,
      up: 0,
      connections: 1,
      movable: true
    })
    expect(board.direct.unpinned[4]).toMatchObject({
      key: 'unattributed',
      kind: 'unattributed',
      name: t('tile.unattributed'),
      lane: 'direct',
      pinned: false,
      down: 900
    })

    // without activity (routing is off) the special tiles are still there, quiet
    const off = boardView([], 'vpn', null, { vpn: null, direct: null }, '')
    expect(off.vpn.unpinned.map((c) => [c.key, c.down + c.up])).toEqual([
      ['system', 0],
      ['unattributed', 0]
    ])
    expect(off.direct.unpinned).toEqual([])
    expect(off.vpn).toMatchObject({ down: 0, up: 0, address: null })
  })

  it('the system tile disappears while a system rule is pinned', () => {
    const sys = rule('rs', '', '', 'direct', { matcher: { kind: 'system', value: '' } })
    const act = activity([
      entry('rs', { kind: 'system', ruleId: 'rs', name: 'Host Process', lane: 'direct', down: 70 }),
      unattributedTile()
    ])
    const board = boardView([sys], 'vpn', act, addresses, '')
    expect(board.direct.pinned).toEqual([
      {
        key: 'rs',
        ruleId: 'rs',
        kind: 'system',
        name: t('tile.system'),
        exe: '',
        lane: 'direct',
        pinned: true,
        down: 70,
        up: 0,
        connections: 0,
        movable: true
      }
    ])
    expect(board.vpn.unpinned.map((c) => c.key)).toEqual(['unattributed'])

    // the rule is on its way to the helper: the activity still has the tile, the board shows it once, pinned
    const pending = boardView([sys], 'vpn', activity([systemTile({ down: 40 }), unattributedTile()]), addresses, '')
    expect(pending.direct.pinned.map((c) => [c.key, c.kind, c.down])).toEqual([['rs', 'system', 40]])
    expect(pending.vpn.unpinned.map((c) => c.key)).toEqual(['unattributed'])
  })

  it('unattributed is not movable', () => {
    const act = activity([
      entry(EDITOR_DIR, { name: 'Редактор', exe: EDITOR }),
      systemTile(),
      unattributedTile({ down: 100 })
    ])
    const board = boardView([rule('r1', 'Игра', GAME, 'direct')], 'vpn', act, addresses, '')
    const cards = [...board.vpn.pinned, ...board.vpn.unpinned, ...board.direct.pinned, ...board.direct.unpinned]
    expect(cards.map((c) => [c.key, c.movable])).toEqual([
      [EDITOR_DIR, true],
      ['system', true],
      ['unattributed', false],
      ['r1', true]
    ])
  })

  it('search filters by name and path', () => {
    const apps = [rule('r1', 'Игра', GAME, 'vpn'), rule('r2', 'Браузер', BROWSER, 'direct')]
    const act = activity([entry(EDITOR_DIR, { name: 'Редактор', exe: EDITOR }), systemTile(), unattributedTile()])
    const keys = (search: string): string[] => {
      const b = boardView(apps, 'vpn', act, addresses, search)
      return [...b.vpn.pinned, ...b.vpn.unpinned, ...b.direct.pinned, ...b.direct.unpinned].map((c) => c.key)
    }
    expect(keys('игРА')).toEqual(['r1'])
    expect(keys('browser.EXE')).toEqual(['r2'])
    expect(keys('d:\\tools')).toEqual([EDITOR_DIR])
    expect(keys('службы')).toEqual(['system'])
    expect(keys('  ')).toEqual(['r1', EDITOR_DIR, 'system', 'unattributed', 'r2'])
    expect(keys('нет такой')).toEqual([])
  })

  it('a program pinned by a rule still on its way is shown once, with its name', () => {
    // the window sent a new rule without a name; the activity still groups the program by its folder
    const fresh = rule('id-1', '', EDITOR, 'direct')
    const act = activity([
      entry(EDITOR_DIR, { name: 'Редактор', exe: EDITOR.toUpperCase(), down: 300, connections: 2 }),
      systemTile(),
      unattributedTile()
    ])
    const board = boardView([fresh], 'vpn', act, addresses, '')
    expect(board.direct.pinned).toEqual([
      {
        key: 'id-1',
        ruleId: 'id-1',
        kind: 'app',
        name: 'Редактор',
        exe: EDITOR,
        lane: 'direct',
        pinned: true,
        down: 300,
        up: 0,
        connections: 2,
        movable: true
      }
    ])
    expect(board.vpn.unpinned.map((c) => c.key)).toEqual(['system', 'unattributed'])

    // no activity at all: the name is the file name without its extension
    expect(boardView([fresh], 'vpn', null, addresses, '').direct.pinned[0].name).toBe('editor')
  })

  it('a program whose rule is gone stays on the board unpinned until the helper regroups it', () => {
    const act = activity([entry('r1', { ruleId: 'r1', name: 'Игра', exe: GAME, down: 20 }), unattributedTile()])
    const board = boardView([], 'direct', act, addresses, '')
    expect(board.direct.pinned).toEqual([])
    expect(board.direct.unpinned.map((c) => [c.key, c.ruleId, c.pinned, c.name])).toEqual([
      ['r1', null, false, 'Игра'],
      ['system', null, false, t('tile.system')],
      ['unattributed', null, false, t('tile.unattributed')]
    ])
  })
})
