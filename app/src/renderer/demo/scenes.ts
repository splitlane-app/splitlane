// The fixed, invented data of the demo window (screenshots of the site and the README). Nothing here is real: the
// addresses are documentation ones, the paths are default install folders, the speeds are made up.
import type { Snapshot, VpsEvent, VpsOutcome } from '../../shared/ipc'
import type { Activity, ActivityApp, AppRule, HelperState, Lane } from '../../shared/protocol'
import { DEFAULT_SETTINGS } from '../../shared/settings'
import { newRun, runEnd, runEvent, type VpsRun } from '../model/vps'

export const SCENES = ['board', 'connection', 'vps-progress', 'vps-done', 'phone'] as const
export type SceneName = (typeof SCENES)[number]

export const VPS_ADDRESS = '203.0.113.10'
/** The address the direct lane shows (masked on screen), in the same documentation block. */
export const DIRECT_ADDRESS = '203.0.113.77'
export const SUBSCRIPTION_HOST = '203-0-113-10.sslip.io'
export const PHONE_ADDRESS = `https://${SUBSCRIPTION_HOST}:8443/7f3a9c1e5b2d48a6c0e9d1b3a5f72e84/sub`

const SUBSCRIPTION_ID = 'sub-0123456789abcdef'
const SERVER_VLESS = '00000000000000a1'
const SERVER_HYSTERIA = '00000000000000a2'

export interface DemoProgram {
  id: string
  name: string
  exe: string
  lane: Lane
  /** bytes per second */
  down: number
  up: number
  connections: number
}

const STEAM = 'C:\\Program Files (x86)\\Steam\\'
const EPIC = 'C:\\Program Files\\Epic Games\\'

// The two games are what the app shows for a detected Steam or Epic game: a rule on the game's exe, whose folder
// matcher covers the game's folder (games.ts).
export const PROGRAMS: readonly DemoProgram[] = [
  { id: 'r-steam', name: 'Steam', exe: `${STEAM}steam.exe`, lane: 'vpn', down: 2_516_582, up: 41_000, connections: 14 },
  {
    id: 'r-epic',
    name: 'Epic Games Launcher',
    exe: `${EPIC}Launcher\\Portal\\Binaries\\Win64\\EpicGamesLauncher.exe`,
    lane: 'vpn',
    down: 655_360,
    up: 12_300,
    connections: 6
  },
  { id: 'r-discord', name: 'Discord', exe: 'C:\\Program Files\\Discord\\Discord.exe', lane: 'vpn', down: 14_800, up: 9_400, connections: 5 },
  {
    id: 'r-firefox',
    name: 'Firefox',
    exe: 'C:\\Program Files\\Mozilla Firefox\\firefox.exe',
    lane: 'vpn',
    down: 389_000,
    up: 43_000,
    connections: 22
  },
  {
    id: 'r-cs2',
    name: 'Counter-Strike 2',
    exe: `${STEAM}steamapps\\common\\Counter-Strike Global Offensive\\game\\bin\\win64\\cs2.exe`,
    lane: 'direct',
    down: 52_000,
    up: 31_000,
    connections: 3
  },
  {
    id: 'r-fortnite',
    name: 'Fortnite',
    exe: `${EPIC}Fortnite\\FortniteGame\\Binaries\\Win64\\FortniteClient-Win64-Shipping.exe`,
    lane: 'direct',
    down: 28_000,
    up: 17_000,
    connections: 4
  },
  {
    id: 'r-edge',
    name: 'Microsoft Edge',
    exe: 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',
    lane: 'direct',
    down: 1_153_434,
    up: 66_000,
    connections: 18
  }
]

function rule(p: DemoProgram): AppRule {
  const dir = p.exe.slice(0, p.exe.lastIndexOf('\\') + 1)
  return { id: p.id, name: p.name, exe: p.exe, matcher: { kind: 'folder', value: dir }, lane: p.lane }
}

const SERVERS: HelperState['proxy']['servers'] = [
  { id: SERVER_VLESS, name: `${VPS_ADDRESS} VLESS`, protocol: 'vless', source: SUBSCRIPTION_ID },
  { id: SERVER_HYSTERIA, name: `${VPS_ADDRESS} Hysteria2`, protocol: 'hysteria2', source: SUBSCRIPTION_ID }
]

const SUBSCRIPTION = {
  id: SUBSCRIPTION_ID,
  name: SUBSCRIPTION_HOST,
  updatedAt: '2026-11-16T09:30:00Z',
  count: 2,
  error: null
}

function state(over: Partial<HelperState>): HelperState {
  return {
    phase: 'on',
    reason: null,
    allDirect: false,
    defaultLane: 'vpn',
    apps: PROGRAMS.map(rule),
    addresses: { vpn: VPS_ADDRESS, direct: DIRECT_ADDRESS },
    connector: 'proxy',
    tailscale: { backend: 'stopped', authUrl: null, deviceName: '', exitNodes: [], selectedExitNode: null, path: null },
    proxy: { servers: [...SERVERS], selected: SERVER_VLESS, subscriptions: [SUBSCRIPTION] },
    systemTailscale: { installed: false, exitNodeOn: false, restoreOnDisable: false },
    ...over
  }
}

function snapshot(st: HelperState, connectedOnce: boolean): Snapshot {
  return {
    link: { kind: 'ready', hello: { protocol: 3, helper: '0.5.0', core: '1.14.4' } },
    state: st,
    settings: { ...DEFAULT_SETTINGS, connectedOnce, language: 'en' },
    language: 'en',
    install: { kind: 'idle' },
    update: { kind: 'idle' }
  }
}

/** A repeatable pseudo-random number in [0, 1) for a program and a second. */
function noise(i: number, tick: number): number {
  const x = Math.sin(i * 12.9898 + tick * 78.233) * 43758.5453
  return x - Math.floor(x)
}

/**
 * The share of the program's base speed in second `tick`, by the kind of program: a download is bursty (pauses and
 * bursts), a game is steady and low, a browser is spiky (quiet, then a page loads), Discord is nearly flat.
 * The activity bars draw it on a log scale, so the swings are by factors, not by percents.
 */
function shape(p: DemoProgram, i: number, tick: number): number {
  const r = noise(i, tick)
  switch (p.id) {
    case 'r-steam':
      return r < 0.2 ? 0.02 + r : tick % 9 < 3 ? 0.15 + r * 0.3 : 0.6 + r * 0.7
    case 'r-epic':
      return r < 0.35 ? 0.03 : 0.3 + r * 1.1
    case 'r-firefox':
    case 'r-edge':
      return r > 0.78 ? 2.5 + r * 4 : 0.02 + r * 0.12
    case 'r-discord':
      return 0.85 + r * 0.3
    default:
      return 0.75 + r * 0.5
  }
}

/** The activity of one second: the base speeds shaped per program, deterministic (`tick` is the second). */
export function activityAt(tick: number): Activity {
  const apps: ActivityApp[] = PROGRAMS.map((p, i) => ({
    key: p.id,
    kind: 'app',
    name: p.name,
    exe: p.exe,
    ruleId: p.id,
    lane: p.lane,
    down: Math.round(p.down * shape(p, i, tick)),
    up: Math.round(p.up * shape(p, i, tick + 100)),
    connections: p.connections
  }))
  const sum = (lane: Lane, k: 'down' | 'up'): number => apps.filter((a) => a.lane === lane).reduce((n, a) => n + a[k], 0)
  return {
    lanes: {
      vpn: { down: sum('vpn', 'down'), up: sum('vpn', 'up') },
      direct: { down: sum('direct', 'down'), up: sum('direct', 'up') }
    },
    apps
  }
}

/** How many seconds of history the board starts with (the activity bars hold 30). */
export const HISTORY_SECONDS = 30

function vpsRun(done: boolean, now: number): VpsRun {
  let run = newRun(now - (done ? 168_000 : 71_000), false)
  const events: VpsEvent[] = [
    { step: 'connect', state: 'start' },
    { step: 'connect', state: 'done' },
    { step: 'system', state: 'start' },
    { step: 'system', state: 'done' },
    { step: 'install', state: 'start' }
  ]
  if (done) {
    events.push(
      { step: 'install', state: 'done' },
      { step: 'harden', state: 'start' },
      { step: 'harden', state: 'done' },
      { step: 'subscription', state: 'start' },
      { step: 'subscription', state: 'done' },
      { step: 'add', state: 'start' },
      { step: 'add', state: 'done' }
    )
  }
  for (const e of events) run = runEvent(run, e)
  if (!done) return run
  const outcome: VpsOutcome = { kind: 'done', servers: 2, added: 2, subscriptionId: SUBSCRIPTION_ID, warnings: [] }
  return runEnd(run, outcome, now)
}

export interface Scene {
  snapshot: Snapshot
  /** The «Подключение» screen instead of the board. */
  connection: boolean
  /** The run of «Свой VPS» put in the store, built at `now`. */
  vps: ((now: number) => VpsRun) | null
  /** Click the «Your own VPS» tile open (a finished run opens on another one). */
  vpsTile: boolean
  /** Click «Connect a phone». */
  phone: boolean
  /** Feed the activity (the board shows speeds). */
  live: boolean
}

export function sceneOf(name: SceneName): Scene {
  // the setup runs with routing off, on the first-run screen: the servers appear when it is done
  const setupOff = (servers: boolean): Snapshot =>
    snapshot(
      state({
        phase: 'off',
        apps: [],
        addresses: { vpn: null, direct: null },
        proxy: servers
          ? { servers: [...SERVERS], selected: SERVER_VLESS, subscriptions: [SUBSCRIPTION] }
          : { servers: [], selected: null, subscriptions: [] }
      }),
      false
    )
  const base: Scene = { snapshot: snapshot(state({}), true), connection: false, vps: null, vpsTile: false, phone: false, live: true }
  switch (name) {
    case 'board':
      return base
    case 'connection':
      return { ...base, connection: true }
    case 'vps-progress':
      return { ...base, snapshot: setupOff(false), vps: (now) => vpsRun(false, now), vpsTile: true, live: false }
    case 'vps-done':
      return { ...base, snapshot: setupOff(true), vps: (now) => vpsRun(true, now), vpsTile: true, live: false }
    case 'phone':
      return { ...base, snapshot: setupOff(true), vps: (now) => vpsRun(true, now), vpsTile: true, phone: true, live: false }
  }
}
