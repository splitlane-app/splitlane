import { useEffect, useId, useRef, useState } from 'react'
import type { ProxyOutcome } from '../../shared/ipc'
import { t } from '../i18n/t'
import { proxyCall, proxyErrorText, rejectionLine } from '../model/proxy'
import { useApp, useServices } from '../services'
import styles from './ProxyPanel.module.css'
import { ServerList } from './ServerList'

/** What an add or a refresh gave, as the screen shows it; nothing for a closed file dialog. */
export type ShownOutcome = Exclude<ProxyOutcome, { kind: 'cancelled' }>

/**
 * The answer of an add or a refresh (spec 6): how many servers came, how many were there already, each line refused
 * with its number and reason (never the line itself: it may hold a secret), or the error of a failed call.
 */
export function ProxyResult({ outcome }: { outcome: ShownOutcome }): React.JSX.Element {
  if (outcome.kind === 'failed') {
    return (
      <p className={styles.failed} role="alert">
        {proxyErrorText(outcome.error)}
      </p>
    )
  }
  const { added, existing, rejected, rejectedMore } = outcome.result
  return (
    <div className={styles.result} role="status">
      <p className={styles.line}>{t('proxy.added', { n: added.length })}</p>
      {existing > 0 && <p className={styles.line}>{t('proxy.existing', { n: existing })}</p>}
      {rejected.length > 0 && (
        <ul className={styles.rejected}>
          {rejected.map((r, i) => (
            <li key={i}>{rejectionLine(r)}</li>
          ))}
        </ul>
      )}
      {rejectedMore > 0 && <p className={styles.line}>{t('proxy.rejectedMore', { n: rejectedMore })}</p>}
    </div>
  )
}

/**
 * The add of a key or a subscription (`key`: the pasted text and «Добавить») or of a WireGuard file (`file`: the
 * dialog of the main process), its result, and under them the list of servers. The pasted text holds secrets: it
 * lives in this component only, never in the store or the settings, and goes once an add took a server of it.
 */
export function ProxyPanel({
  mode,
  removable = false
}: {
  mode: 'key' | 'file'
  /** «Удалить» beside each server («Подключение»). */
  removable?: boolean
}): React.JSX.Element | null {
  const { bridge } = useServices()
  const state = useApp((s) => s.snapshot?.state ?? null)
  const [text, setText] = useState('')
  const [running, setRunning] = useState(false)
  const [outcome, setOutcome] = useState<ShownOutcome | null>(null)
  // a new region for every answer, so that the same summary twice is announced twice
  const [shownCount, setShownCount] = useState(0)
  const mounted = useRef(true)
  const fieldId = useId()
  const hintId = useId()

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  if (state === null) return null

  const run = async (call: () => Promise<ProxyOutcome>): Promise<ProxyOutcome | null> => {
    setRunning(true)
    setOutcome(null)
    const o = await proxyCall(call)
    if (!mounted.current) return null
    setRunning(false)
    setOutcome(o.kind === 'cancelled' ? null : o)
    setShownCount((n) => n + 1)
    return o
  }

  const add = async (): Promise<void> => {
    const o = await run(() => bridge.proxyAdd(text))
    if (o?.kind === 'done' && o.result.added.length + o.result.existing > 0) setText('')
  }

  return (
    <div className={styles.panel}>
      {mode === 'key' ? (
        <div className={styles.field}>
          <label htmlFor={fieldId} className={styles.label}>
            {t('proxy.textLabel')}
          </label>
          <textarea
            id={fieldId}
            className={styles.text}
            value={text}
            rows={4}
            spellCheck={false}
            autoComplete="off"
            readOnly={running}
            aria-describedby={hintId}
            onChange={(e) => setText(e.target.value)}
          />
          <p id={hintId} className={styles.hint}>
            {t('proxy.textHint')}
          </p>
          <button
            type="button"
            className={`${styles.button} ${styles.primary}`}
            disabled={running || text.trim() === ''}
            onClick={() => void add()}
          >
            {running ? t('proxy.adding') : t('proxy.add')}
          </button>
        </div>
      ) : (
        <button
          type="button"
          className={`${styles.button} ${styles.primary}`}
          disabled={running}
          onClick={() => void run(() => bridge.proxyAddFile())}
        >
          {t('proxy.chooseFile')}
        </button>
      )}
      {outcome !== null && <ProxyResult key={shownCount} outcome={outcome} />}
      {state.proxy.servers.length > 0 && <ServerList state={state} removable={removable} />}
    </div>
  )
}
