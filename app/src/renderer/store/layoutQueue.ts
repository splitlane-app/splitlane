import type { CommandResult } from '../../shared/ipc'
import type { Layout } from '../model/layout'

export type LayoutOp = (l: Layout) => Layout

interface Entry {
  op: LayoutOp
  base: Layout | (() => Layout)
  done: (r: CommandResult) => void
}

/**
 * Changes of the layout on their way to the helper (spec 9.2: shown at once, rolled back on an error).
 * One lanes.set is in flight at a time; the changes go in the order they came, each one applied on the base
 * it is given — a function gives the newest base, read when the change goes out — together with the changes
 * still ahead of it (none: those have been answered by then). A change leaves the queue once its send is
 * answered; a failed one takes every change queued after it along, unsent, and each of their pushes resolves
 * with that failure. Until then pending() shows them all on any base.
 */
export class LayoutQueue {
  private entries: Entry[] = []
  private running = false

  constructor(
    private readonly send: (l: Layout) => Promise<CommandResult>,
    /** Called with the changes still in the queue, in order, after each change of that list. */
    private readonly onChange?: (ops: readonly LayoutOp[]) => void
  ) {}

  push(op: LayoutOp, base: Layout | (() => Layout)): Promise<CommandResult> {
    const result = new Promise<CommandResult>((done) => {
      this.entries.push({ op, base, done })
    })
    this.changed()
    if (!this.running) void this.run()
    return result
  }

  /** base with every change of the queue applied, in order. */
  pending(base: Layout): Layout {
    return this.entries.reduce((l, e) => e.op(l), base)
  }

  private changed(): void {
    this.onChange?.(this.entries.map((e) => e.op))
  }

  private async run(): Promise<void> {
    this.running = true
    try {
      while (this.entries.length > 0) {
        const head = this.entries[0]
        const result = await this.attempt(head)
        if (result.ok) {
          this.entries.shift()
          this.changed()
          head.done(result)
        } else {
          const dropped = this.entries
          this.entries = []
          this.changed()
          for (const e of dropped) e.done(result)
        }
      }
    } finally {
      this.running = false
    }
  }

  /** The send of one change; a throw (of the send or the change) is a failure like an error answer. */
  private async attempt(entry: Entry): Promise<CommandResult> {
    try {
      const base = typeof entry.base === 'function' ? entry.base() : entry.base
      return await this.send(entry.op(base))
    } catch (e) {
      return { ok: false, error: { code: 'internal', message: e instanceof Error ? e.message : String(e) } }
    }
  }
}
