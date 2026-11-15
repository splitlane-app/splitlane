import { useEffect, useId, useRef, useState, type KeyboardEvent } from 'react'
import type { HelperState, Subscription } from '../../shared/protocol'
import { t } from '../i18n/t'
import { selectedHeld, selectedServer } from '../model/connect'
import { proxyCall, subscriptionErrorText, updatedText } from '../model/proxy'
import { sendCommand, useLang, useServices } from '../services'
import { ProxyResult, type ShownOutcome } from './ProxyPanel'
import styles from './Subscriptions.module.css'

/**
 * One subscription: its name, how many servers it gave, when it was last updated and why the last refresh failed;
 * «Обновить» with its result beside it, and «Удалить» that asks once more in place (never a dialog of the system).
 */
function SubscriptionItem({ sub, state }: { sub: Subscription; state: HelperState }): React.JSX.Element {
  const services = useServices()
  const lang = useLang()
  const [refreshing, setRefreshing] = useState(false)
  const [outcome, setOutcome] = useState<ShownOutcome | null>(null)
  const [shownCount, setShownCount] = useState(0)
  const [confirming, setConfirming] = useState(false)
  const [removing, setRemoving] = useState(false)
  const nameId = useId()
  const mounted = useRef(true)
  const removeRef = useRef<HTMLButtonElement | null>(null)
  const cancelRef = useRef<HTMLButtonElement | null>(null)
  // focus follows the question: on «Отмена» when it opens, back on «Удалить» when it is cancelled
  const asked = useRef(false)

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])
  useEffect(() => {
    if (confirming) cancelRef.current?.focus()
    else if (asked.current) removeRef.current?.focus()
    asked.current = confirming
  }, [confirming])

  // the helper keeps the subscription of the server live routing runs on
  const held = selectedHeld(state) && selectedServer(state)?.source === sub.id
  const updated = updatedText(sub.updatedAt, lang)

  const refresh = async (): Promise<void> => {
    setRefreshing(true)
    setOutcome(null)
    const o = await proxyCall(() => services.bridge.proxyRefresh(sub.id))
    if (!mounted.current) return
    setRefreshing(false)
    setOutcome(o.kind === 'cancelled' ? null : o)
    setShownCount((n) => n + 1)
  }
  const remove = async (): Promise<void> => {
    setRemoving(true)
    await sendCommand(services, { method: 'proxy.remove', id: sub.id })
    if (!mounted.current) return
    setRemoving(false)
    setConfirming(false)
  }
  const onConfirmKey = (e: KeyboardEvent<HTMLDivElement>): void => {
    // Esc answers «Отмена» here; the screen does not go back to the lanes
    if (e.key !== 'Escape') return
    e.preventDefault()
    setConfirming(false)
  }

  return (
    <li className={styles.item}>
      <div className={styles.head}>
        <strong id={nameId} className={styles.name}>
          {sub.name}
        </strong>
        <span className={styles.meta}>{t('proxy.subCount', { n: sub.count })}</span>
        {updated !== null && <span className={styles.meta}>{updated}</span>}
      </div>
      {sub.error !== null && (
        <p className={styles.error}>{t('proxy.subError', { reason: subscriptionErrorText(sub.error) })}</p>
      )}
      {confirming ? (
        <div className={styles.actions} role="group" aria-labelledby={nameId} onKeyDown={onConfirmKey}>
          <span className={styles.question}>{t('proxy.removeSubConfirm')}</span>
          <button
            type="button"
            className={`${styles.button} ${styles.danger}`}
            disabled={removing || held}
            onClick={() => void remove()}
          >
            {t('proxy.remove')}
          </button>
          <button ref={cancelRef} type="button" className={styles.button} onClick={() => setConfirming(false)}>
            {t('proxy.cancel')}
          </button>
        </div>
      ) : (
        <div className={styles.actions}>
          <button
            type="button"
            className={styles.button}
            aria-describedby={nameId}
            disabled={refreshing}
            onClick={() => void refresh()}
          >
            {refreshing ? t('proxy.refreshing') : t('proxy.refresh')}
          </button>
          <button
            ref={removeRef}
            type="button"
            className={styles.button}
            aria-describedby={nameId}
            title={held ? t('proxy.subRemoveBlocked') : undefined}
            disabled={held}
            onClick={() => setConfirming(true)}
          >
            {t('proxy.remove')}
          </button>
        </div>
      )}
      {held && <p className={styles.note}>{t('proxy.subRemoveBlocked')}</p>}
      {outcome !== null && <ProxyResult key={shownCount} outcome={outcome} />}
    </li>
  )
}

/** «Подписки» of «Подключение»: each subscription with its refresh and its removal (with all its servers). */
export function Subscriptions({ state }: { state: HelperState }): React.JSX.Element | null {
  const headingId = useId()
  const subs = state.proxy.subscriptions
  if (subs.length === 0) return null
  return (
    <section className={styles.section} aria-labelledby={headingId}>
      <h2 id={headingId} className={styles.title}>
        {t('proxy.subscriptions')}
      </h2>
      <ul className={styles.list}>
        {subs.map((s) => (
          <SubscriptionItem key={s.id} sub={s} state={state} />
        ))}
      </ul>
    </section>
  )
}
