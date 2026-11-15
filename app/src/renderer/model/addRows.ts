import type { Program } from '../../shared/programs'
import type { Lane } from '../../shared/protocol'
import type { DropZone } from '../dnd/dropTarget'
import { matchesSearch, protectedLane, type boardView } from './board'
import type { Layout } from './layout'

/** One row of the add panel: a program with the buttons it offers. */
export interface AddRow {
  key: string
  name: string
  exe: string
  /** The lane the program goes by now (a pinned one: its rule's); null: not seen anywhere yet. */
  currentLane: Lane | null
  pinned: boolean
  /** A built-in rule of the helper routes it (spec 5.6): it offers no lane. */
  builtin: boolean
  actions: ('pin' | 'vpn' | 'direct')[]
}

const other = (lane: Lane): 'vpn' | 'direct' => (lane === 'vpn' ? 'direct' : 'vpn')

/**
 * «Сейчас в сети»: the unpinned programs of the live activity (not the special tiles, not the programs of a built-in
 * rule) with their lane; they go by the default lane, so each offers «Закрепить» there and the button of the other
 * lane.
 */
export function onlineRows(board: ReturnType<typeof boardView>): AddRow[] {
  const main = board.vpn.isDefault ? board.vpn : board.direct
  return main.unpinned
    .filter((c) => c.kind === 'app')
    .map((c) => ({
      key: c.key,
      name: c.name,
      exe: c.exe,
      currentLane: main.lane,
      pinned: false,
      builtin: false,
      actions: ['pin', other(main.lane)]
    }))
}

/**
 * «Установленные»: a program pinned already (by its file, ignoring case) shows its lane; both lanes are offered. A
 * protected program (the system Tailscale) goes by its built-in rule and offers none: the helper would reject it.
 */
export function installedRows(programs: Program[], layout: Layout): AddRow[] {
  return programs.map((p) => {
    const fixed = protectedLane(p.exe)
    if (fixed !== undefined) {
      return { key: p.exe, name: p.name, exe: p.exe, currentLane: fixed, pinned: false, builtin: true, actions: [] }
    }
    const rule = layout.apps.find((r) => r.matcher.kind !== 'system' && r.exe.toLowerCase() === p.exe.toLowerCase())
    return {
      key: p.exe,
      name: p.name,
      exe: p.exe,
      currentLane: rule?.lane ?? null,
      pinned: rule !== undefined,
      builtin: false,
      actions: ['vpn', 'direct']
    }
  })
}

/** The rows of the header's search: by name or path (the file name with it), ignoring case; all of them without one. */
export function searchRows(rows: AddRow[], search: string): AddRow[] {
  if (search.trim() === '') return rows
  return rows.filter((r) => matchesSearch(r.name, r.exe, search))
}

/** The lane a row of the add panel dropped on a zone is pinned in; null where it cannot be dropped. */
export function addRowDrop(zone: DropZone): Lane | null {
  return zone.area === 'pinned' ? zone.lane : null
}
