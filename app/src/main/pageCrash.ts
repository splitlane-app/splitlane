/** A page that dies again within this time of its reload is not reloaded again. */
export const RELOAD_QUIET_MS = 60_000

/**
 * What the window does when its page process is gone (render-process-gone): the window closing ends the page
 * cleanly and is left alone; a crash reloads the page once, so that the window is not left blank. A page that dies
 * again within RELOAD_QUIET_MS of that reload stays as it is (no loop of reloads); a crash after that is a new one.
 */
export class PageCrashReload {
  private lastReload: number | null = null

  constructor(private readonly now: () => number) {}

  /** True: reload the page now. */
  onGone(reason: string): boolean {
    if (reason === 'clean-exit') return false
    const t = this.now()
    if (this.lastReload !== null && t - this.lastReload < RELOAD_QUIET_MS) return false
    this.lastReload = t
    return true
  }
}
