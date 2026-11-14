import type { HelperState } from '../shared/protocol'

export interface AutoEnableDeps {
  enable(): Promise<void>
  setTimer(fn: () => void, ms: number): () => void
  now(): number
}

const RETRY_MS = 10_000
const WINDOW_MS = 120_000

/** "Enable splitting on start": one cycle per window run (spec 9.6). */
export class AutoEnable {
  private started = false
  private active = false
  private startedAt = 0
  private cancel: (() => void) | null = null

  constructor(private readonly d: AutoEnableDeps) {}

  onReady(state: HelperState, setting: boolean): void {
    if (!setting || this.started) return
    this.started = true
    if (state.phase !== 'off') return
    this.active = true
    this.startedAt = this.d.now()
    this.attempt()
  }

  onState(state: HelperState): void {
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

  stop(): void {
    this.active = false
    this.cancel?.()
    this.cancel = null
  }

  private attempt(): void {
    this.d.enable().catch(() => {})
  }
}
