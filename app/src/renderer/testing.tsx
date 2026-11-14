// Test fixtures of the page: a fake bridge, a store on it and a render inside the services context.
// Not imported by the page itself, so it never reaches the bundle.
import { render, type RenderResult } from '@testing-library/react'
import type { ReactNode } from 'react'
import type { Command, CommandResult, Snapshot, SplitLaneBridge } from '../shared/ipc'
import type { Activity, ActivityApp, AppRule, HelperState, Lane } from '../shared/protocol'
import { DEFAULT_SETTINGS } from '../shared/settings'
import type { LinkStatus } from '../shared/status'
import { IconCache } from './bridge'
import { ServicesContext, type Services } from './services'
import { createAppStore } from './store/appStore'

export const ready: LinkStatus = { kind: 'ready', hello: { protocol: 1, helper: '0.1.0', core: '1.14.4' } }

export function state(over: Partial<HelperState> = {}): HelperState {
  return {
    phase: 'on',
    reason: null,
    allDirect: false,
    defaultLane: 'vpn',
    apps: [],
    addresses: { vpn: '198.51.100.7', direct: '203.0.113.9' },
    tailscale: {
      backend: 'running',
      authUrl: null,
      deviceName: 'pc',
      exitNodes: [{ id: '100.64.0.2', name: 'node-a', online: true }],
      selectedExitNode: '100.64.0.2',
      path: 'direct'
    },
    systemTailscale: { installed: false, exitNodeOn: false, restoreOnDisable: false },
    ...over
  }
}

export function snap(over: Partial<HelperState> = {}, link: LinkStatus = ready): Snapshot {
  return { link, state: state(over), settings: DEFAULT_SETTINGS, install: { kind: 'idle' } }
}

export function rule(id: string, name: string, exe: string, lane: Lane): AppRule {
  const dir = exe.slice(0, exe.lastIndexOf('\\') + 1)
  return { id, name, exe, matcher: { kind: 'folder', value: dir }, lane }
}

export function entry(key: string, over: Partial<ActivityApp> = {}): ActivityApp {
  return { key, kind: 'app', name: '', exe: '', ruleId: null, lane: 'vpn', down: 0, up: 0, connections: 0, ...over }
}

export function activity(apps: ActivityApp[]): Activity {
  return { lanes: { vpn: { down: 0, up: 0 }, direct: { down: 0, up: 0 } }, apps }
}

export interface FakeBridge {
  bridge: SplitLaneBridge
  /** Every command the page sent, in order. */
  commands: Command[]
}

/** A bridge that answers every command with `answer` (ok by default) and records it. */
export function fakeBridge(over: Partial<SplitLaneBridge> = {}, answer: CommandResult = { ok: true }): FakeBridge {
  const commands: Command[] = []
  const bridge: SplitLaneBridge = {
    snapshot: () => new Promise<Snapshot>(() => {}),
    onSnapshot: () => () => {},
    onActivity: () => () => {},
    command: async (c) => {
      commands.push(c)
      return answer
    },
    setActivityDetail: async () => {},
    installHelper: async () => {},
    retryLink: async () => {},
    setSettings: async (patch) => ({ ...DEFAULT_SETTINGS, ...patch }),
    installedPrograms: async () => [],
    pickProgram: async () => null,
    resolveDropped: async () => [],
    icon: async () => null,
    openLogin: async () => true,
    saveDiagnostics: async () => ({ saved: false }),
    quit: async () => {},
    ...over
  }
  return { bridge, commands }
}

/** The services of the page on a fresh store wired to bridge, with the snapshot set when one is given. */
export function servicesFor(bridge: SplitLaneBridge, snapshot?: Snapshot): Services {
  const store = createAppStore({ command: (c) => bridge.command(c) })
  if (snapshot !== undefined) store.getState().setSnapshot(snapshot)
  return { store, bridge, icons: new IconCache((exe) => bridge.icon(exe)) }
}

export function renderWith(ui: ReactNode, services: Services): RenderResult {
  return render(<ServicesContext value={services}>{ui}</ServicesContext>)
}
