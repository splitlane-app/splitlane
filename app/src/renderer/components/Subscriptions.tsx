import { useEffect, useId, useRef, useState, type KeyboardEvent } from 'react'
import type { HelperState, Subscription } from '../../shared/protocol'
import { t } from '../i18n/t'
import { holdsSelected, refreshRequest, subscriptionErrorText, updatedText } from '../model/proxy'
import { sendCommand, useLang, useServices } from '../services'
import { ConnectPhone } from './PhoneQr'
import { ProxyResult } from './ProxyResult'
import styles from './Subscriptions.module.css'
import { focusLost, useMounted } from './useMounted'
import { useProxyOutcome } from './useProxyOutcome'

/**
 * One subscription: its name, how many servers it gave, when it was last updated and why the last refresh failed;
 * «Обновить» with its result beside it, and «Удалить» that asks once more in place (never a dialog of the system).
 */
function SubscriptionItem({
  sub,
  state,
  remove
}: {
  sub: Subscription
  state: HelperState
  /** Sends the removal; true when the helper took it. */
  remove(id: string): Promise<boolean>
}): React.JSX.Element {
  const lang = useLang()
  const mounted = useMounted()
  const refreshRef = useRef<HTMLButtonElement | null>(null)
  const { running, shown, shownKey, run } = useProxyOutcome(() => refreshRef.current)
  const [confirming, setConfirming] = useState(false)
  const [removing, setRemoving] = useState(false)
  const nameId = useId()
  const { bridge } = useServices()
  const removeRef = useRef<HTMLButtonElement | null>(null)
  const cancelRef = useRef<HTMLButtonElement | null>(null)
  // focus follows the question: on «Отмена» when it opens, back on «Удалить» when it is answered and the row stays
  const asked = useRef(false)

  useEffect(() => {
    if (confirming) cancelRef.current?.focus()
    else if (asked.current) removeRef.current?.focus()
    asked.current = confirming
  }, [confirming])

  // the helper keeps the subscription of the server live routing runs on
  const held = holdsSelected(state, sub)
  const updated = updatedText(sub.updatedAt, lang)
  // the refresh just failed for the reason the subscription keeps: one line says it
  const errorShown = shown?.kind === 'failed' && shown.error.code === sub.error

  const confirm = async (): Promise<void> => {
    setRemoving(true)
    await remove(sub.id)
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
    <li className={styles.item} data-sub={sub.id}>
      <div className={styles.head}>
        <strong id={nameId} className={styles.name}>
          {sub.name}
        </strong>
        <span className={styles.meta}>{t('proxy.subCount', { n: sub.count })}</span>
        {updated !== null && <span className={styles.meta}>{updated}</span>}
      </div>
      {sub.error !== null && !errorShown && (
        <p className={styles.error}>{t('proxy.subError', { reason: subscriptionErrorText(sub.error) })}</p>
      )}
      {confirming ? (
        <div className={styles.actions} role="group" aria-labelledby={nameId} onKeyDown={onConfirmKey}>
          <span className={styles.question}>{t('proxy.removeSubConfirm')}</span>
          <button
            type="button"
            className={`${styles.button} ${styles.danger}`}
            disabled={removing || held}
            onClick={() => void confirm()}
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
            ref={refreshRef}
            type="button"
            className={styles.button}
            aria-describedby={nameId}
            disabled={running}
            onClick={() => void run(() => bridge.proxyRefresh(sub.id), refreshRequest(state, sub))}
          >
            {running ? t('proxy.refreshing') : t('proxy.refresh')}
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
          <ConnectPhone subscriptionId={sub.id} />
        </div>
      )}
      {held && <p className={styles.note}>{t('proxy.subRemoveBlocked')}</p>}
      {shown !== null && <ProxyResult key={shownKey} outcome={shown} />}
    </li>
  )
}

/**
 * «Подписки» of «Подключение»: each subscription with its refresh and its removal (with all its servers). Once one
 * went, focus goes to the next one, else to the heading, else (none left) to `fallbackFocus`. Nothing is drawn
 * without subscriptions.
 */
export function Subscriptions({
  state,
  fallbackFocus
}: {
  state: HelperState
  fallbackFocus?: () => void
}): React.JSX.Element | null {
  const services = useServices()
  const headingId = useId()
  const listRef = useRef<HTMLUListElement | null>(null)
  const headingRef = useRef<HTMLHeadingElement | null>(null)
  const removed = useRef<{ id: string; index: number } | null>(null)
  const subs = state.proxy.subscriptions

  useEffect(() => {
    const r = removed.current
    if (r === null || subs.some((s) => s.id === r.id)) return
    removed.current = null
    if (!focusLost()) return
    const next = subs[r.index]
    if (next !== undefined) {
      listRef.current?.querySelector<HTMLElement>(`[data-sub="${next.id}"] button:not(:disabled)`)?.focus()
    } else if (subs.length > 0) headingRef.current?.focus()
    else fallbackFocus?.()
  }, [subs, fallbackFocus])

  if (subs.length === 0) return null

  const remove = async (id: string): Promise<boolean> => {
    removed.current = { id, index: subs.findIndex((s) => s.id === id) }
    const ok = await sendCommand(services, { method: 'proxy.remove', id })
    if (!ok && removed.current?.id === id) removed.current = null
    return ok
  }

  return (
    <section className={styles.section} aria-labelledby={headingId}>
      <h2 ref={headingRef} id={headingId} className={styles.title} tabIndex={-1}>
        {t('proxy.subscriptions')}
      </h2>
      <ul ref={listRef} className={styles.list}>
        {subs.map((s) => (
          <SubscriptionItem key={s.id} sub={s} state={state} remove={remove} />
        ))}
      </ul>
    </section>
  )
}
