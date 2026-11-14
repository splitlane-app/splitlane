import { useDraggable } from '@dnd-kit/react'
import { useCallback, useRef, useState, type KeyboardEvent, type MouseEvent } from 'react'
import { CARD, CARD_SENSORS } from '../dnd/BoardDnd'
import { MachinesIcon, PinIcon, ProgramIcon, ServicesIcon } from '../icons/builtin'
import { t } from '../i18n/t'
import type { CardVM } from '../model/board'
import { formatSpeed } from '../model/format'
import { pin, unpin } from '../model/layout'
import { useApp, useIcon, useServices } from '../services'
import { ActivityBars } from './ActivityBars'
import styles from './AppCard.module.css'
import { ContextMenu, menuOps, type MenuAnchor } from './ContextMenu'

const NO_POINTS: readonly number[] = []

/** The icon of a card: the program's own, or a built-in one for the special tiles and programs without one. */
function CardIcon({ card }: { card: CardVM }): React.JSX.Element {
  const url = useIcon(card.kind === 'app' ? card.exe : '')
  if (card.kind === 'app' && url !== null) {
    return <img className={styles.icon} src={url} alt="" draggable={false} />
  }
  const Builtin = card.kind === 'system' ? ServicesIcon : card.kind === 'unattributed' ? MachinesIcon : ProgramIcon
  return (
    <span className={styles.icon} data-builtin={card.kind}>
      <Builtin />
    </span>
  )
}

/**
 * Focuses the card of key: el when it is still on the page, else — after the next render has moved the card to
 * its new place — the card of the same key there.
 */
function focusCard(el: HTMLElement | null, key: string): void {
  el?.focus()
  requestAnimationFrame(() => {
    if (el?.isConnected === true) return
    const card = Array.from(document.querySelectorAll<HTMLElement>('[data-card]')).find((c) => c.dataset.card === key)
    card?.querySelector<HTMLElement>('[data-card-main]')?.focus()
  })
}

/**
 * A program on the board (spec 9.1, 9.2): icon, name, speed or «тихо», activity bars and the pin. A pinned card has
 * a solid frame and its pin; an unpinned one a dashed frame, its pin shows on hover and focus. The tile of WSL and
 * virtual machines never moves, so it has no pin, no drag and no menu.
 *
 * The card is a frame with two controls side by side: the card itself (focusable, named by the program and its
 * lane) and the pin. A click or Enter on the card opens the program card; Space takes it for a keyboard drag
 * (dnd-kit); a right click, Shift+F10 or the menu key open its context menu. The frame is what a drag moves.
 */
export function AppCard({ card, onOpen }: { card: CardVM; onOpen(key: string): void }): React.JSX.Element {
  const { store } = useServices()
  const points = useApp((s) => s.history[card.key]) ?? NO_POINTS
  const speed = formatSpeed(card.down + card.up)
  const movable = card.pinned || card.movable
  const [menu, setMenu] = useState<MenuAnchor | null>(null)
  const main = useRef<HTMLDivElement | null>(null)

  const { ref, handleRef } = useDraggable({
    id: card.key,
    data: card,
    type: CARD,
    disabled: !movable,
    sensors: CARD_SENSORS
  })
  // a card that never moves is not a draggable at all (dnd-kit would mark it as a disabled one)
  const mainRef = useCallback(
    (el: HTMLDivElement | null) => {
      main.current = el
      handleRef(movable ? el : null)
    },
    [handleRef, movable]
  )
  const key = card.key
  const closeMenu = useCallback(
    (returnFocus: boolean) => {
      setMenu(null)
      if (returnFocus) focusCard(main.current, key)
    },
    [key]
  )
  const openMenu = (at: MenuAnchor): void => {
    if (menuOps(card).length > 0) setMenu(at)
  }

  const togglePin = (): void => {
    void store.getState().change((l, newId) => (card.ruleId !== null ? unpin(l, card.ruleId) : pin(l, card, newId)))
  }
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>): void => {
    // a key that ended a keyboard drag (dnd-kit prevents it) is not a key of the card
    if (e.target !== e.currentTarget || e.defaultPrevented) return
    if (e.key === 'Enter') {
      e.preventDefault()
      onOpen(card.key)
    } else if (e.key === 'ContextMenu' || (e.key === 'F10' && e.shiftKey)) {
      e.preventDefault()
      const r = e.currentTarget.getBoundingClientRect()
      openMenu({ x: r.left + 12, y: r.bottom + 4 })
    }
  }
  const onContextMenu = (e: MouseEvent): void => {
    e.preventDefault()
    openMenu({ x: e.clientX, y: e.clientY })
  }
  const label = t(card.pinned ? 'card.label' : 'card.labelUnpinned', {
    name: card.name,
    lane: t(card.lane === 'vpn' ? 'lane.vpn' : 'lane.direct')
  })

  return (
    <>
      <div
        ref={movable ? ref : undefined}
        className={styles.card}
        data-card={card.key}
        data-name={card.name}
        data-pinned={card.pinned}
        onContextMenu={onContextMenu}
      >
        <div
          ref={mainRef}
          role="button"
          tabIndex={0}
          aria-label={label}
          aria-roledescription={t('card.role')}
          className={styles.main}
          data-card-main=""
          title={card.exe !== '' ? card.exe : undefined}
          onClick={() => onOpen(card.key)}
          onKeyDown={onKeyDown}
        >
          <CardIcon card={card} />
          <span className={styles.name}>{card.name}</span>
          <span className={styles.speed} data-quiet={speed === ''}>
            {speed !== '' ? speed : t('card.quiet')}
          </span>
          <ActivityBars points={points} />
        </div>
        {movable ? (
          <button
            type="button"
            className={styles.pin}
            aria-label={t(card.pinned ? 'unpin' : 'pin')}
            title={t(card.pinned ? 'unpin' : 'pin')}
            onClick={togglePin}
          >
            <PinIcon />
          </button>
        ) : (
          // keeps the columns of speed and bars in line with the cards that have a pin
          <span className={styles.pinSpace} aria-hidden="true" />
        )}
      </div>
      {menu !== null && <ContextMenu card={card} at={menu} onClose={closeMenu} />}
    </>
  )
}
