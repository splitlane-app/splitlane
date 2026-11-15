import { describe, expect, it } from 'vitest'
import type { Activity, ActivityApp, AppRule, Lane, Traffic } from '../../shared/protocol'
import { setLang, t } from '../i18n/t'
import { boardView, fileName, protectedLane } from './board'

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
        movable: true,
        builtin: false
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
        movable: true,
        builtin: false
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
        movable: true,
        builtin: false
      }
    ])
    expect(board.vpn).toMatchObject({ lane: 'vpn', isDefault: true, down: 1100, up: 30, address: addresses.vpn })
    expect(board.direct).toMatchObject({
      lane: 'direct',
      isDefault: false,
      down: 60,
      up: 6,
      address: addresses.direct,
      unpinned: [],
      builtin: []
    })
  })

  it('unpinned programs sit in the default lane by name, the special tiles last', () => {
    const act = activity([
      unattributedTile({ lane: 'direct', down: 900, connections: 4 }),
      entry(BETA_DIR, { name: 'Beta', exe: `${BETA_DIR}beta.exe`, lane: 'direct', down: 500, connections: 2 }),
      entry(GAMMA_DIR, { name: 'gamma', exe: `${GAMMA_DIR}gamma.exe`, lane: 'direct', down: 10 }),
      entry(ALPHA_DIR, { name: 'Alpha', exe: `${ALPHA_DIR}alpha.exe`, lane: 'direct', down: 4, up: 6 }),
      systemTile({ lane: 'direct', down: 5, connections: 1 })
    ])
    const board = boardView([], 'direct', act, addresses, '')

    expect(board.vpn.unpinned).toEqual([])
    expect(board.direct.unpinned.map((c) => c.key)).toEqual([ALPHA_DIR, BETA_DIR, GAMMA_DIR, 'system', 'unattributed'])
    expect(board.direct.unpinned[1]).toEqual({
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
      movable: true,
      builtin: false
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
      movable: true,
      builtin: false
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

  it('the order of the cards does not follow their traffic from one second to the next', () => {
    const CURL_DIR = 'C:\\Tools\\Curl\\'
    // two copies of one program: the same description, told apart by the file
    const tick = (busy: 'vpn' | 'direct' | 'alpha'): Activity => {
      const speed = (who: typeof busy): number => (who === busy ? 5000 : 1)
      return activity(
        [
          entry(`${CURL_DIR}curl-vpn.exe`, {
            name: 'The curl executable',
            exe: `${CURL_DIR}curl-vpn.exe`,
            down: speed('vpn')
          }),
          entry(`${CURL_DIR}curl-direct.exe`, {
            name: 'The curl executable',
            exe: `${CURL_DIR}curl-direct.exe`,
            down: speed('direct')
          }),
          entry(ALPHA_DIR, { name: 'Alpha', exe: `${ALPHA_DIR}alpha.exe`, down: speed('alpha') }),
          unattributedTile({ down: 2 }),
          systemTile({ down: 3 })
        ].sort((a, b) => b.down - a.down) // the helper's order: the busiest first
      )
    }
    const want = [ALPHA_DIR, `${CURL_DIR}curl-direct.exe`, `${CURL_DIR}curl-vpn.exe`, 'system', 'unattributed']
    for (const busy of ['vpn', 'direct', 'alpha'] as const) {
      expect(boardView([], 'vpn', tick(busy), addresses, '').vpn.unpinned.map((c) => c.key), busy).toEqual(want)
    }
  })

  it('programs sort by the current language, and the tiles take its names', () => {
    const ZOOM_DIR = 'C:\\Apps\\Zoom\\'
    const YA_DIR = 'C:\\Apps\\Ya\\'
    const act = activity([
      entry(ZOOM_DIR, { name: 'Zoom', exe: `${ZOOM_DIR}zoom.exe` }),
      entry(YA_DIR, { name: 'Яндекс', exe: `${YA_DIR}ya.exe` }),
      systemTile()
    ])
    const order = (): string[] => boardView([], 'vpn', act, addresses, '').vpn.unpinned.map((c) => c.name)
    try {
      // the Russian collation puts Cyrillic first, the English one Latin
      setLang('ru')
      expect(order()).toEqual(['Яндекс', 'Zoom', 'Службы Windows', 'WSL и виртуальные машины'])
      setLang('en')
      expect(order()).toEqual(['Zoom', 'Яндекс', 'Windows services', 'WSL and virtual machines'])
    } finally {
      setLang('ru')
    }
  })

  it('a program of a built-in rule sits in its own lane and never moves', () => {
    const TS_DIR = 'C:\\Program Files\\Tailscale\\'
    const TS = `${TS_DIR}tailscaled.exe`
    const SL_DIR = 'C:\\Program Files\\SplitLane\\'
    const ODD_DIR = 'C:\\Tools\\Odd\\'
    const act = activity([
      // the helper puts every program of no rule on its main lane (that of the special tiles): the protected
      // files are known by name; a program the helper reports in the other lane goes by a rule of its own
      entry(TS_DIR, { name: 'Tailscale service', exe: TS, lane: 'vpn', down: 30, connections: 2 }),
      entry(SL_DIR, { name: 'SplitLane helper', exe: `${SL_DIR}splitlane-helper.exe`, lane: 'vpn', down: 3 }),
      entry(ODD_DIR, { name: 'Odd', exe: `${ODD_DIR}odd.exe`, lane: 'direct', down: 7 }),
      entry(EDITOR_DIR, { name: 'Редактор', exe: EDITOR }),
      systemTile(),
      unattributedTile()
    ])
    const board = boardView([], 'vpn', act, addresses, '')

    expect(board.direct.builtin.map((c) => c.key)).toEqual([ODD_DIR, TS_DIR])
    expect(board.direct.builtin[1]).toEqual({
      key: TS_DIR,
      ruleId: null,
      kind: 'app',
      name: 'Tailscale service',
      exe: TS,
      lane: 'direct',
      pinned: false,
      down: 30,
      up: 0,
      connections: 2,
      movable: false,
      builtin: true
    })
    expect(board.vpn.builtin.map((c) => [c.key, c.lane, c.movable, c.builtin])).toEqual([[SL_DIR, 'vpn', false, true]])
    expect(board.vpn.unpinned.map((c) => c.key)).toEqual([EDITOR_DIR, 'system', 'unattributed'])
    expect(board.direct.unpinned).toEqual([])
    expect(boardView([], 'vpn', act, addresses, 'TAILSCALED').direct.builtin.map((c) => c.key)).toEqual([TS_DIR])

    // in «всё напрямую» the helper's main lane is direct, the tiles' too: an unpinned program on it is no built-in one
    const allDirect = activity([
      entry(EDITOR_DIR, { name: 'Редактор', exe: EDITOR, lane: 'direct' }),
      systemTile({ lane: 'direct' }),
      unattributedTile({ lane: 'direct' })
    ])
    const b = boardView([], 'vpn', allDirect, addresses, '')
    expect(b.vpn.unpinned.map((c) => c.key)).toEqual([EDITOR_DIR, 'system', 'unattributed'])
    expect([...b.vpn.builtin, ...b.direct.builtin]).toEqual([])
  })

  it('protectedLane knows the protected files by name, fileName takes the last part of a path', () => {
    expect(protectedLane('C:\\Program Files\\Tailscale\\TailScaled.EXE')).toBe('direct')
    expect(protectedLane('C:\\Program Files\\Tailscale\\tailscale-ipn.exe')).toBe('direct')
    expect(protectedLane('C:\\Program Files\\SplitLane\\splitlane-helper.exe')).toBe('vpn')
    expect(protectedLane('C:\\Program Files\\SplitLane\\sing-box.exe')).toBeNull()
    expect(protectedLane(GAME)).toBeUndefined()
    expect(protectedLane('')).toBeUndefined()
    expect(fileName(GAME)).toBe('game.exe')
    expect(fileName('C:/x/y.exe')).toBe('y.exe')
    expect(fileName('')).toBe('')
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
        movable: true,
        builtin: false
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
        movable: true,
        builtin: false
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
