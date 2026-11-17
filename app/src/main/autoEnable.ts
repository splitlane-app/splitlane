import type { HelperState } from '../shared/protocol'

export interface AutoEnableDeps {
  enable(): Promise<void>
  setTimer(fn: () => void, ms: number): () => void
  now(): number
}

const RETRY_MS = 10_000
const WINDOW_MS = 120_000

/**
 * "Enable splitting on start": one cycle per window run (spec 9.6). A helper that is still in the
 * stopping of its own start-up (it gives the system exit node back, spec 5.7) is waited for, up to
 * 2 minutes: the cycle begins when that stopping ends in off.
 */
export class AutoEnable {
  private started = false
  private active = false
  private startedAt = 0
  private waitingSince: number | null = null
  private cancel: (() => void) | null = null

  constructor(private readonly d: AutoEnableDeps) {}

  onReady(state: HelperState, setting: boolean): void {
    if (!setting || this.started) return
    this.started = true
    if (state.phase === 'stopping') {
      this.waitingSince = this.d.now()
      return
    }
    if (state.phase !== 'off') return
    this.begin()
  }

  onState(state: HelperState): void {
    if (this.waitingSince !== null) {
      if (state.phase === 'stopping') return
      const waited = this.d.now() - this.waitingSince
      this.waitingSince = null
      if (state.phase === 'off' && waited < WINDOW_MS) this.begin()
      return
    }
    if (!this.active) return
    if (state.phase === 'error' && state.reason?.code === 'no_default_route') {
      if (this.cancel || this.d.now() - this.startedAt >= WINDOW_MS) {
        if (!this.cancel) this.active = false
        return
      }
      this.cancel = this.d.setTimer(() => {
        this.cancel = null
        if (this.active) this.attempt()
      }, RETRY_MS)
      return
    }
    if (state.phase === 'starting') return
    this.stop()
  }

  /** True while the start-up enable is still being tried (its failed attempts are expected). */
  retrying(): boolean {
    return this.active
  }

  stop(): void {
    this.active = false
    this.waitingSince = null
    this.cancel?.()
    this.cancel = null
  }

  private begin(): void {
    this.active = true
    this.startedAt = this.d.now()
    this.attempt()
  }

  private attempt(): void {
    this.d.enable().catch(() => {})
  }
}
