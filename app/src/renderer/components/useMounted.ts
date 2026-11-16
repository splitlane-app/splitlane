import { useEffect, useRef, type RefObject } from 'react'

/** True while the component is on screen: an answer that comes after it went does not set its state. */
export function useMounted(): RefObject<boolean> {
  const mounted = useRef(true)
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])
  return mounted
}

/** Focus went to the page itself: the control that had it is gone or was disabled. */
export function focusLost(): boolean {
  return document.activeElement === null || document.activeElement === document.body
}
