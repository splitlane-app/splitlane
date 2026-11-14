import type { HelperState } from '../../shared/protocol'
import type { Snapshot } from '../../shared/ipc'

export type ConnectStep = 'login' | 'waitLogin' | 'approval' | 'exitNode' | 'enable'

export type ScreenName =
  | 'splash'
  | 'setup'
  | 'update-helper'
  | 'update-app'
  | 'other-user'
  | 'connect'
  | 'connection'
  | 'board'

/**
 * Where the user is on the way to the first connection (spec 9.5, 7). After a failed or cancelled enable no core
 * runs (the helper starts its setup core again only on a login, a window connect or a logout) and the backend is
 * `stopped`: the login stays done unless the reason says it is gone. With nothing chosen and no exit node list
 * yet (the first run) a stopped backend is a login still to do.
 */
export function connectStep(state: HelperState): ConnectStep {
  const { authUrl, backend, exitNodes, selectedExitNode } = state.tailscale
  if (authUrl !== null) return 'waitLogin'
  if (backend === 'needs_approval') return 'approval'
  const next: ConnectStep = selectedExitNode === null ? 'exitNode' : 'enable'
  const live =
    backend === 'starting' ||
    backend === 'running' ||
    state.phase === 'on' ||
    state.phase === 'degraded' ||
    state.phase === 'blocked' ||
    state.phase === 'starting'
  if (live) return next
  const code = state.reason?.code
  const loginGone = code === 'not_logged_in' || code === 'login_expired'
  const afterEnable = state.phase === 'off' || state.phase === 'error' || state.phase === 'stopping'
  const known = selectedExitNode !== null || exitNodes.length > 0
  if (afterEnable && backend === 'stopped' && !loginGone && known) return next
  return 'login'
}

/** Whether the connect screen is due: first run, no exit node, or the Tailscale login is gone while routing is off. */
function needsConnect(s: Snapshot, state: HelperState): boolean {
  if (!s.settings.connectedOnce) return true
  const { authUrl, backend, selectedExitNode } = state.tailscale
  if (selectedExitNode === null) return true
  const loginGone = authUrl !== null || backend === 'needs_login' || backend === 'needs_approval' || backend === 'expired'
  return (state.phase === 'off' || state.phase === 'error') && loginGone
}

/** The screen by the link and the state; a link that restarts or does not answer keeps the screen of the last state. */
export function screenFor(s: Snapshot, uiScreen: 'board' | 'connection'): ScreenName {
  switch (s.link.kind) {
    case 'setup-needed':
      return 'setup'
    case 'helper-older':
      return 'update-helper'
    case 'helper-newer':
      return 'update-app'
    case 'other-user':
      return 'other-user'
    case 'connecting':
      return 'splash'
    case 'ready':
    case 'restarting':
    case 'unresponsive':
      break
  }
  if (s.state === null) return 'splash'
  if (needsConnect(s, s.state)) return 'connect'
  return uiScreen === 'connection' ? 'connection' : 'board'
}
