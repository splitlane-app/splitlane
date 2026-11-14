import type { Lane } from '../../shared/protocol'
import { t } from '../i18n/t'
import type { CardVM } from '../model/board'
import { moveTo, pin, unpin } from '../model/layout'
import type { LayoutChange } from '../store/appStore'

/** A place to drop a card on: above the line of a lane (its pinned cards) or below it (only in the default lane). */
export type DropZone = { lane: Lane; area: 'pinned' | 'unpinned' }

/** What a drop (or an item of the context menu) does to the card; null: nothing. */
export type DropOp = { op: 'move'; lane: Lane } | { op: 'pin' } | { op: 'unpin' } | null

export const otherLane = (lane: Lane): Lane => (lane === 'vpn' ? 'direct' : 'vpn')

/** The id of the droppable of a zone: "<lane>:<area>". */
export function zoneId(zone: DropZone): string {
  return `${zone.lane}:${zone.area}`
}

export function parseZone(id: string): DropZone | null {
  const [lane, area, ...rest] = id.split(':')
  if (rest.length > 0 || (lane !== 'vpn' && lane !== 'direct') || (area !== 'pinned' && area !== 'unpinned')) {
    return null
  }
  return { lane, area }
}

/**
 * The table of spec 9.2: a card dropped in the other lane is pinned there; an unpinned one dropped above the line
 * is pinned in the default lane; a pinned one dropped below the line is unpinned. A card dropped where it is
 * already, on a place the board does not have (below the line of a lane that is not the default one) or the tile
 * that never moves (WSL and virtual machines) — nothing. An unpinned card goes by the default lane.
 */
export function dropOp(card: CardVM, zone: DropZone, defaultLane: Lane): DropOp {
  if (card.kind === 'unattributed' || (!card.pinned && !card.movable)) return null
  if (zone.area === 'unpinned') {
    return zone.lane === defaultLane && card.pinned ? { op: 'unpin' } : null
  }
  const lane = card.pinned ? card.lane : defaultLane
  if (zone.lane !== lane) return { op: 'move', lane: zone.lane }
  return card.pinned ? null : { op: 'pin' }
}

/** The change of the layout that an op makes on the card. */
export function changeOf(card: CardVM, op: NonNullable<DropOp>): LayoutChange {
  switch (op.op) {
    case 'move':
      return (l, newId) => moveTo(l, card, op.lane, newId)
    case 'pin':
      return (l, newId) => pin(l, card, newId)
    case 'unpin':
      return (l) => (card.ruleId !== null ? unpin(l, card.ruleId) : l)
  }
}

/** The words of an op, the same as the items of the context menu: «Пустить напрямую», «Закрепить»… */
export function opLabel(op: NonNullable<DropOp>): string {
  if (op.op === 'move') return t(op.lane === 'vpn' ? 'move.vpn' : 'move.direct')
  return t(op.op)
}
