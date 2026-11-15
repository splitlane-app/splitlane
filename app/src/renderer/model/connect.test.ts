import { expect, test } from 'vitest'
import type { Snapshot } from '../../shared/ipc'
import type { HelperState } from '../../shared/protocol'
import { DEFAULT_SETTINGS } from '../../shared/settings'
import { snap, state } from '../testing'
import { connectStep, exitNodeChoices, knownExitNodeName, screenFor } from './connect'

const ts = (over: Partial<HelperState['tailscale']>, phase: HelperState['phase'] = 'off'): HelperState => {
  const base = state()
  return { ...base, phase, tailscale: { ...base.tailscale, ...over } }
}

test('connectStep walks login, approval, exit node, enable', () => {
  expect(connectStep(ts({ backend: 'needs_login' }))).toBe('login')
  // the first run: no core, nothing chosen and no exit node list yet
  expect(connectStep(ts({ backend: 'stopped', exitNodes: [], selectedExitNode: null }))).toBe('login')
  expect(connectStep(ts({ backend: 'expired' }))).toBe('login')
  expect(connectStep(ts({ backend: 'needs_login', authUrl: 'https://login.example/a' }))).toBe('waitLogin')
  expect(connectStep(ts({ backend: 'needs_approval' }))).toBe('approval')
  expect(connectStep(ts({ backend: 'running', selectedExitNode: null }))).toBe('exitNode')
  expect(connectStep(ts({ backend: 'starting', selectedExitNode: null }))).toBe('exitNode')
  expect(connectStep(ts({ backend: 'running' }))).toBe('enable')
  expect(connectStep(ts({ backend: 'running' }, 'starting'))).toBe('enable')
})

const withReason = (s: HelperState, code: NonNullable<HelperState['reason']>['code']): HelperState => ({
  ...s,
  reason: { code, params: {} }
})

test('connectStep stays past the login after a failed or cancelled first enable', () => {
  // the helper restarts its setup core only on a login, a window connect or a logout: no core runs here
  const stopped = (phase: HelperState['phase'], over: Partial<HelperState['tailscale']> = {}): HelperState =>
    ts({ backend: 'stopped', ...over }, phase)
  for (const code of ['same_address', 'system_dns_override', 'tunnel_not_proven', 'ip_forwarding'] as const) {
    expect(connectStep(withReason(stopped('error'), code)), code).toBe('enable')
  }
  expect(connectStep(stopped('error'))).toBe('enable')
  expect(connectStep(stopped('off'))).toBe('enable') // cancelled
  expect(connectStep(stopped('stopping'))).toBe('enable') // on the way to off or error
  expect(connectStep(withReason(stopped('stopping'), 'restore_pending'))).toBe('enable')
  expect(connectStep(stopped('off', { selectedExitNode: null }))).toBe('exitNode')
  expect(connectStep(withReason(stopped('error', { selectedExitNode: null }), 'no_exit_node'))).toBe('exitNode')

  // the login is gone: back to the login step
  expect(connectStep(withReason(stopped('error'), 'not_logged_in'))).toBe('login')
  expect(connectStep(withReason(stopped('error'), 'login_expired'))).toBe('login')
  expect(connectStep(withReason(stopped('off'), 'not_logged_in'))).toBe('login')
  expect(connectStep(ts({ backend: 'needs_login' }, 'error'))).toBe('login')
  expect(connectStep(ts({ backend: 'expired' }, 'error'))).toBe('login')
})

test('connectStep in the live phases does not depend on the backend', () => {
  for (const phase of ['on', 'degraded', 'blocked'] as const) {
    for (const backend of ['stopped', 'needs_login', 'expired'] as const) {
      expect(connectStep(ts({ backend }, phase)), `${phase} ${backend}`).toBe('enable')
      expect(connectStep(ts({ backend, selectedExitNode: null }, phase)), `${phase} ${backend}`).toBe('exitNode')
    }
  }
})

const withSettings = (s: Snapshot, connectedOnce: boolean): Snapshot => ({
  ...s,
  settings: { ...DEFAULT_SETTINGS, connectedOnce }
})

test('screenFor maps every link status', () => {
  const base = withSettings(snap(), true)
  const link = (l: Snapshot['link']): Snapshot => ({ ...base, link: l })
  expect(screenFor(link({ kind: 'setup-needed' }), 'board')).toBe('setup')
  expect(screenFor(link({ kind: 'helper-older', helperProtocol: 1 }), 'board')).toBe('update-helper')
  expect(screenFor(link({ kind: 'helper-newer', helperProtocol: 2 }), 'board')).toBe('update-app')
  expect(screenFor(link({ kind: 'other-user' }), 'board')).toBe('other-user')
  expect(screenFor(link({ kind: 'connecting' }), 'board')).toBe('splash')
  expect(screenFor(base, 'board')).toBe('board')
  expect(screenFor(base, 'connection')).toBe('connection')
  expect(screenFor(link({ kind: 'restarting', attempt: 1 }), 'board')).toBe('board')
  expect(screenFor(link({ kind: 'unresponsive' }), 'connection')).toBe('connection')
  expect(screenFor({ ...link({ kind: 'restarting', attempt: 1 }), state: null }, 'board')).toBe('splash')
  expect(screenFor({ ...link({ kind: 'unresponsive' }), state: null }, 'board')).toBe('splash')
  // first run and no exit node
  expect(screenFor(withSettings(snap(), false), 'board')).toBe('connect')
  expect(screenFor(withSettings(snap({ tailscale: { ...state().tailscale, selectedExitNode: null } }), true), 'board')).toBe(
    'connect'
  )
})

test('screenFor returns to connect after logout and an expired login', () => {
  const t0 = state().tailscale
  const off = (over: Partial<HelperState['tailscale']>, phase: HelperState['phase'] = 'off'): Snapshot =>
    withSettings(snap({ phase, tailscale: { ...t0, ...over } }), true)
  expect(screenFor(off({ backend: 'needs_login' }), 'board')).toBe('connect')
  expect(screenFor(off({ backend: 'expired' }), 'board')).toBe('connect')
  expect(screenFor(off({ backend: 'needs_approval' }), 'connection')).toBe('connect')
  expect(screenFor(off({ authUrl: 'https://login.example/a' }, 'error'), 'board')).toBe('connect')
  // while routing is live the board stays (its banner offers «Выключить деление и войти»)
  expect(screenFor(off({ backend: 'expired' }, 'degraded'), 'board')).toBe('board')
  expect(screenFor(off({ backend: 'running' }), 'board')).toBe('board')
})

const A = { id: '100.64.0.2', name: 'node-a', online: true }

test('knownExitNodeName prefers the list, then the remembered name, never the address', () => {
  expect(knownExitNodeName(ts({ exitNodes: [A], selectedExitNode: A.id }), null)).toBe('node-a')
  const off = ts({ exitNodes: [], selectedExitNode: A.id })
  expect(knownExitNodeName(off, { id: A.id, name: 'remembered' })).toBe('remembered')
  expect(knownExitNodeName(off, { id: '100.64.0.9', name: 'other' })).toBeNull()
  expect(knownExitNodeName(off, null)).toBeNull()
  expect(knownExitNodeName(ts({ exitNodes: [A], selectedExitNode: null }), { id: A.id, name: 'x' })).toBeNull()
})

test('exitNodeChoices adds the selected node when the list lacks it', () => {
  const full = ts({ exitNodes: [A], selectedExitNode: A.id })
  expect(exitNodeChoices(full, null)).toEqual([A])
  const off = ts({ exitNodes: [], selectedExitNode: A.id })
  expect(exitNodeChoices(off, { id: A.id, name: 'remembered' })).toEqual([{ id: A.id, name: 'remembered', online: true }])
  expect(exitNodeChoices(off, null)).toEqual([{ id: A.id, name: A.id, online: true }])
  expect(exitNodeChoices(ts({ exitNodes: [], selectedExitNode: null }), null)).toEqual([])
})
