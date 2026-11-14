import { CollisionPriority, CollisionType, configure, type CollisionDetector } from '@dnd-kit/abstract'
import {
  Accessibility,
  defaultPreset,
  Feedback,
  KeyboardSensor,
  PointerActivationConstraints,
  PointerSensor,
  type DragDropManager,
  type DragEndEvent,
  type Draggable,
  type DropAnimationFunction,
  type Sensors
} from '@dnd-kit/dom'
import { DragDropProvider, useDragOperation, useDroppable } from '@dnd-kit/react'
import { useMemo, type ReactNode } from 'react'
import type { StoreApi } from 'zustand'
import type { Program } from '../../shared/programs'
import type { Lane } from '../../shared/protocol'
import { t } from '../i18n/t'
import { addRowDrop } from '../model/addRows'
import type { CardVM } from '../model/board'
import { addPrograms } from '../model/layout'
import { useServices } from '../services'
import { effectiveLayout, type AppState, type LayoutChange } from '../store/appStore'
import { changeOf, dropOp, opLabel, parseZone, zoneId, type DropOp, type DropZone } from './dropTarget'

/** The type of the draggables of the cards of the board; their data is the card (CardVM). */
export const CARD = 'card'

/** The type of the rows of the add panel that are programs of the Start menu or a picked file; their data is the Program. */
export const ADD_ROW = 'add-row'

/**
 * How a card is dragged. With the mouse: after a move of 5 pixels, so that a click (and a slow click) still opens
 * the program card. A finger holds a quarter of a second first, as dnd-kit does by default. From the keyboard:
 * Space takes the card (Enter opens the program card), the arrows move it, Space, Enter or Tab put it down,
 * Escape cancels.
 */
export const CARD_SENSORS: Sensors = [
  PointerSensor.configure({
    activationConstraints: (event) =>
      event.pointerType === 'touch'
        ? [new PointerActivationConstraints.Delay({ value: 250, tolerance: 5 })]
        : [new PointerActivationConstraints.Distance({ value: 5 })]
  }),
  KeyboardSensor.configure({ keyboardCodes: { ...KeyboardSensor.defaults.keyboardCodes, start: ['Space'] } })
]

type AccessibilityOptions = NonNullable<ConstructorParameters<typeof Accessibility>[1]>
type Operation = DragEndEvent['operation']

/** The id that the probe of a change hands out for the rules it would add (the probe is never sent). */
const PROBE_ID = 'probe'

/** The card a drag operation moves, when it is a card of the board. */
function draggedCard(operation: { readonly source: Draggable | null | undefined }): CardVM | null {
  const { source } = operation
  return source != null && source.type === CARD ? (source.data as CardVM) : null
}

/** The program of an add-panel row (not an online one: those are cards) a drag operation moves. */
function draggedProgram(operation: { readonly source: Draggable | null | undefined }): Program | null {
  const { source } = operation
  return source != null && source.type === ADD_ROW ? (source.data as Program) : null
}

/** The lane a row of the add panel dropped on the operation's target is pinned in. */
function addRowLane(operation: Operation): Lane | null {
  const zone = operation.target != null ? parseZone(String(operation.target.id)) : null
  return zone === null ? null : addRowDrop(zone)
}

/** The card of the operation and what dropping it on the operation's target does, by the layout on screen. */
function dropOf(store: StoreApi<AppState>, operation: Operation): { card: CardVM; op: DropOp } | null {
  const card = draggedCard(operation)
  if (card === null) return null
  const zone = operation.target != null ? parseZone(String(operation.target.id)) : null
  const layout = effectiveLayout(store.getState())
  return { card, op: zone !== null && layout !== null ? dropOp(card, zone, layout.defaultLane) : null }
}

/** The words a screen reader says while a card is dragged: what letting it go there would do. */
function announcements(store: StoreApi<AppState>): AccessibilityOptions['announcements'] {
  const said = (operation: Operation): string | undefined => {
    const program = draggedProgram(operation)
    if (program !== null) {
      const lane = addRowLane(operation)
      return lane === null
        ? t('dnd.stay', { name: program.name })
        : t('dnd.addAction', { name: program.name, lane: t(lane === 'vpn' ? 'lane.vpn' : 'lane.direct') })
    }
    const drop = dropOf(store, operation)
    if (drop === null) return undefined
    const { card, op } = drop
    return op === null ? t('dnd.stay', { name: card.name }) : t('dnd.action', { name: card.name, action: opLabel(op) })
  }
  return {
    dragstart: ({ operation }) => {
      const name = (draggedCard(operation) ?? draggedProgram(operation))?.name
      return name === undefined ? undefined : t('dnd.picked', { name })
    },
    dragover: ({ operation }) => said(operation),
    dragend: ({ operation, canceled }) => {
      const name = (draggedCard(operation) ?? draggedProgram(operation))?.name
      if (name === undefined) return undefined
      return canceled ? t('dnd.canceled', { name }) : said(operation)
    }
  }
}

/**
 * A card that moves somewhere else is drawn there by the next render, and its old element is gone: no animation
 * to fly back to it (dnd-kit then still gives the focus back to the card of a keyboard drag).
 */
const STAY_WHERE_DROPPED: DropAnimationFunction = () => {}

/**
 * Dragging the cards of the board (spec 9.2) with @dnd-kit/react: a card dropped on a zone of a lane makes the
 * change of dropOp, shown at once and rolled back by the store on an error. A drop that changes nothing (or a
 * cancelled drag) lets the card fly back to its place.
 */
export function BoardDnd({ children }: { children: ReactNode }): React.JSX.Element {
  const { store } = useServices()
  const plugins = useMemo(
    () =>
      defaultPreset.plugins.map((p) =>
        p === Accessibility
          ? configure(Accessibility, {
              announcements: announcements(store),
              screenReaderInstructions: { draggable: t('dnd.instructions') }
            } satisfies AccessibilityOptions)
          : p
      ),
    [store]
  )

  const onDragEnd = (event: DragEndEvent, manager: DragDropManager): void => {
    const drop = event.canceled ? null : dropOf(store, event.operation)
    let change: LayoutChange | null = drop?.op != null ? changeOf(drop.card, drop.op) : null
    const program = event.canceled ? null : draggedProgram(event.operation)
    const lane = program !== null ? addRowLane(event.operation) : null
    if (program !== null && lane !== null) change = (l, newId) => addPrograms(l, [program], lane, newId)
    const layout = effectiveLayout(store.getState())
    // does it change anything? (the ids of the rules it would add do not matter for that)
    const moves = change !== null && layout !== null && change(layout, () => PROBE_ID) !== layout
    const feedback = manager.registry.plugins.get(Feedback)
    if (feedback !== undefined) feedback.dropAnimation = moves ? STAY_WHERE_DROPPED : undefined
    if (moves && change !== null) void store.getState().change(change)
  }

  return (
    <DragDropProvider sensors={CARD_SENSORS} plugins={plugins} onDragEnd={onDragEnd}>
      {children}
    </DragDropProvider>
  )
}

/**
 * The place below the line takes a card only while the pointer (or, for a keyboard drag, the card's centre) is in
 * it: a card held above the line that only touches it with its edge stays above.
 */
const pointerInside: CollisionDetector = ({ dragOperation, droppable }) => {
  const at = dragOperation.position.current
  const { shape } = droppable
  if (shape === undefined || !shape.containsPoint(at)) return null
  const distance = Math.hypot(shape.center.x - at.x, shape.center.y - at.y)
  return {
    id: droppable.id,
    value: 1 / Math.max(distance, 1),
    type: CollisionType.PointerIntersection,
    priority: CollisionPriority.Highest
  }
}

/**
 * A zone of a lane to drop cards on; `over` while a card that would change there is dragged over it. The zone
 * below the line lies inside the lane (the zone above it) and wins over it while the pointer is in it; the lane
 * keeps dnd-kit's own detection (the pointer first, else the most overlap).
 */
export function useDropZone(
  zone: DropZone,
  defaultLane: Lane,
  disabled = false
): { ref: (element: Element | null) => void; over: boolean } {
  const below = zone.area === 'unpinned'
  const { ref, isDropTarget } = useDroppable({
    id: zoneId(zone),
    data: zone,
    disabled,
    ...(below ? { collisionDetector: pointerInside, collisionPriority: CollisionPriority.Highest } : {})
  })
  const operation = useDragOperation()
  const card = draggedCard(operation)
  const over =
    isDropTarget &&
    ((card !== null && dropOp(card, zone, defaultLane) !== null) ||
      (draggedProgram(operation) !== null && addRowDrop(zone) !== null))
  return { ref, over }
}
