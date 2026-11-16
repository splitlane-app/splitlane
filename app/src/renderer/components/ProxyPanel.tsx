import { useId, useRef, useState } from 'react'
import { t } from '../i18n/t'
import { addRequest, FILE_REQUEST } from '../model/proxy'
import { useApp, useServices } from '../services'
import styles from './ProxyPanel.module.css'
import { ProxyResult } from './ProxyResult'
import { ServerList } from './ServerList'
import { Subscriptions } from './Subscriptions'
import { useProxyOutcome } from './useProxyOutcome'

/**
 * The add of a key or a subscription (`key`: the pasted text and «Добавить») or of a WireGuard file (`file`: the
 * dialog of the main process), its result, and under them the list of servers; with `manage` («Подключение») each
 * server can be removed and the subscriptions follow. The pasted text holds secrets: it lives in this component only,
 * never in the store or the settings, and goes once an add took a server of it.
 */
export function ProxyPanel({
  mode,
  manage = false
}: {
  mode: 'key' | 'file'
  /** «Удалить» beside each server and the subscriptions («Подключение»). */
  manage?: boolean
}): React.JSX.Element | null {
  const { bridge } = useServices()
  const state = useApp((s) => s.snapshot?.state ?? null)
  const [text, setText] = useState('')
  const addControl = useRef<HTMLTextAreaElement | HTMLButtonElement | null>(null)
  const { running, shown, shownKey, run } = useProxyOutcome(() => addControl.current)
  const fieldId = useId()
  const hintId = useId()

  if (state === null) return null
  // a row removed with nothing left after it: focus goes to the add of this panel
  const focusAdd = (): void => addControl.current?.focus()

  const add = async (): Promise<void> => {
    const o = await run(() => bridge.proxyAdd(text), addRequest(state, text))
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
            ref={(el) => {
              addControl.current = el
            }}
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
          ref={(el) => {
            addControl.current = el
          }}
          type="button"
          className={`${styles.button} ${styles.primary}`}
          disabled={running}
          onClick={() => void run(() => bridge.proxyAddFile(), FILE_REQUEST)}
        >
          {t('proxy.chooseFile')}
        </button>
      )}
      {shown !== null && <ProxyResult key={shownKey} outcome={shown} />}
      <ServerList state={state} removable={manage} fallbackFocus={focusAdd} />
      {manage && <Subscriptions state={state} fallbackFocus={focusAdd} />}
    </div>
  )
}
