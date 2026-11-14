import type { Program } from '../../shared/programs'
import type { Lane } from '../../shared/protocol'
import type { DropZone } from '../dnd/dropTarget'
import type { boardView } from './board'
import type { Layout } from './layout'

/** One row of the add panel: a program with the buttons it offers. */
export interface AddRow {
  key: string
  name: string
  exe: string
  /** The lane the program goes by now (a pinned one: its rule's); null: not seen anywhere yet. */
  currentLane: Lane | null
  pinned: boolean
  actions: ('pin' | 'vpn' | 'direct')[]
}

const other = (lane: Lane): 'vpn' | 'direct' => (lane === 'vpn' ? 'direct' : 'vpn')

/**
 * «Сейчас в сети»: the unpinned programs of the live activity (not the special tiles) with their lane; they go by
 * the default lane, so each offers «Закрепить» there and the button of the other lane.
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
      actions: ['pin', other(main.lane)]
    }))
}

/** «Установленные»: a program pinned already (by its file, ignoring case) shows its lane; both lanes are offered. */
export function installedRows(programs: Program[], layout: Layout): AddRow[] {
  return programs.map((p) => {
    const rule = layout.apps.find((r) => r.matcher.kind !== 'system' && r.exe.toLowerCase() === p.exe.toLowerCase())
    return {
      key: p.exe,
      name: p.name,
      exe: p.exe,
      currentLane: rule?.lane ?? null,
      pinned: rule !== undefined,
      actions: ['vpn', 'direct']
    }
  })
}

/** The lane a row of the add panel dropped on a zone is pinned in; null where it cannot be dropped. */
export function addRowDrop(zone: DropZone): Lane | null {
  return zone.area === 'pinned' ? zone.lane : null
}
