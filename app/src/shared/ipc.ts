import type { Dropped, Program } from './programs'
import type { Activity, AppRule, HelperError, HelperState, Lane } from './protocol'
import type { WindowSettings } from './settings'
import type { InstallOutcome, LinkStatus } from './status'

/** The install of the helper started from the window (spec 4.3). */
export type InstallState = { kind: 'idle' } | { kind: 'running' } | { kind: 'done'; outcome: InstallOutcome }

/** Everything the page shows apart from the activity; sent on every change. */
export interface Snapshot {
  link: LinkStatus
  state: HelperState | null
  settings: WindowSettings
  install: InstallState
}

/** The helper methods the page may send (spec 5.1); the main process owns the rest. */
export type Command =
  | { method: 'routing.enable' }
  | { method: 'routing.disable'; skipRestore?: boolean }
  | { method: 'routing.allDirect'; on: boolean }
  | { method: 'lanes.set'; apps: AppRule[]; defaultLane: Lane }
  | { method: 'tailscale.login' }
  | { method: 'tailscale.logout' }
  | { method: 'tailscale.setExitNode'; id: string }
  | { method: 'systemTailscale.clearExitNode' }

/** A lost link (closed pipe, no answer in time) is `{ code: 'internal', message: 'link closed' }`. */
export type CommandResult = { ok: true } | { ok: false; error: HelperError }

export const LINK_CLOSED: HelperError = { code: 'internal', message: 'link closed' }

/** At most this many files of one Explorer drop go to the main process. */
export const MAX_DROPPED = 100

/** `window.splitlane`: the only way from the page to the main process. */
export interface SplitLaneBridge {
  snapshot(): Promise<Snapshot>
  onSnapshot(cb: (s: Snapshot) => void): () => void
  onActivity(cb: (a: Activity) => void): () => void
  /** «Настройки…» of the tray: the page opens «Подключение». The push carries nothing. */
  onOpenSettings(cb: () => void): () => void
  command(c: Command): Promise<CommandResult>
  setActivityDetail(key: string | null): Promise<void>
  installHelper(): Promise<void>
  retryLink(): Promise<void>
  setSettings(patch: Partial<WindowSettings>): Promise<WindowSettings>
  installedPrograms(): Promise<Program[]>
  /** The program of the file picked in the dialog, or the name of a file that is none; null when cancelled. */
  pickProgram(): Promise<Dropped | null>
  resolveDropped(files: File[]): Promise<Dropped[]>
  icon(exe: string): Promise<string | null>
  openLogin(url: string): Promise<boolean>
  saveDiagnostics(): Promise<{ saved: boolean; error?: HelperError }>
  quit(): Promise<void>
}

/** IPC channel names: requests go through ipcMain.handle, the three pushes through webContents.send. */
export const IPC = {
  snapshot: 'splitlane:snapshot',
  command: 'splitlane:command',
  setActivityDetail: 'splitlane:set-activity-detail',
  installHelper: 'splitlane:install-helper',
  retryLink: 'splitlane:retry-link',
  setSettings: 'splitlane:set-settings',
  installedPrograms: 'splitlane:installed-programs',
  pickProgram: 'splitlane:pick-program',
  resolveDropped: 'splitlane:resolve-dropped',
  icon: 'splitlane:icon',
  openLogin: 'splitlane:open-login',
  saveDiagnostics: 'splitlane:save-diagnostics',
  quit: 'splitlane:quit',
  /** main to page: a new Snapshot */
  snapshotChanged: 'splitlane:snapshot-changed',
  /** main to page: an Activity event of the helper */
  activity: 'splitlane:activity',
  /** main to page: «Настройки…» in the tray menu, no data */
  openSettings: 'splitlane:open-settings'
} as const
