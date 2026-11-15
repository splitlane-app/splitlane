import type { HelperState } from '../../shared/protocol'
import type { LastExitNode } from '../../shared/settings'
import { t } from '../i18n/t'
import { knownExitNodeName } from '../model/connect'
import styles from './ConnectionPill.module.css'

/** The parts of the pill: «Tailscale · <exit node> · прямое соединение | через ретранслятор» (spec 9.1). */
export function pillText(state: HelperState, last: LastExitNode | null = null): string {
  const { selectedExitNode, path } = state.tailscale
  const parts = [t('pill.tailscale'), knownExitNodeName(state, last) ?? selectedExitNode ?? t('pill.noExitNode')]
  if (path !== null) parts.push(t(path === 'direct' ? 'pill.direct' : 'pill.relay'))
  return parts.join(t('sep'))
}

/** The dot of the pill: up while the node runs, a warning while the server is unreachable, idle otherwise. */
function tone(state: HelperState): 'up' | 'warn' | 'idle' {
  if (state.phase === 'degraded') return 'warn'
  return state.tailscale.backend === 'running' && state.tailscale.path !== null ? 'up' : 'idle'
}

export function ConnectionPill({
  state,
  last = null,
  onOpen
}: {
  state: HelperState
  /** The last name seen for the selected node; used while the list is empty. */
  last?: LastExitNode | null
  onOpen(): void
}): React.JSX.Element {
  const text = pillText(state, last)
  return (
    <button type="button" className={styles.pill} data-tone={tone(state)} onClick={onOpen} title={text}>
      <span className={styles.dot} aria-hidden="true" />
      <span className={styles.text}>{text}</span>
    </button>
  )
}
