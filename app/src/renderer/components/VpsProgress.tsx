import { useEffect, useState } from 'react'
import type { Key } from '../i18n/ru'
import { t } from '../i18n/t'
import { elapsedText, stepView, VPS_STEPS, type StepView, type VpsRun } from '../model/vps'
import styles from './VpsProgress.module.css'

const STATE: Record<StepView, Key> = {
  waiting: 'vps.state.waiting',
  running: 'vps.state.running',
  done: 'vps.state.done',
  skip: 'vps.state.skip',
  warn: 'vps.state.warn',
  failed: 'vps.state.failed',
  cancelled: 'vps.state.cancelled'
}

/**
 * The six steps of a setup (spec 6), each with its state, and the time since «Настроить», which stops at the end.
 * Once the setup is over a step it never reached says nothing (it is not waiting any more).
 */
export function VpsProgress({ run }: { run: VpsRun }): React.JSX.Element {
  const ended = run.endedAt !== null
  const [now, setNow] = useState(() => Date.now())

  // a tick a second while it runs; a new setup is a new component (the form comes between two)
  useEffect(() => {
    if (ended) return
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [ended])

  return (
    <div className={styles.progress}>
      <ol className={styles.steps} aria-label={t('vps.progress')}>
        {VPS_STEPS.map((step) => {
          const view = stepView(run.steps[step])
          const silent = ended && view === 'waiting'
          return (
            <li key={step} className={styles.step} data-state={silent ? 'none' : view}>
              <span className={styles.mark} aria-hidden="true" />
              <span className={styles.name}>{t(`vps.step.${step}`)}</span>
              {!silent && <span className={styles.state}>{t(STATE[view])}</span>}
            </li>
          )
        })}
      </ol>
      <p className={styles.elapsed}>{t('vps.elapsed', { time: elapsedText((run.endedAt ?? now) - run.startedAt) })}</p>
    </div>
  )
}
