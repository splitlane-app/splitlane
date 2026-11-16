import type { Lang } from './i18n'
import type { Dropped, Program } from './programs'
import type { Activity, AppRule, HelperError, HelperState, Lane, ProxyAddResult } from './protocol'
import type { WindowSettings } from './settings'
import type { InstallOutcome, LinkStatus } from './status'
import type { UpdateState } from './update'

/** The install of the helper started from the window (spec 4.3). */
export type InstallState = { kind: 'idle' } | { kind: 'running' } | { kind: 'done'; outcome: InstallOutcome }

export interface AppInfo {
  version: string
  installed: boolean
}

/** Everything the page shows apart from the activity; sent on every change. */
export interface Snapshot {
  link: LinkStatus
  state: HelperState | null
  settings: WindowSettings
  /** The language in force: the setting resolved against the language of Windows. */
  language: Lang
  install: InstallState
  /** The window's own updates (stage 2 part 2). */
  update: UpdateState
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
  | { method: 'proxy.remove'; id: string }
  | { method: 'proxy.select'; id: string }

/** A lost link (closed pipe, no answer in time) is `{ code: 'internal', message: 'link closed' }`. */
export type CommandResult = { ok: true } | { ok: false; error: HelperError }

/**
 * The answer of proxy.add and proxy.refresh as the page sees it: what the helper took and refused, the helper's
 * own error, or `cancelled` (the file dialog was closed). A text that is too long is a `done` with a line-0
 * `too_long`, so that the screen shows one kind of answer.
 */
export type ProxyOutcome = { kind: 'done'; result: ProxyAddResult } | { kind: 'failed'; error: HelperError } | { kind: 'cancelled' }

/** The steps of «Свой VPS»: the first five are the subcommand's, `add` is the window's own (proxy.add and proxy.select). */
export type VpsStep = 'connect' | 'system' | 'install' | 'harden' | 'subscription' | 'add'
export type VpsStepState = 'start' | 'done' | 'skip' | 'warn' | 'failed'
export interface VpsEvent {
  step: VpsStep
  state: VpsStepState
  detail?: string
}
/** `address` is as the person typed it; the main process checks it and sends the bare host and the port apart. */
export interface VpsRequest {
  address: string
  user: string
  password?: string
  useKey: boolean
  keyPassphrase?: string
}
export type VpsOutcome =
  | { kind: 'done'; servers: number; added: number; subscriptionId: string | null; warnings: string[] }
  | { kind: 'failed'; code: string; detail?: string }
  | { kind: 'cancelled' }

export const LINK_CLOSED: HelperError = { code: 'internal', message: 'link closed' }

/**
 * The window's refusal of routing.enable while a server setup of «Свой VPS» runs (from the page, the tray or the
 * auto-enable): a tunnel through the server being set up would cut the setup's SSH. Told apart by its message.
 */
export const SETUP_RUNNING: HelperError = { code: 'busy', message: 'a server setup is running' }

export function isSetupRunning(e: HelperError): boolean {
  return e.code === SETUP_RUNNING.code && e.message === SETUP_RUNNING.message
}

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
  /** Share links, a WireGuard config or a subscription address, as typed or pasted. */
  proxyAdd(text: string): Promise<ProxyOutcome>
  /** Opens the file dialog for a WireGuard `.conf`; the main process reads it, the path never reaches the page or the helper. */
  proxyAddFile(): Promise<ProxyOutcome>
  /** Downloads a subscription again; `id` is the id of a subscription. */
  proxyRefresh(id: string): Promise<ProxyOutcome>
  /**
   * The address of a subscription (`id` is a subscription's), to show it as a QR code for a phone: a secret, kept
   * out of the state, the logs and the page's storage by the caller.
   */
  proxyReveal(id: string): Promise<{ address: string } | { error: HelperError }>
  /** The dialog for an SSH key: the key stays in the main process, the page gets only its name. */
  vpsPickKey(): Promise<{ name: string } | { error: 'too_large' | 'not_text' } | null>
  vpsForgetKey(): Promise<void>
  /** Sets up the server (one at a time); the progress comes through onVpsEvent. */
  vpsSetup(req: VpsRequest): Promise<VpsOutcome>
  vpsCancel(): Promise<void>
  onVpsEvent(cb: (e: VpsEvent) => void): () => void
  /** The window's version, and whether it runs from the installed folder (only then it can repair the helper). */
  appInfo(): Promise<AppInfo>
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
  /** A manual check for a new version; the result comes in the snapshot. */
  checkUpdates(): Promise<void>
  installUpdate(): Promise<void>
  cancelUpdate(): Promise<void>
  /** «Позже»: hides the offer of this version. */
  updateLater(): Promise<void>
  /** Closes a result banner. */
  dismissUpdate(): Promise<void>
  /** Opens the release page of the current state; false when there is none or it cannot be opened. */
  openUpdateNotes(): Promise<boolean>
}

/** IPC channel names: requests go through ipcMain.handle, the three pushes through webContents.send. */
export const IPC = {
  snapshot: 'splitlane:snapshot',
  command: 'splitlane:command',
  setActivityDetail: 'splitlane:set-activity-detail',
  installHelper: 'splitlane:install-helper',
  proxyAdd: 'splitlane:proxy-add',
  proxyAddFile: 'splitlane:proxy-add-file',
  proxyRefresh: 'splitlane:proxy-refresh',
  proxyReveal: 'splitlane:proxy-reveal',
  vpsPickKey: 'splitlane:vps-pick-key',
  vpsForgetKey: 'splitlane:vps-forget-key',
  vpsSetup: 'splitlane:vps-setup',
  vpsCancel: 'splitlane:vps-cancel',
  appInfo: 'splitlane:app-info',
  retryLink: 'splitlane:retry-link',
  setSettings: 'splitlane:set-settings',
  installedPrograms: 'splitlane:installed-programs',
  pickProgram: 'splitlane:pick-program',
  resolveDropped: 'splitlane:resolve-dropped',
  icon: 'splitlane:icon',
  openLogin: 'splitlane:open-login',
  saveDiagnostics: 'splitlane:save-diagnostics',
  quit: 'splitlane:quit',
  checkUpdates: 'splitlane:check-updates',
  installUpdate: 'splitlane:install-update',
  cancelUpdate: 'splitlane:cancel-update',
  updateLater: 'splitlane:update-later',
  dismissUpdate: 'splitlane:dismiss-update',
  openUpdateNotes: 'splitlane:open-update-notes',
  /** main to page: a new Snapshot */
  snapshotChanged: 'splitlane:snapshot-changed',
  /** main to page: an Activity event of the helper */
  activity: 'splitlane:activity',
  /** main to page: a VpsEvent of the server setup */
  vpsEvent: 'splitlane:vps-event',
  /** main to page: «Настройки…» in the tray menu, no data */
  openSettings: 'splitlane:open-settings'
} as const
