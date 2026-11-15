import { expect, test } from 'vitest'
import type { Snapshot } from '../../shared/ipc'
import type { HelperState } from '../../shared/protocol'
import { DEFAULT_SETTINGS } from '../../shared/settings'
import { proxyServer, proxyState, snap, state } from '../testing'
import { bannerFor } from './banners'
import { connectStep, laneServerName, screenFor, selectedServer, serverChoices, subscriptionOf } from './connect'

const A = proxyServer('0123456789abcdef', 'edge-a.example.com')
const B = proxyServer('fedcba9876543210', 'edge-b.example.com', { source: 'sub-0123456789abcdef', protocol: 'trojan' })
const SUB = { id: 'sub-0123456789abcdef', name: 'sub.example.com', updatedAt: '2026-11-16T10:00:00Z', count: 1, error: null }

const withSettings = (s: HelperState, connectedOnce: boolean): Snapshot => ({
  ...snap(),
  state: s,
  settings: { ...DEFAULT_SETTINGS, connectedOnce }
})

test('proxy connectStep: server until one in the list is selected, then enable', () => {
  expect(connectStep(proxyState([], null))).toBe('server')
  expect(connectStep(proxyState([A, B], null))).toBe('server')
  expect(connectStep(proxyState([A, B], A.id))).toBe('enable')
  // the selected id is not in the list
  expect(connectStep(proxyState([B], A.id))).toBe('server')
  expect(connectStep(proxyState([], A.id))).toBe('server')
})

test('proxy connectStep does not look at the Tailscale block or the phase', () => {
  const ts = { ...state().tailscale, backend: 'needs_login' as const, authUrl: 'https://login.example/a', selectedExitNode: null }
  for (const phase of ['off', 'starting', 'on', 'degraded', 'blocked', 'stopping', 'error'] as const) {
    expect(connectStep(proxyState([A], A.id, [], { tailscale: ts, phase })), phase).toBe('enable')
    expect(connectStep(proxyState([A], null, [], { tailscale: ts, phase })), phase).toBe('server')
  }
})

test('tailscale connectStep is unchanged and never says server', () => {
  expect(connectStep(state())).toBe('enable')
  expect(connectStep(state({ tailscale: { ...state().tailscale, selectedExitNode: null } }))).toBe('exitNode')
  // a proxy list on the Tailscale connector changes nothing
  expect(connectStep(state({ proxy: { servers: [A], selected: A.id, subscriptions: [] } }))).toBe('enable')
})

test('screenFor with the proxy connector', () => {
  // first run: the connect screen even with a server selected
  expect(screenFor(withSettings(proxyState([A], A.id), false), 'board')).toBe('connect')
  // no server selected, or the selected one is gone from the list
  expect(screenFor(withSettings(proxyState([A], null), true), 'board')).toBe('connect')
  expect(screenFor(withSettings(proxyState([B], A.id), true), 'connection')).toBe('connect')
  // a selected server after connectedOnce: the board, whatever the phase and the Tailscale login
  expect(screenFor(withSettings(proxyState([A], A.id), true), 'board')).toBe('board')
  expect(screenFor(withSettings(proxyState([A], A.id), true), 'connection')).toBe('connection')
  const lostLogin = { ...state().tailscale, backend: 'expired' as const, selectedExitNode: null }
  expect(screenFor(withSettings(proxyState([A], A.id, [], { phase: 'off', tailscale: lostLogin }), true), 'board')).toBe('board')
})

test('screenFor with Tailscale does not care about the proxy block', () => {
  expect(screenFor(withSettings(state(), true), 'board')).toBe('board')
  expect(screenFor(withSettings(state({ phase: 'off', tailscale: { ...state().tailscale, backend: 'expired' } }), true), 'board')).toBe('connect')
  expect(screenFor(withSettings(state(), false), 'board')).toBe('connect')
})

test('selectedServer, serverChoices and subscriptionOf', () => {
  const s = proxyState([A, B], B.id, [SUB])
  expect(selectedServer(s)).toBe(s.proxy.servers[1])
  expect(selectedServer(proxyState([A, B], null))).toBeNull()
  expect(selectedServer(proxyState([A], B.id))).toBeNull()
  expect(serverChoices(s).map((x) => x.id)).toEqual([A.id, B.id])
  expect(subscriptionOf(s, B)).toEqual(SUB)
  expect(subscriptionOf(s, A)).toBeNull()
  expect(subscriptionOf(proxyState([B], null), B)).toBeNull()
})

test('laneServerName: the exit node for Tailscale, the server for the proxy, null when none', () => {
  expect(laneServerName(state(), null)).toBe('node-a')
  const off = state({ tailscale: { ...state().tailscale, exitNodes: [] } })
  expect(laneServerName(off, { id: '100.64.0.2', name: 'remembered' })).toBe('remembered')
  expect(laneServerName(off, null)).toBeNull()
  const p = proxyState([A, B], B.id, [SUB])
  expect(laneServerName(p, null)).toBe('edge-b.example.com')
  // the remembered exit node is a Tailscale thing
  expect(laneServerName(p, { id: '100.64.0.2', name: 'remembered' })).toBe('edge-b.example.com')
  expect(laneServerName(proxyState([A], null), null)).toBeNull()
  expect(laneServerName(proxyState([A], B.id), null)).toBeNull()
})

test('the degraded banner names the server of either connector', () => {
  const link = snap().link
  expect(bannerFor(link, state({ phase: 'degraded' }))?.title).toContain('node-a')
  expect(bannerFor(link, proxyState([A], A.id, [], { phase: 'degraded' }))?.title).toContain('edge-a.example.com')
})
