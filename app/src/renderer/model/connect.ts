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

/** Where the user is on the way to the first connection (spec 9.5, 7). */
export function connectStep(state: HelperState): ConnectStep {
  const { authUrl, backend, selectedExitNode } = state.tailscale
  if (authUrl !== null) return 'waitLogin'
  if (backend === 'needs_approval') return 'approval'
  const live =
    backend === 'starting' ||
    backend === 'running' ||
    state.phase === 'on' ||
    state.phase === 'degraded' ||
    state.phase === 'blocked' ||
    state.phase === 'starting'
  if (live) return selectedExitNode === null ? 'exitNode' : 'enable'
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
