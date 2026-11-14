import { describe, expect, it, vi } from 'vitest'
import type { AppRule, HelperState, Lane } from '../../shared/protocol'
import type { CardVM } from './board'
import {
  addPrograms,
  layoutOf,
  makeDefault,
  moveTo,
  newRule,
  pin,
  setWholeFolder,
  unpin,
  type Layout
} from './layout'

const GAME_DIR = 'C:\\Games\\Game\\'
const GAME = `${GAME_DIR}game.exe`
const EDITOR_DIR = 'D:\\Tools\\Editor\\'
const EDITOR = `${EDITOR_DIR}editor.exe`
const NOTEPAD =
  'C:\\Program Files\\WindowsApps\\Microsoft.WindowsNotepad_11.2410.21.0_x64__8wekyb3d8bbwe\\Notepad\\Notepad.exe'

const game: AppRule = { id: 'r1', name: 'Игра', exe: GAME, matcher: { kind: 'folder', value: GAME_DIR }, lane: 'vpn' }
const base: Layout = { apps: [game], defaultLane: 'vpn' }

function card(over: Partial<CardVM>): CardVM {
  return {
    key: 'k',
    ruleId: null,
    kind: 'app',
    name: '',
    exe: '',
    lane: 'vpn',
    pinned: false,
    down: 0,
    up: 0,
    connections: 0,
    movable: true,
    ...over
  }
}

const pinnedGame = card({ key: 'r1', ruleId: 'r1', name: 'Игра', exe: GAME, pinned: true })
const editor = card({ key: EDITOR_DIR, name: 'Редактор', exe: EDITOR })
const systemTile = card({ key: 'system', kind: 'system' })

function ids(...list: string[]): () => string {
  let i = 0
  return () => {
    if (i >= list.length) throw new Error('no more ids')
    return list[i++]
  }
}

const fresh = (exe: string, lane: Lane, id: string, name = ''): AppRule => ({
  id,
  name,
  exe,
  matcher: { kind: 'folder', value: '' },
  lane
})

describe('layout operations', () => {
  it('moveTo changes the lane of a pinned rule and keeps its value', () => {
    const newId = vi.fn(() => 'unused')
    const out = moveTo(base, pinnedGame, 'direct', newId)
    expect(out).toEqual({ apps: [{ ...game, lane: 'direct' }], defaultLane: 'vpn' })
    expect(out.apps[0].matcher).toEqual({ kind: 'folder', value: GAME_DIR })
    expect(newId).not.toHaveBeenCalled()
    expect(base.apps[0].lane).toBe('vpn') // the input is not changed
    expect(moveTo(base, pinnedGame, 'vpn', newId)).toBe(base) // already there
  })

  it('moveTo pins an unpinned program as a new folder rule', () => {
    const out = moveTo(base, editor, 'direct', ids('id-1'))
    // the name is left to the helper: it is not a shortcut name
    expect(out.apps).toEqual([game, fresh(EDITOR, 'direct', 'id-1')])
    expect(out.apps[0]).toBe(game) // the other rules go back unchanged
  })

  it('moveTo pins a Store program as a package rule', () => {
    const out = moveTo(base, card({ key: 'notepad', name: 'Блокнот', exe: NOTEPAD }), 'direct', ids('id-1'))
    expect(out.apps[1]).toEqual({
      id: 'id-1',
      name: '',
      exe: NOTEPAD,
      matcher: { kind: 'package', value: '' },
      lane: 'direct'
    })
  })

  it('moveTo pins the system tile as a system rule', () => {
    const out = moveTo(base, systemTile, 'direct', ids('id-1'))
    expect(out.apps[1]).toEqual({ id: 'id-1', name: '', exe: '', matcher: { kind: 'system', value: '' }, lane: 'direct' })
    // unattributed never moves
    expect(moveTo(base, card({ key: 'unattributed', kind: 'unattributed', movable: false }), 'direct', ids())).toBe(base)
  })

  it('pin uses the default lane', () => {
    const direct: Layout = { apps: [game], defaultLane: 'direct' }
    expect(pin(direct, editor, ids('id-1')).apps[1]).toEqual(fresh(EDITOR, 'direct', 'id-1'))
    expect(pin(base, systemTile, ids('id-2')).apps[1]).toMatchObject({ id: 'id-2', lane: 'vpn' })
    expect(pin(base, pinnedGame, ids())).toBe(base) // already pinned
  })

  it('unpin removes the rule', () => {
    const two: Layout = { apps: [game, fresh(EDITOR, 'direct', 'id-1')], defaultLane: 'vpn' }
    expect(unpin(two, 'r1')).toEqual({ apps: [fresh(EDITOR, 'direct', 'id-1')], defaultLane: 'vpn' })
    expect(unpin(base, 'missing')).toBe(base)
  })

  it('makeDefault keeps pinned rules', () => {
    const out = makeDefault(base, 'direct')
    expect(out).toEqual({ apps: [game], defaultLane: 'direct' })
    expect(out.apps[0]).toBe(game)
    expect(makeDefault(base, 'vpn')).toBe(base)
  })

  it('setWholeFolder switches folder and file with an empty value', () => {
    const asFile = setWholeFolder(base, 'r1', false)
    expect(asFile.apps).toEqual([{ ...game, matcher: { kind: 'file', value: '' } }])

    const helperFile: Layout = { apps: [{ ...game, matcher: { kind: 'file', value: GAME } }], defaultLane: 'vpn' }
    expect(setWholeFolder(helperFile, 'r1', true).apps).toEqual([{ ...game, matcher: { kind: 'folder', value: '' } }])
    // the kind it has already: the helper's value stays
    expect(setWholeFolder(helperFile, 'r1', false)).toBe(helperFile)
    // package and system rules have no folder to switch
    const store: Layout = {
      apps: [{ id: 'p', name: 'Блокнот', exe: NOTEPAD, matcher: { kind: 'package', value: 'x' }, lane: 'vpn' }],
      defaultLane: 'vpn'
    }
    expect(setWholeFolder(store, 'p', false)).toBe(store)
    expect(setWholeFolder(base, 'missing', false)).toBe(base)
  })

  it('addPrograms moves an already pinned program instead of duplicating', () => {
    const out = addPrograms(
      base,
      [
        { exe: GAME.toUpperCase(), name: 'Игра (ярлык)' },
        { exe: EDITOR, name: 'Редактор' },
        { exe: EDITOR, name: 'Редактор' }
      ],
      'direct',
      ids('id-1', 'id-2', 'id-3')
    )
    expect(out.apps).toEqual([{ ...game, lane: 'direct' }, fresh(EDITOR, 'direct', 'id-2', 'Редактор')])
  })

  it('a repeated operation updates the rule of its id instead of adding another', () => {
    const once = moveTo(base, editor, 'direct', ids('id-1'))
    expect(moveTo(once, editor, 'direct', ids('id-1'))).toBe(once)

    // the helper normalized the rule (folder to file, the name filled): its version stays, only the lane is the op's
    const normalized: AppRule = {
      id: 'id-1',
      name: 'Редактор',
      exe: EDITOR,
      matcher: { kind: 'file', value: EDITOR },
      lane: 'vpn'
    }
    const helper: Layout = { apps: [game, normalized], defaultLane: 'vpn' }
    expect(moveTo(helper, editor, 'direct', ids('id-1')).apps).toEqual([game, { ...normalized, lane: 'direct' }])
    expect(addPrograms(helper, [{ exe: EDITOR, name: 'x' }], 'direct', ids('id-1')).apps).toEqual([
      game,
      { ...normalized, lane: 'direct' }
    ])
    // the system rule of the op is found by its id as well
    const sys = moveTo(base, systemTile, 'direct', ids('id-9'))
    expect(moveTo(sys, systemTile, 'vpn', ids('id-9')).apps).toHaveLength(2)
  })

  it('newRule keeps the limits of the helper', () => {
    expect(newRule({ exe: EDITOR, name: 'Редактор' }, 'vpn', 'id-1')).toEqual(fresh(EDITOR, 'vpn', 'id-1', 'Редактор'))
    const long = newRule({ exe: EDITOR, name: 'я'.repeat(200) }, 'vpn', 'id-1')
    expect(new TextEncoder().encode(long.name).length).toBe(256)

    const full: Layout = {
      apps: Array.from({ length: 500 }, (_, i) => fresh(`C:\\P${i}\\p.exe`, 'vpn', `id-${i}`)),
      defaultLane: 'vpn'
    }
    expect(addPrograms(full, [{ exe: EDITOR, name: '' }], 'vpn', ids('id-x'))).toBe(full)
    const tooLong = `C:\\${'a'.repeat(1100)}\\a.exe`
    expect(addPrograms(base, [{ exe: tooLong, name: '' }], 'vpn', ids('id-x'))).toBe(base)
  })

  it('layoutOf takes the rules and the default lane of the state', () => {
    const state = { apps: [game], defaultLane: 'direct' } as unknown as HelperState
    expect(layoutOf(state)).toEqual({ apps: [game], defaultLane: 'direct' })
  })
})
