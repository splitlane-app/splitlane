import type { ExitNode, HelperState } from '../../shared/protocol'
import type { LastExitNode } from '../../shared/settings'
import { currentLang, t } from '../i18n/t'
import { exitNodeChoices, exitNodeInForce, routingLive } from '../model/connect'
import { sendCommand, useServices } from '../services'
import styles from './ChoiceList.module.css'
import { useSettledChoice } from './useSettledChoice'

/** Online nodes first, then by name in the order of the current language. */
function sortedNodes(nodes: ExitNode[]): ExitNode[] {
  const lang = currentLang()
  return [...nodes].sort((a, b) => Number(b.online) - Number(a.online) || a.name.localeCompare(b.name, lang))
}

/**
 * The exit nodes of the tailnet as one radio group; choosing one sends tailscale.setExitNode (the helper validates
 * it), which also makes Tailscale the connector: while a server is in force no node is checked. The selected node is
 * always in it, named as far as the window knows (the list is empty while routing is off).
 */
export function ExitNodeList({ state, last }: { state: HelperState; last: LastExitNode | null }): React.JSX.Element {
  const services = useServices()
  const nodes = exitNodeChoices(state, last)
  const { shown, choose } = useSettledChoice(exitNodeInForce(state), routingLive(state.phase), (id) =>
    sendCommand(services, { method: 'tailscale.setExitNode', id })
  )
  if (nodes.length === 0) return <p className={styles.note}>{t('connect.noExitNodes')}</p>
  return (
    <div className={styles.list} role="radiogroup" aria-label={t('connect.exitNodes')}>
      {sortedNodes(nodes).map((n) => (
        <label key={n.id} className={styles.item} title={n.online ? undefined : t('connect.offlineWarn')}>
          <input
            type="radio"
            name="exit-node"
            className={styles.radio}
            checked={n.id === shown}
            onChange={() => choose(n.id)}
          />
          <span className={styles.name}>{n.name || n.id}</span>
          {!n.online && <span className={styles.offline}>{t('connect.offline')}</span>}
        </label>
      ))}
    </div>
  )
}
