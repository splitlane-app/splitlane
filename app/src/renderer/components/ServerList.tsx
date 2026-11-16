import { useEffect, useRef, useState } from 'react'
import type { HelperState } from '../../shared/protocol'
import { t } from '../i18n/t'
import { routingLive, selectedHeld, serverChoices, serverInForce } from '../model/connect'
import { protocolLabel, sourceText } from '../model/proxy'
import { sendCommand, useServices } from '../services'
import styles from './ChoiceList.module.css'
import { focusLost } from './useMounted'
import { useSettledChoice } from './useSettledChoice'

/**
 * The servers of the proxy list as one group of radios, as the exit nodes are: the name, the protocol and where the
 * server came from. Choosing one sends proxy.select, which also makes the proxy the connector: while Tailscale is in
 * force no server is checked. With `removable` each server has «Удалить», at once; the selected one cannot go while
 * live routing runs on it, and a note says so. Once a row went, focus goes to the next row, else to the legend, else
 * (no server left) to `fallbackFocus`. Nothing is drawn without servers.
 */
export function ServerList({
  state,
  removable = false,
  fallbackFocus
}: {
  state: HelperState
  removable?: boolean
  fallbackFocus?: () => void
}): React.JSX.Element | null {
  const services = useServices()
  const [removing, setRemoving] = useState<string | null>(null)
  const listRef = useRef<HTMLFieldSetElement | null>(null)
  const legendRef = useRef<HTMLLegendElement | null>(null)
  // the row asked to go and its place, until the state no longer has it
  const removed = useRef<{ id: string; index: number } | null>(null)
  const live = routingLive(state.phase)
  const { shown, choose } = useSettledChoice(serverInForce(state), live, (id) =>
    sendCommand(services, { method: 'proxy.select', id })
  )
  const servers = serverChoices(state)
  const held = selectedHeld(state) ? state.proxy.selected : null
  const heldListed = held !== null && servers.some((s) => s.id === held)

  useEffect(() => {
    const r = removed.current
    if (r === null || servers.some((s) => s.id === r.id)) return
    removed.current = null
    if (!focusLost()) return
    const next = servers[r.index]
    if (next !== undefined) {
      const row = listRef.current?.querySelector(`[data-server="${next.id}"]`)
      ;(row?.querySelector<HTMLElement>('button:not(:disabled)') ?? row?.querySelector<HTMLElement>('input'))?.focus()
    } else if (servers.length > 0) legendRef.current?.focus()
    else fallbackFocus?.()
  }, [servers, fallbackFocus])

  if (servers.length === 0) return null

  const remove = (id: string): void => {
    // the button stays enabled, so that it keeps the focus until its row goes; a second press waits for the first
    if (removing === id) return
    setRemoving(id)
    removed.current = { id, index: servers.findIndex((s) => s.id === id) }
    void sendCommand(services, { method: 'proxy.remove', id }).then((ok) => {
      if (!ok && removed.current?.id === id) removed.current = null
      setRemoving((r) => (r === id ? null : r))
    })
  }

  return (
    <>
      <fieldset ref={listRef} className={styles.list}>
        <legend ref={legendRef} className={styles.legend} tabIndex={-1}>
          {t('proxy.servers')}
        </legend>
        {servers.map((s) => {
          const source = sourceText(state, s)
          const name = s.name || s.id
          return (
            <div key={s.id} className={styles.row} data-server={s.id}>
              <label className={styles.item}>
                <input
                  type="radio"
                  name="proxy-server"
                  className={styles.radio}
                  checked={s.id === shown}
                  onChange={() => choose(s.id)}
                />
                <span className={styles.name}>{name}</span>
                <span className={styles.meta}>
                  <span className={styles.protocol}>{protocolLabel(s.protocol)}</span>
                  {source !== null && (
                    <>
                      {t('sep')}
                      {source}
                    </>
                  )}
                </span>
              </label>
              {removable && (
                <button
                  type="button"
                  className={styles.remove}
                  aria-label={t('proxy.removeServer', { name })}
                  title={s.id === held ? t('proxy.removeBlocked') : undefined}
                  disabled={s.id === held}
                  onClick={() => remove(s.id)}
                >
                  {t('proxy.remove')}
                </button>
              )}
            </div>
          )
        })}
      </fieldset>
      {removable && heldListed && <p className={styles.note}>{t('proxy.removeBlocked')}</p>}
      {live && <p className={styles.note}>{t('proxy.changeNote')}</p>}
    </>
  )
}
