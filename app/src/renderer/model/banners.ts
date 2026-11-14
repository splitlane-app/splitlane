import type { HelperState } from '../../shared/protocol'
import type { LinkStatus } from '../../shared/status'
import { t } from '../i18n/t'

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

/** The name of the selected exit node; null when none is selected or the list has no name for it. */
export function selectedExitNodeName(state: HelperState): string | null {
  const { exitNodes, selectedExitNode } = state.tailscale
  return exitNodes.find((n) => n.id === selectedExitNode)?.name || null
}

function exitNodeName(state: HelperState): string {
  return selectedExitNodeName(state) ?? t('banner.serverFallback')
}

export function bannerFor(link: LinkStatus, state: HelperState | null): Banner | null {
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
    case 'degraded':
      if (state.reason?.code === 'login_expired') {
        return { tone: 'warn', title: t('banner.loginAgain'), actions: ['loginAgain'] }
      }
      return {
        tone: 'warn',
        title: t('banner.degraded', { name: exitNodeName(state) }),
        actions: ['retry', 'allDirectOn']
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
    case 'error':
      return {
        tone: 'error',
        title: state.reason
          ? t('banner.error', { reason: t(`reason.${state.reason.code}`) })
          : t('banner.errorUnknown'),
        actions: ['retry', 'saveLog']
      }
  }
}
