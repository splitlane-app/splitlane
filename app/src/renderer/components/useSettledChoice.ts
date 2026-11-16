import { useEffect, useRef, useState } from 'react'
import { useMounted } from './useMounted'

/** How long a live choice waits for the next one before it is sent. */
export const CHOICE_SETTLE_MS = 500

/**
 * The choice of a radio group whose every change restarts live split routing (the exit node, the server). Arrow keys
 * move through native radios and change the choice on each press, so while routing is live a choice is sent when
 * settled: the last one, after a pause. Without routing it goes at once, so the steps of the wizard do not lag.
 * Choosing the one in force again sends nothing; leaving the screen applies a choice still waiting, it does not drop
 * it. `shown` is the choice to check: the one waiting or sent and not answered yet, else the one in force.
 */
export function useSettledChoice(
  inForce: string | null,
  live: boolean,
  send: (id: string) => Promise<unknown>
): { shown: string | null; choose(id: string): void } {
  const [pending, setPending] = useState<string | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const waiting = useRef<string | null>(null)
  const mounted = useMounted()
  const sendNow = (id: string): void => {
    const answered = (): void => {
      if (mounted.current) setPending((p) => (p === id ? null : p))
    }
    send(id).then(answered, answered)
  }
  const sendRef = useRef(sendNow)
  sendRef.current = sendNow
  // leaving the screen applies the choice still waiting
  useEffect(
    () => () => {
      if (timer.current !== null && waiting.current !== null) {
        clearTimeout(timer.current)
        sendRef.current(waiting.current)
      }
    },
    []
  )
  const choose = (id: string): void => {
    if (timer.current !== null) clearTimeout(timer.current)
    timer.current = null
    waiting.current = null
    // back on the one in force: nothing to send (the helper restarts on any change while live)
    if (id === inForce) {
      setPending(null)
      return
    }
    setPending(id)
    if (!live) {
      sendNow(id)
      return
    }
    waiting.current = id
    timer.current = setTimeout(() => {
      timer.current = null
      waiting.current = null
      sendNow(id)
    }, CHOICE_SETTLE_MS)
  }
  return { shown: pending ?? inForce, choose }
}
