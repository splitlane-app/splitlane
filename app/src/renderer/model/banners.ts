import type { HelperState, ProxyProtocol } from '../../shared/protocol'
import type { LastExitNode } from '../../shared/settings'
import type { LinkStatus } from '../../shared/status'
import { t } from '../i18n/t'
import { laneServerName, selectedServer } from './connect'
import { protocolLabel } from './proxy'

export { selectedExitNodeName } from './connect'

export type BannerAction =
  | 'enable'
  | 'cancel'
  | 'retry'
  | 'allDirectOn'
  | 'allDirectOff'
  | 'loginAgain'
  | 'clearSystemExitNode'
  | 'disable'
  | 'skipRestore'
  | 'saveLog'
  | 'retryLink'

export interface Banner {
  tone: 'info' | 'warn' | 'error'
  title: string
  detail?: string
  command?: string
  actions: BannerAction[]
}

/** The banner of the link and the state; `last` is the last name seen for the exit node, as the lane uses it. */
export function bannerFor(link: LinkStatus, state: HelperState | null, last: LastExitNode | null = null): Banner | null {
  if (link.kind === 'restarting') return { tone: 'warn', title: t('banner.restarting'), actions: [] }
  if (link.kind === 'unresponsive') return { tone: 'error', title: t('banner.unresponsive'), actions: ['retryLink'] }
  if (link.kind !== 'ready' || state === null) return null

  if (state.allDirect && (state.phase === 'on' || state.phase === 'degraded')) {
    return { tone: 'warn', title: t('banner.allDirect'), actions: ['allDirectOff'] }
  }

  switch (state.phase) {
    case 'off':
      return { tone: 'info', title: t('banner.off'), actions: ['enable'] }
    case 'starting':
      return { tone: 'info', title: t('banner.starting'), actions: ['cancel'] }
    case 'on':
      return null
    case 'degraded': {
      if (state.reason?.code === 'login_expired') {
        return { tone: 'warn', title: t('banner.loginAgain'), actions: ['loginAgain'] }
      }
      // the server of either connector, by name when the window knows it
      const name = laneServerName(state, last)
      return {
        tone: 'warn',
        title: name === null ? t('banner.degradedUnnamed') : t('banner.degraded', { name }),
        actions: ['retry', 'allDirectOn']
      }
    }
    case 'blocked':
      return { tone: 'warn', title: t('banner.blocked'), actions: ['clearSystemExitNode', 'disable'] }
    case 'stopping': {
      if (state.reason?.code !== 'restore_pending') return { tone: 'info', title: t('banner.stopping'), actions: [] }
      const banner: Banner = { tone: 'error', title: t('banner.restorePending'), actions: ['retry', 'skipRestore'] }
      const { error, command } = state.reason.params
      if (error) banner.detail = error
      if (command) banner.command = command
      return banner
    }
    case 'error': {
      const banner: Banner = {
        tone: 'error',
        title: state.reason
          ? t('banner.error', { reason: t(`reason.${state.reason.code}`) })
          : t('banner.errorUnknown'),
        actions: ['retry', 'saveLog']
      }
      const hint = tunnelHint(state)
      if (hint !== null) banner.detail = hint
      return banner
    }
  }
}

/**
 * The protocols that pass for ordinary TLS or QUIC. In the live check of stage 3 a provider's DPI let
 * these through and cut Shadowsocks and WireGuard to the same server, which a failed check can't tell
 * from a server that is down.
 */
const DISGUISED: ReadonlySet<ProxyProtocol> = new Set<ProxyProtocol>(['vless', 'hysteria2'])

/** The hint under a failed tunnel check of the selected server (enable or switch); null otherwise. */
function tunnelHint(state: HelperState): string | null {
  const code = state.reason?.code
  if (state.connector !== 'proxy' || (code !== 'tunnel_not_proven' && code !== 'switch_check_failed')) return null
  const server = selectedServer(state)
  if (server === null) return null
  if (DISGUISED.has(server.protocol)) return t('banner.proxyCheckServer')
  return t('banner.proxyMaybeBlocked', { protocol: protocolLabel(server.protocol) })
}
