import { HISTORY_POINTS } from '../store/appStore'
import styles from './ActivityBars.module.css'

/** The speed that fills a bar: 10 MB/s. */
const FULL = 10 * 1024 * 1024

/**
 * The height of a bar, 0..1, on a log scale: a program at a few KB/s is visible next to one at MB/s, and the
 * scale is the same on every card (a per-card scale would make a trickle look like a download).
 */
export function barHeight(bytesPerSecond: number): number {
  if (!(bytesPerSecond > 0)) return 0
  return Math.min(1, Math.log1p(bytesPerSecond) / Math.log1p(FULL))
}

/**
 * The bars of the last HISTORY_POINTS seconds of a card, the newest on the right, in the lane's colour. Without
 * traffic in that time they keep their place, unseen, so the columns of the cards stay aligned.
 */
export function ActivityBars({ points }: { points: readonly number[] }): React.JSX.Element {
  const last = points.slice(-HISTORY_POINTS)
  const empty = HISTORY_POINTS - last.length
  const heights = Array.from({ length: HISTORY_POINTS }, (_, i) => barHeight(i < empty ? 0 : last[i - empty]))
  return (
    <span className={styles.bars} data-idle={heights.every((h) => h === 0)} aria-hidden="true">
      {heights.map((h, i) =>
        h > 0 ? (
          <i key={i} className={styles.bar} style={{ height: `${Math.max(12, Math.round(h * 100))}%` }} />
        ) : (
          <i key={i} className={styles.idle} />
        )
      )}
    </span>
  )
}
