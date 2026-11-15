import { useState } from 'react'
import type { HelperState } from '../../shared/protocol'
import { t } from '../i18n/t'
import { routingLive, selectedHeld, serverChoices, serverInForce } from '../model/connect'
import { protocolLabel, sourceText } from '../model/proxy'
import { sendCommand, useServices } from '../services'
import styles from './ChoiceList.module.css'
import { useSettledChoice } from './useSettledChoice'

/**
 * The servers of the proxy list as one radio group, as the exit nodes are: the name, the protocol and where the server
 * came from. Choosing one sends proxy.select, which also makes the proxy the connector: while Tailscale is in force no
 * server is checked. With `removable` each server has «Удалить», at once; the selected one cannot go while live
 * routing runs on it, and a note says so.
 */
export function ServerList({ state, removable = false }: { state: HelperState; removable?: boolean }): React.JSX.Element {
  const services = useServices()
  const [removing, setRemoving] = useState<string | null>(null)
  const live = routingLive(state.phase)
  const { shown, choose } = useSettledChoice(serverInForce(state), live, (id) =>
    sendCommand(services, { method: 'proxy.select', id })
  )
  const held = selectedHeld(state) ? state.proxy.selected : null
  const heldListed = held !== null && state.proxy.servers.some((s) => s.id === held)

  const remove = (id: string): void => {
    setRemoving(id)
    void sendCommand(services, { method: 'proxy.remove', id }).finally(() => setRemoving((r) => (r === id ? null : r)))
  }

  return (
    <>
      <div className={styles.list} role="radiogroup" aria-label={t('proxy.servers')}>
        {serverChoices(state).map((s) => {
          const source = sourceText(state, s)
          const name = s.name || s.id
          return (
            <div key={s.id} className={styles.row}>
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
                  disabled={s.id === held || removing === s.id}
                  onClick={() => remove(s.id)}
                >
                  {t('proxy.remove')}
                </button>
              )}
            </div>
          )
        })}
      </div>
      {removable && heldListed && <p className={styles.note}>{t('proxy.removeBlocked')}</p>}
      {live && <p className={styles.note}>{t('proxy.changeNote')}</p>}
    </>
  )
}
