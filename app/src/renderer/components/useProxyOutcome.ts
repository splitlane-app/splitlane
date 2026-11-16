import { useEffect, useRef, useState } from 'react'
import type { ProxyOutcome } from '../../shared/ipc'
import type { HelperError, ProxyAddResult } from '../../shared/protocol'
import { outcomeNotice, proxyCall, type ProxyRequest } from '../model/proxy'
import { useServices } from '../services'
import { focusLost, useMounted } from './useMounted'

/** What an add or a refresh gave, as the screen shows it; a failure keeps what was asked, for the text that fits. */
export type ShownOutcome =
  | { kind: 'done'; result: ProxyAddResult }
  | { kind: 'failed'; error: HelperError; request: ProxyRequest }

/**
 * One add or refresh at a time and its answer: `running` while it goes, then `shown` (nothing for a closed file
 * dialog) under a new `shownKey`, so that the same summary twice is announced twice. Focus that was lost meanwhile
 * (the control was disabled) goes back to `refocus()`. An answer that comes after the component went is told by a
 * notice with the numbers only.
 */
export function useProxyOutcome(refocus: () => HTMLElement | null): {
  running: boolean
  shown: ShownOutcome | null
  shownKey: number
  run(call: () => Promise<ProxyOutcome>, request: ProxyRequest): Promise<ProxyOutcome | null>
} {
  const { store } = useServices()
  const mounted = useMounted()
  const [running, setRunning] = useState(false)
  const [shown, setShown] = useState<ShownOutcome | null>(null)
  const [shownKey, setShownKey] = useState(0)
  const refocusDue = useRef(false)
  const refocusRef = useRef(refocus)
  refocusRef.current = refocus

  // after the answer is on screen, so that the control is enabled again
  useEffect(() => {
    if (!refocusDue.current) return
    refocusDue.current = false
    if (focusLost()) refocusRef.current()?.focus()
  }, [shownKey])

  const run = async (call: () => Promise<ProxyOutcome>, request: ProxyRequest): Promise<ProxyOutcome | null> => {
    setRunning(true)
    setShown(null)
    const o = await proxyCall(call)
    if (!mounted.current) {
      const notice = outcomeNotice(o, request)
      if (notice !== null) store.getState().showNotice(notice)
      return null
    }
    setRunning(false)
    setShown(o.kind === 'cancelled' ? null : o.kind === 'done' ? o : { ...o, request })
    refocusDue.current = true
    setShownKey((n) => n + 1)
    return o
  }

  return { running, shown, shownKey, run }
}
