import type { KeyboardEvent, MouseEvent } from 'react'
import { MachinesIcon, PinIcon, ProgramIcon, ServicesIcon } from '../icons/builtin'
import { t } from '../i18n/t'
import type { CardVM } from '../model/board'
import { formatSpeed } from '../model/format'
import { pin, unpin } from '../model/layout'
import { useApp, useIcon, useServices } from '../services'
import { ActivityBars } from './ActivityBars'
import styles from './AppCard.module.css'

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
 * A program on the board (spec 9.1): icon, name, speed or «тихо», activity bars and the pin. A pinned card has a
 * solid frame and its pin; an unpinned one a dashed frame, its pin shows on hover and focus. The tile of WSL and
 * virtual machines never moves, so it has no pin. A click (or Enter) opens the program card.
 */
export function AppCard({ card, onOpen }: { card: CardVM; onOpen(key: string): void }): React.JSX.Element {
  const { store } = useServices()
  const points = useApp((s) => s.history[card.key]) ?? NO_POINTS
  const speed = formatSpeed(card.down + card.up)
  const canPin = card.pinned || card.movable

  const togglePin = (e: MouseEvent): void => {
    e.stopPropagation()
    void store.getState().change((l, newId) => (card.ruleId !== null ? unpin(l, card.ruleId) : pin(l, card, newId)))
  }
  const onKeyDown = (e: KeyboardEvent): void => {
    if (e.key === 'Enter' && e.target === e.currentTarget) {
      e.preventDefault()
      onOpen(card.key)
    }
  }

  return (
    <div
      role="button"
      tabIndex={0}
      className={styles.card}
      data-card={card.key}
      data-name={card.name}
      data-pinned={card.pinned}
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
      {canPin ? (
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
  )
}
