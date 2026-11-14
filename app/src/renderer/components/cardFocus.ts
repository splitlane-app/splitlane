import type { CardVM } from '../model/board'

/**
 * What finds a card again after a change has moved it, maybe under another key: a pin makes the folder key of an
 * unpinned program the id of its new rule, an unpin the other way round.
 */
export interface CardTarget {
  key: string
  exe: string
  kind: CardVM['kind']
}

export function cardTarget(card: CardVM): CardTarget {
  return { key: card.key, exe: card.exe, kind: card.kind }
}

/**
 * The card of target on the page: the one of the same key, else the one of the same program file (ignoring case),
 * else, for the Windows services, their tile or rule. The placeholder a drag leaves behind (inert) is no card.
 */
export function findCard(target: CardTarget): HTMLElement | null {
  const cards = Array.from(document.querySelectorAll<HTMLElement>('[data-card]')).filter(
    (c) => c.closest('[inert]') === null
  )
  const exe = target.exe.toLowerCase()
  return (
    cards.find((c) => c.dataset.card === target.key) ??
    (exe !== '' ? cards.find((c) => (c.dataset.exe ?? '').toLowerCase() === exe) : undefined) ??
    (target.kind === 'system' ? cards.find((c) => c.dataset.kind === 'system') : undefined) ??
    null
  )
}

/**
 * Focuses el when it is still on the page, else — after the next render has moved the card to its new place —
 * the same part (the card itself or its pin) of the card of target there.
 */
export function focusCard(el: HTMLElement | null, target: CardTarget, part: 'main' | 'pin' = 'main'): void {
  el?.focus()
  requestAnimationFrame(() => {
    if (el?.isConnected === true) return
    const card = findCard(target)
    const pin = part === 'pin' ? card?.querySelector<HTMLElement>('[data-card-pin]') : null
    const next = pin ?? card?.querySelector<HTMLElement>('[data-card-main]')
    next?.focus()
  })
}
