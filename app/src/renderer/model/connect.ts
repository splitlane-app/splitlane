import type { ExitNode, HelperState, ProxyServer, Subscription } from '../../shared/protocol'
import type { LastExitNode } from '../../shared/settings'
import type { Snapshot } from '../../shared/ipc'

export type ConnectStep = 'login' | 'waitLogin' | 'approval' | 'exitNode' | 'server' | 'enable'

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
  // the proxy connector has no login: a server is chosen or added, then routing is turned on
  if (state.connector === 'proxy') return selectedServer(state) === null ? 'server' : 'enable'
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

/**
 * Whether the connect screen is due: first run, no exit node (no server for the proxy), or the Tailscale login is
 * gone while routing is off.
 */
function needsConnect(s: Snapshot, state: HelperState): boolean {
  if (!s.settings.connectedOnce) return true
  if (state.connector === 'proxy') return selectedServer(state) === null
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

/** The name of the selected exit node; null when none is selected or the list has no name for it. */
export function selectedExitNodeName(state: HelperState): string | null {
  const { exitNodes, selectedExitNode } = state.tailscale
  return exitNodes.find((n) => n.id === selectedExitNode)?.name || null
}

/**
 * The name of the selected exit node: from the list, else the last name the window saw for that very node (the
 * list is empty while routing is off); null when there is none, the caller then shows the address.
 */
export function knownExitNodeName(state: HelperState, last: LastExitNode | null): string | null {
  const { selectedExitNode } = state.tailscale
  const listed = selectedExitNodeName(state)
  if (listed !== null) return listed
  return selectedExitNode !== null && last !== null && last.id === selectedExitNode ? last.name : null
}

/** The nodes to choose from: the list, plus the selected node when the list lacks it (named as far as known). */
export function exitNodeChoices(state: HelperState, last: LastExitNode | null): ExitNode[] {
  const { exitNodes, selectedExitNode } = state.tailscale
  if (selectedExitNode === null || exitNodes.some((n) => n.id === selectedExitNode)) return exitNodes
  return [...exitNodes, { id: selectedExitNode, name: knownExitNodeName(state, last) ?? selectedExitNode, online: true }]
}

/** The selected server of the proxy list; null when none is selected or the list lacks it. */
export function selectedServer(state: HelperState): ProxyServer | null {
  const { servers, selected } = state.proxy
  return selected === null ? null : (servers.find((s) => s.id === selected) ?? null)
}

/** The servers to choose from, in the order of the list. */
export function serverChoices(state: HelperState): ProxyServer[] {
  return state.proxy.servers
}

/** The subscription a server came from; null for a server added by hand or from a file. */
export function subscriptionOf(state: HelperState, server: ProxyServer): Subscription | null {
  return state.proxy.subscriptions.find((s) => s.id === server.source) ?? null
}

/** The name the VPN lane shows for either connector: the exit node, or the server; null when there is none. */
export function laneServerName(state: HelperState, last: LastExitNode | null): string | null {
  if (state.connector === 'proxy') return selectedServer(state)?.name || null
  return knownExitNodeName(state, last)
}
