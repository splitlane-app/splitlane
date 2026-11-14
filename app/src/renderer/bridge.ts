import type { StoreApi } from 'zustand'
import type { Snapshot, SplitLaneBridge } from '../shared/ipc'
import type { AppState } from './store/appStore'

/**
 * Wires the bridge of the main process to the store: every snapshot push goes to setSnapshot at once, inside the
 * push callback (the layout queue relies on the state of a lanes.set arriving before its answer, see layoutQueue.ts);
 * the activity goes to setActivity while the link is ready, and is dropped whenever it is not. The snapshot the page
 * asks for at start is taken only when no push came first: a push is never older than an answer it overtook, and a
 * change after the answer comes as a push of its own. Returns the unsubscribe.
 */
export function connectBridge(
  bridge: Pick<SplitLaneBridge, 'snapshot' | 'onSnapshot' | 'onActivity'>,
  store: StoreApi<AppState>
): () => void {
  let pushed = false
  let stopped = false
  const apply = (s: Snapshot): void => {
    const st = store.getState()
    st.setSnapshot(s)
    if (s.link.kind !== 'ready') st.setActivity(null)
  }
  const offSnapshot = bridge.onSnapshot((s) => {
    pushed = true
    apply(s)
  })
  const offActivity = bridge.onActivity((a) => {
    const st = store.getState()
    st.setActivity(st.snapshot?.link.kind === 'ready' ? a : null)
  })
  bridge.snapshot().then(
    (s) => {
      if (!pushed && !stopped) apply(s)
    },
    // the page stays on the splash until the first push
    () => {}
  )
  return () => {
    stopped = true
    offSnapshot()
    offActivity()
  }
}

/**
 * Icons of programs by file, asked from the main process once per file (case ignored, as Windows paths) and kept
 * for the life of the page. Only data URLs of images are kept (the page's CSP allows no other images). Listeners
 * hear of an icon that arrived; a file without one changes nothing on screen, so it is not announced.
 */
export class IconCache {
  private readonly icons = new Map<string, string | null>()
  private readonly listeners = new Set<() => void>()

  constructor(private readonly load: (exe: string) => Promise<string | null>) {}

  get(exe: string): string | null {
    return this.icons.get(exe.toLowerCase()) ?? null
  }

  request(exe: string): void {
    const key = exe.toLowerCase()
    if (exe === '' || this.icons.has(key)) return
    this.icons.set(key, null)
    this.load(exe).then(
      (url) => {
        if (typeof url !== 'string' || !url.startsWith('data:image/')) return
        this.icons.set(key, url)
        for (const cb of this.listeners) cb()
      },
      () => {}
    )
  }

  /** For useSyncExternalStore: a stable function. */
  readonly subscribe = (cb: () => void): (() => void) => {
    this.listeners.add(cb)
    return () => {
      this.listeners.delete(cb)
    }
  }
}
