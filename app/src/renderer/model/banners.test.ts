import { describe, expect, it } from 'vitest'
import type { HelperState, Phase, Reason } from '../../shared/protocol'
import type { LinkStatus } from '../../shared/status'
import { t } from '../i18n/t'
import { bannerFor } from './banners'

const ready: LinkStatus = { kind: 'ready', hello: { protocol: 2, helper: '1', core: '1' } }

function state(phase: Phase, over: Partial<HelperState> = {}, reason: Reason | null = null): HelperState {
  return {
    phase,
    reason,
    allDirect: false,
    defaultLane: 'vpn',
    apps: [],
    addresses: { vpn: null, direct: null },
    connector: 'tailscale',
    tailscale: {
      backend: 'running',
      authUrl: null,
      deviceName: 'pc',
      exitNodes: [{ id: 'n1', name: 'node-a', online: true }],
      selectedExitNode: 'n1',
      path: null
    },
    proxy: { servers: [], selected: null, subscriptions: [] },
    systemTailscale: { installed: true, exitNodeOn: false, restoreOnDisable: false },
    ...over
  }
}

describe('bannerFor', () => {
  it('follows the table of 9.5', () => {
    expect(bannerFor(ready, state('off'))).toEqual({ tone: 'info', title: t('banner.off'), actions: ['enable'] })
    expect(bannerFor(ready, state('starting'))).toEqual({ tone: 'info', title: t('banner.starting'), actions: ['cancel'] })
    expect(bannerFor(ready, state('degraded', {}, { code: 'vpn_unreachable', params: {} }))).toEqual({
      tone: 'warn',
      title: t('banner.degraded', { name: 'node-a' }),
      actions: ['retry', 'allDirectOn']
    })
    expect(bannerFor(ready, state('degraded', {}, { code: 'core_crashed', params: {} }))?.actions).toEqual([
      'retry',
      'allDirectOn'
    ])
    const noNode = state('degraded', {}, null)
    noNode.tailscale.selectedExitNode = null
    expect(bannerFor(ready, noNode)?.title).toBe(t('banner.degraded', { name: 'VPN' }))
    expect(bannerFor(ready, state('degraded', {}, { code: 'login_expired', params: {} }))).toEqual({
      tone: 'warn',
      title: t('banner.loginAgain'),
      actions: ['loginAgain']
    })
    expect(bannerFor(ready, state('blocked'))).toEqual({
      tone: 'warn',
      title: t('banner.blocked'),
      actions: ['clearSystemExitNode', 'disable']
    })
    expect(bannerFor(ready, state('stopping'))).toEqual({ tone: 'info', title: t('banner.stopping'), actions: [] })
    expect(
      bannerFor(ready, state('stopping', {}, { code: 'restore_pending', params: { error: 'boom', command: 'tailscale set' } }))
    ).toEqual({
      tone: 'error',
      title: t('banner.restorePending'),
      detail: 'boom',
      command: 'tailscale set',
      actions: ['retry', 'skipRestore']
    })
    expect(bannerFor(ready, state('error', {}, { code: 'no_exit_node', params: {} }))).toEqual({
      tone: 'error',
      title: t('banner.error', { reason: t('reason.no_exit_node') }),
      actions: ['retry', 'saveLog']
    })
    expect(bannerFor(ready, state('error'))?.title).toBe(t('banner.errorUnknown'))
  })

  it('shows the restore command only when there is one', () => {
    const b = bannerFor(ready, state('stopping', {}, { code: 'restore_pending', params: { error: '', command: '' } }))
    expect(b).not.toBeNull()
    expect(b).not.toHaveProperty('command')
    expect(b).not.toHaveProperty('detail')
  })

  it('covers allDirect, restarting and unresponsive', () => {
    expect(bannerFor(ready, state('on', { allDirect: true }))).toEqual({
      tone: 'warn',
      title: t('banner.allDirect'),
      actions: ['allDirectOff']
    })
    expect(bannerFor(ready, state('degraded', { allDirect: true }, { code: 'vpn_unreachable', params: {} }))?.actions).toEqual([
      'allDirectOff'
    ])
    expect(bannerFor({ kind: 'restarting', attempt: 2 }, state('on'))).toEqual({
      tone: 'warn',
      title: t('banner.restarting'),
      actions: []
    })
    expect(bannerFor({ kind: 'unresponsive' }, null)).toEqual({
      tone: 'error',
      title: t('banner.unresponsive'),
      actions: ['retryLink']
    })
    expect(bannerFor({ kind: 'connecting' }, null)).toBeNull()
    expect(bannerFor({ kind: 'setup-needed' }, null)).toBeNull()
    expect(bannerFor(ready, null)).toBeNull()
  })

  it('no banner in on without allDirect', () => {
    expect(bannerFor(ready, state('on'))).toBeNull()
  })
})
