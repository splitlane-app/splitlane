import { expect, test } from 'vitest'
import type { Snapshot } from '../../shared/ipc'
import type { HelperState } from '../../shared/protocol'
import { DEFAULT_SETTINGS } from '../../shared/settings'
import { snap, state } from '../testing'
import { connectStep, screenFor } from './connect'

const ts = (over: Partial<HelperState['tailscale']>, phase: HelperState['phase'] = 'off'): HelperState => {
  const base = state()
  return { ...base, phase, tailscale: { ...base.tailscale, ...over } }
}

test('connectStep walks login, approval, exit node, enable', () => {
  expect(connectStep(ts({ backend: 'needs_login' }))).toBe('login')
  expect(connectStep(ts({ backend: 'stopped' }))).toBe('login')
  expect(connectStep(ts({ backend: 'expired' }))).toBe('login')
  expect(connectStep(ts({ backend: 'needs_login', authUrl: 'https://login.example/a' }))).toBe('waitLogin')
  expect(connectStep(ts({ backend: 'needs_approval' }))).toBe('approval')
  expect(connectStep(ts({ backend: 'running', selectedExitNode: null }))).toBe('exitNode')
  expect(connectStep(ts({ backend: 'starting', selectedExitNode: null }))).toBe('exitNode')
  expect(connectStep(ts({ backend: 'running' }))).toBe('enable')
  expect(connectStep(ts({ backend: 'running' }, 'starting'))).toBe('enable')
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
