import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type FocusEvent,
  type KeyboardEvent,
  type MouseEvent
} from 'react'
import { createPortal } from 'react-dom'
import { changeOf, opLabel, otherLane, type DropOp } from '../dnd/dropTarget'
import { PinIcon } from '../icons/builtin'
import { t } from '../i18n/t'
import type { CardVM } from '../model/board'
import { useServices } from '../services'
import styles from './ContextMenu.module.css'

/** Where a menu opens: the pointer of a right click, or under the card for the keyboard (client pixels). */
export interface MenuAnchor {
  x: number
  y: number
}

/**
 * The items of a card's menu (spec 9.2): the move to the other lane — only the one that changes something; an
 * unpinned program goes by the default lane, its card's lane — then «Открепить» for a pinned card, «Закрепить»
 * for an unpinned one. The tile of WSL and virtual machines never moves: no items, no menu.
 */
export function menuOps(card: CardVM): NonNullable<DropOp>[] {
  if (card.kind === 'unattributed' || (!card.pinned && !card.movable)) return []
  return [{ op: 'move', lane: otherLane(card.lane) }, card.pinned ? { op: 'unpin' } : { op: 'pin' }]
}

/** The menu keeps this far from the edges of the window. */
const EDGE = 8

/**
 * The context menu of a card: at the anchor, kept inside the window, the first item focused. The arrows (and
 * Home, End) move the focus over the items; an item does its change at once. Escape, Tab, an item, a scroll or a
 * resize close it and give the focus back to the card (onClose(true)); a press outside, the focus going out of
 * the menu or leaving the window close it and leave the focus where it went (onClose(false)).
 */
export function ContextMenu({
  card,
  at,
  onClose
}: {
  card: CardVM
  at: MenuAnchor
  onClose(returnFocus: boolean): void
}): React.JSX.Element {
  const { store } = useServices()
  const ref = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState(at)
  const ops = menuOps(card)

  useLayoutEffect(() => {
    const el = ref.current
    if (el === null) return
    const { width, height } = el.getBoundingClientRect()
    const x = Math.max(EDGE, Math.min(at.x, window.innerWidth - width - EDGE))
    const y = at.y + height > window.innerHeight - EDGE ? Math.max(EDGE, at.y - height) : at.y
    setPos((p) => (p.x === x && p.y === y ? p : { x, y }))
  }, [at])

  useEffect(() => {
    ref.current?.querySelector<HTMLElement>('[role="menuitem"]')?.focus()
  }, [])

  useEffect(() => {
    const outside = (e: Event): void => {
      if (!(e.target instanceof Node) || !(ref.current?.contains(e.target) ?? false)) onClose(false)
    }
    const away = (): void => onClose(true)
    const left = (): void => onClose(false)
    document.addEventListener('pointerdown', outside, true)
    window.addEventListener('scroll', away, true)
    window.addEventListener('resize', away)
    window.addEventListener('blur', left)
    return () => {
      document.removeEventListener('pointerdown', outside, true)
      window.removeEventListener('scroll', away, true)
      window.removeEventListener('resize', away)
      window.removeEventListener('blur', left)
    }
  }, [onClose])

  const choose = (op: NonNullable<DropOp>): void => {
    void store.getState().change(changeOf(card, op))
    onClose(true)
  }

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>): void => {
    const items = Array.from(ref.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])
    const i = items.findIndex((item) => item === document.activeElement)
    const focus = (n: number): void => items[(n + items.length) % items.length]?.focus()
    switch (e.key) {
      case 'ArrowDown':
        focus(i + 1)
        break
      case 'ArrowUp':
        focus(i < 0 ? -1 : i - 1)
        break
      case 'Home':
        focus(0)
        break
      case 'End':
        focus(-1)
        break
      case 'Escape':
      case 'Tab':
        onClose(true)
        break
      case 'ContextMenu':
      case 'F10':
        break
      default:
        return
    }
    e.preventDefault()
    e.stopPropagation()
  }

  // a context menu of the menu itself (the menu key, a right click on it) is not a new menu of the card
  const onContextMenu = (e: MouseEvent): void => {
    e.preventDefault()
    e.stopPropagation()
  }
  // the focus went elsewhere (another card, a click in the window): the menu goes too
  const onBlur = (e: FocusEvent<HTMLDivElement>): void => {
    if (!(e.relatedTarget instanceof Node && e.currentTarget.contains(e.relatedTarget))) onClose(false)
  }

  return createPortal(
    <div
      ref={ref}
      role="menu"
      aria-label={t('menu.label', { name: card.name })}
      className={styles.menu}
      style={{ left: pos.x, top: pos.y }}
      onKeyDown={onKeyDown}
      onContextMenu={onContextMenu}
      onBlur={onBlur}
    >
      {ops.map((op) => (
        <button
          key={op.op === 'move' ? `move-${op.lane}` : op.op}
          type="button"
          role="menuitem"
          tabIndex={-1}
          className={styles.item}
          data-lane={op.op === 'move' ? op.lane : undefined}
          onClick={() => choose(op)}
        >
          <span className={styles.mark} aria-hidden="true">
            {op.op !== 'move' && <PinIcon />}
          </span>
          {opLabel(op)}
        </button>
      ))}
    </div>,
    document.body
  )
}
