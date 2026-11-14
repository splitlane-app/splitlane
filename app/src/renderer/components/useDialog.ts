import { useEffect, useRef, type KeyboardEvent, type RefObject } from 'react'

const FOCUSABLE = 'button:not([disabled]), [href], input:not([disabled]), [tabindex]:not([tabindex="-1"])'

/**
 * The keyboard behaviour of a dialog or a side sheet: the focus goes into it when it opens and back to what had it
 * when it closes; Tab stays inside a modal one (trap); Escape closes (a key that something else handled already is not a close).
 * Returns the key handler for the dialog's element.
 */
export function useDialog(
  ref: RefObject<HTMLElement | null>,
  onClose: () => void,
  trap = true
): { onKeyDown(e: KeyboardEvent<HTMLElement>): void } {
  const close = useRef(onClose)
  close.current = onClose

  useEffect(() => {
    const before = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const first = ref.current?.querySelector<HTMLElement>(FOCUSABLE)
    first?.focus()
    return () => {
      if (before?.isConnected === true) before.focus()
    }
  }, [ref])

  return {
    onKeyDown(e) {
      if (e.defaultPrevented) return
      if (e.key === 'Escape') {
        e.preventDefault()
        e.stopPropagation()
        close.current()
        return
      }
      if (e.key !== 'Tab' || !trap) return
      const items = Array.from(e.currentTarget.querySelectorAll<HTMLElement>(FOCUSABLE))
      if (items.length === 0) return
      const first = items[0]
      const last = items[items.length - 1]
      const active = document.activeElement
      if (e.shiftKey && (active === first || !e.currentTarget.contains(active))) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && (active === last || !e.currentTarget.contains(active))) {
        e.preventDefault()
        first.focus()
      }
    }
  }
}
