import { create, type StoreApi, type UseBoundStore } from 'zustand'
import type { CommandResult, Snapshot, SplitLaneBridge } from '../../shared/ipc'
import type { Activity, HelperError, HelperState, Phase } from '../../shared/protocol'
import type { Key } from '../i18n/ru'
import { isKey, setLang, t } from '../i18n/t'
import { ruleName } from '../model/board'
import { layoutOf, type Layout } from '../model/layout'
import { LayoutQueue, type LayoutOp } from './layoutQueue'

/** The points of the activity bars of a card: down + up of the last seconds. */
export const HISTORY_POINTS = 30

/** The phases with a running core: only in them the helper sends activity (spec 8.4). */
const LIVE_PHASES: ReadonlySet<Phase> = new Set<Phase>(['on', 'degraded', 'blocked'])

/** A running core: the board has unpinned programs and the special tiles only then (spec 8.4, 9.1). */
export function isLivePhase(phase: Phase | null | undefined): boolean {
  return phase != null && LIVE_PHASES.has(phase)
}

/**
 * A change of the layout as the page asks for it: newId gives the ids of the rules it adds, the same ones
 * each time the change runs again on a newer layout (see layout.ts).
 */
export type LayoutChange = (l: Layout, newId: () => string) => Layout

export interface UiState {
  addOpen: boolean
  /** The key of the card whose program card is open. */
  cardKey: string | null
  screen: 'board' | 'connection'
}

export interface AppState {
  snapshot: Snapshot | null
  activity: Activity | null
  /** The last HISTORY_POINTS values of down + up by activity key, the oldest first. */
  history: Record<string, number[]>
  search: string
  ui: UiState
  notice: string | null
  /** The layout changes shown on screen and not answered by the helper yet, in order. */
  pending: readonly LayoutOp[]
  /**
   * This page sent tailscale.login and the link of that login has not been opened yet: only then the connect
   * screen opens the link in the browser by itself (not the link the helper keeps after a logout).
   */
  loginRequested: boolean
  setSnapshot(s: Snapshot): void
  setActivity(a: Activity | null): void
  setSearch(search: string): void
  openAdd(open: boolean): void
  openCard(key: string | null): void
  showConnection(show: boolean): void
  showNotice(text: string): void
  dismissNotice(): void
  requestLogin(requested: boolean): void
  /** Clears loginRequested; true when it was set (the caller opens the link). */
  takeLoginRequest(): boolean
  /** Shows the change at once and sends it; resolves when its lanes.set is answered (or dropped). */
  change(op: LayoutChange): Promise<void>
}

export interface AppStoreDeps {
  command: SplitLaneBridge['command']
  /** The id of a new rule; crypto.randomUUID by default. */
  newId?: () => string
}

/** The text of a code from the helper; an unknown code (a newer helper) gets the fallback. */
function text(key: string, fallback: Key): string {
  return isKey(key) ? t(key) : t(fallback)
}

/** The notice of a failed command: the text of its error code. */
export function errorText(error: HelperError): string {
  return text(`error.${error.code}`, 'error.internal')
}

/**
 * The notice of a failed lanes.set: for rule_rejected one "<name>: <reason>" per rejected rule of the sent
 * layout, joined with "; "; the text of the error code otherwise.
 */
export function noticeFor(error: HelperError, sent: Layout, activity: Activity | null): string {
  if (error.code === 'rule_rejected' && error.rules !== undefined && error.rules.length > 0) {
    return error.rules
      .map(({ id, reason }) => {
        const why = text(`reject.${reason}`, 'error.rule_rejected')
        const rule = sent.apps.find((r) => r.id === id)
        return rule === undefined ? why : t('notice.rejected', { name: ruleName(rule, activity), reason: why })
      })
      .join('; ')
  }
  return errorText(error)
}

const cache = new WeakMap<HelperState, { pending: readonly LayoutOp[]; layout: Layout }>()

/**
 * The layout on screen: the one of the last state with the pending changes applied again, in order. The same
 * inputs give the same object (a selector of a React hook must not make a new one each time).
 */
export function effectiveLayout(s: Pick<AppState, 'snapshot' | 'pending'>): Layout | null {
  const state = s.snapshot?.state ?? null
  if (state === null) return null
  const hit = cache.get(state)
  if (hit !== undefined && hit.pending === s.pending) return hit.layout
  const layout = s.pending.reduce((l, op) => op(l), layoutOf(state))
  cache.set(state, { pending: s.pending, layout })
  return layout
}

/** A change whose new ids are made once, on its first run, and handed out again in the same order after. */
function withFixedIds(op: LayoutChange, newId: () => string): LayoutOp {
  const ids: string[] = []
  return (l) => {
    let i = 0
    return op(l, () => {
      if (i === ids.length) ids.push(newId())
      return ids[i++]
    })
  }
}

function pushPoint(points: number[] | undefined, value: number): number[] {
  const next = points === undefined ? [value] : [...points, value]
  return next.length > HISTORY_POINTS ? next.slice(next.length - HISTORY_POINTS) : next
}

/**
 * History of the activity bars: every key of the event gets its down + up; a key that had history and is gone
 * gets 0 (its bars decay) and is dropped once its last HISTORY_POINTS values are all 0.
 */
function nextHistory(prev: Record<string, number[]>, a: Activity): Record<string, number[]> {
  const next: Record<string, number[]> = {}
  for (const app of a.apps) next[app.key] = pushPoint(prev[app.key], app.down + app.up)
  for (const [key, points] of Object.entries(prev)) {
    if (key in next) continue
    const decayed = pushPoint(points, 0)
    if (decayed.length < HISTORY_POINTS || decayed.some((v) => v !== 0)) next[key] = decayed
  }
  return next
}

export function createAppStore(deps: AppStoreDeps): UseBoundStore<StoreApi<AppState>> {
  const newId = deps.newId ?? ((): string => crypto.randomUUID())

  return create<AppState>()((set, get) => {
    const send = async (l: Layout): Promise<CommandResult> => {
      let r: CommandResult
      try {
        r = await deps.command({ method: 'lanes.set', apps: l.apps, defaultLane: l.defaultLane })
      } catch (e) {
        r = { ok: false, error: { code: 'internal', message: e instanceof Error ? e.message : String(e) } }
      }
      if (!r.ok) set({ notice: noticeFor(r.error, l, get().activity) })
      return r
    }
    const queue = new LayoutQueue(send, (pending) => set({ pending }))

    return {
      snapshot: null,
      activity: null,
      history: {},
      search: '',
      ui: { addOpen: false, cardKey: null, screen: 'board' },
      notice: null,
      pending: [],
      loginRequested: false,

      setSnapshot(s) {
        // the texts first: whatever renders on this snapshot is in its language
        setLang(s.language)
        const live = s.state !== null && LIVE_PHASES.has(s.state.phase)
        const { activity, history } = get()
        if (live || (activity === null && Object.keys(history).length === 0)) set({ snapshot: s })
        else set({ snapshot: s, activity: null, history: {} })
      },
      setActivity(a) {
        const state = get().snapshot?.state ?? null
        if (a === null || state === null || !LIVE_PHASES.has(state.phase)) {
          // no core, no activity: a late event after the phase changed would stay on screen for good
          if (get().activity !== null || Object.keys(get().history).length > 0) set({ activity: null, history: {} })
          return
        }
        set({ activity: a, history: nextHistory(get().history, a) })
      },
      setSearch(search) {
        set({ search })
      },
      openAdd(addOpen) {
        set({ ui: { ...get().ui, addOpen } })
      },
      openCard(cardKey) {
        set({ ui: { ...get().ui, cardKey } })
      },
      showConnection(show) {
        set({ ui: { ...get().ui, screen: show ? 'connection' : 'board' } })
      },
      showNotice(notice) {
        set({ notice })
      },
      dismissNotice() {
        set({ notice: null })
      },
      requestLogin(loginRequested) {
        if (get().loginRequested !== loginRequested) set({ loginRequested })
      },
      takeLoginRequest() {
        if (!get().loginRequested) return false
        set({ loginRequested: false })
        return true
      },

      async change(op) {
        const now = effectiveLayout(get())
        const state = get().snapshot?.state ?? null
        if (now === null || state === null) return
        const fixed = withFixedIds(op, newId)
        if (fixed(now) === now) return // nothing to send
        const atAction = layoutOf(state)
        // the newest state when the change goes out; the main process never forgets the state once it has one
        await queue.push(fixed, () => {
          const latest = get().snapshot?.state ?? null
          return latest === null ? atAction : layoutOf(latest)
        })
      }
    }
  })
}

/** The page's store, wired to window.splitlane (read when a command goes out, not at import). */
export const useAppStore = createAppStore({ command: (c) => window.splitlane.command(c) })
