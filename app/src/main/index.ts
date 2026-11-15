import { execFile } from 'node:child_process'
import type { Dirent } from 'node:fs'
import { access, copyFile, mkdir, readFile, readdir, rm, stat, writeFile } from 'node:fs/promises'
import { dirname, join, win32 } from 'node:path'
import { app, BrowserWindow, dialog, ipcMain, Menu, nativeTheme, net, shell } from 'electron'
import { resolveLanguage, type Lang } from '../shared/i18n'
import { IPC, type InstallState, type Snapshot } from '../shared/ipc'
import type { Dropped } from '../shared/programs'
import { PIPE_NAME, type HelperError, type HelperState } from '../shared/protocol'
import { DEFAULT_SETTINGS, type WindowSettings } from '../shared/settings'
import { feedFrom, type UpdateState } from '../shared/update'
import type { InstallOutcome, LinkStatus } from '../shared/status'
import { AutoEnable } from './autoEnable'
import { Games, type GamesFs } from './games'
import { IconCache, type IconFs } from './icons'
import { exportPath, mergePrograms, registerIpc, runCommand, toHelperError } from './ipc'
import { HelperLink, SHUTDOWN_TIMEOUT_MS } from './link'
import { APP_USER_MODEL_ID, loginItemSettings, startupLoginItem } from './loginItem'
import { PageCrashReload } from './pageCrash'
import { hopWith, type RequestLike } from './netHop'
import { dialPipe } from './pipe'
import { installedPrograms, pickedFile, resolveDropped, startMenuDirs, type DropReaders, type ProgramsFs } from './programs'
import { SettingsFile } from './settingsFile'
import {
  NOT_INSTALLED_REASON,
  canInstall,
  encodePowerShell,
  helperSourceDir,
  installAfterLink,
  installHelper
} from './setup'
import { STORE_APPS_SCRIPT, StoreAppsCache } from './storeApps'
import { forbiddenSwitch } from './switches'
import { runTask, systemTools, taskRegistered, type Exec } from './task'
import { mainText } from './mainText'
import { AppTray, windowIcon } from './tray'
import { installerOutcome, nodeFs, runInstallerScript, Updater } from './updater'

// The helper serves the installed window without elevation: started with a debugger or V8 switch, it exits at once,
// before any window or the pipe (switches.ts).
const forbidden = app.isPackaged ? forbiddenSwitch((name) => app.commandLine.hasSwitch(name)) : null
if (forbidden !== null) {
  log(`startup refused: the switch --${forbidden} is not allowed`)
  app.exit(1)
}

/** schtasks and whoami answer at once; the Store listing takes about a second. */
const QUICK_EXEC_TIMEOUT_MS = 30_000
const STORE_APPS_TIMEOUT_MS = 60_000
/** diagnostics.export writes the archive before it answers. */
const EXPORT_TIMEOUT_MS = 120_000
const MAX_OUTPUT_BYTES = 64 * 1024 * 1024
const START_MENU_DEPTH = 8

// ---- small system helpers ----

function log(message: string): void {
  console.log(`${new Date().toISOString()} ${message}`)
}

function errText(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

function stripBom(s: string): string {
  return s.charCodeAt(0) === 0xfeff ? s.slice(1) : s
}

function utf8Text(b: Buffer | string | null | undefined): string {
  if (b === null || b === undefined) return ''
  return typeof b === 'string' ? b : b.toString('utf8')
}

/** Runs a program without a shell and without a console window; a non-zero exit code is a result. */
function execText(timeoutMs = 0): Exec {
  return (file, args) =>
    new Promise((resolve, reject) => {
      execFile(
        file,
        args,
        { windowsHide: true, encoding: 'buffer', maxBuffer: MAX_OUTPUT_BYTES, timeout: timeoutMs },
        (err, stdout, stderr) => {
          const out = { stdout: utf8Text(stdout), stderr: utf8Text(stderr) }
          if (err === null) resolve({ code: 0, ...out })
          else if (typeof err.code === 'number') resolve({ code: err.code, ...out })
          else reject(err) // not started, killed by the timeout or too much output
        }
      )
    })
}

async function exists(p: string): Promise<boolean> {
  try {
    await access(p)
    return true
  } catch {
    return false
  }
}

/** Files under a folder, recursively; unreadable folders are skipped. */
async function listFiles(dir: string): Promise<string[]> {
  const out: string[] = []
  const walk = async (d: string, depth: number): Promise<void> => {
    let entries: Dirent[]
    try {
      entries = await readdir(d, { withFileTypes: true })
    } catch {
      return
    }
    for (const e of entries) {
      const p = join(d, e.name)
      if (e.isDirectory()) {
        if (depth < START_MENU_DEPTH) await walk(p, depth + 1)
      } else if (e.isFile()) {
        out.push(p)
      }
    }
  }
  await walk(dir, 0)
  return out
}

function describeLink(s: LinkStatus): string {
  switch (s.kind) {
    case 'ready':
      return `ready (helper ${s.hello.helper}, core ${s.hello.core})`
    case 'restarting':
      return `restarting, attempt ${s.attempt}`
    case 'helper-older':
    case 'helper-newer':
      return `${s.kind} (protocol ${s.helperProtocol})`
    default:
      return s.kind
  }
}

// ---- the app: one instance, %AppData%\SplitLane ----

app.setName('SplitLane')
app.setPath('userData', join(app.getPath('appData'), 'SplitLane'))
app.setAppUserModelId(APP_USER_MODEL_ID)

const startHidden = process.argv.includes('--hidden')
const exec = execText(QUICK_EXEC_TIMEOUT_MS)
/** schtasks, whoami and powershell by full path under %SystemRoot%\System32, never through PATH. */
const tools = systemTools(process.env)

let win: BrowserWindow | null = null
let tray: AppTray | null = null
let updater: Updater | null = null
/** Set by before-quit or the end of the Windows session: the window closes instead of hiding. */
let allowClose = false
/** "Выход" in progress. */
let exiting = false

let settings: WindowSettings = { ...DEFAULT_SETTINGS }
/** Resolved from settings.language at the start and on every change of it. */
let language: Lang = 'en'
let install: InstallState = { kind: 'idle' }
let installing: Promise<void> | null = null
let lastPhase: string | null = null
let firstReady = true

const settingsFile = new SettingsFile(join(app.getPath('userData'), 'settings.json'))
let saving: Promise<void> = Promise.resolve()

const link = new HelperLink({
  dial: () => dialPipe(PIPE_NAME),
  taskRegistered: () => taskRegistered(exec, tools),
  runTask: () => runTask(exec, tools),
  sleep: (ms) => new Promise<void>((r) => setTimeout(r, ms)),
  log
})

const autoEnable = new AutoEnable({
  enable: async () => {
    const r = await runCommand(link, { method: 'routing.enable' })
    if (!r.ok) {
      log(`auto-enable: routing.enable: ${r.error.code}`)
      throw new Error(r.error.code)
    }
  },
  setTimer: (fn, ms) => {
    const t = setTimeout(fn, ms)
    return () => clearTimeout(t)
  },
  now: () => Date.now()
})

const gamesFs: GamesFs = {
  readBytes: (p) => readFile(p),
  readDir: async (p) => {
    const entries = await readdir(p, { withFileTypes: true })
    return entries.flatMap((e) => (e.isDirectory() || e.isFile() ? [{ name: e.name, dir: e.isDirectory() }] : []))
  },
  fileSize: async (p) => {
    try {
      const s = await stat(p)
      return s.isFile() ? s.size : null
    } catch {
      return null
    }
  }
}

/** A PowerShell script that prints UTF-8, so that Cyrillic names and paths survive (no progress records). */
async function powershellText(script: string, timeoutMs: number): Promise<string> {
  const full = [
    '[Console]::OutputEncoding = [System.Text.Encoding]::UTF8',
    "$ProgressPreference = 'SilentlyContinue'",
    script
  ].join('\r\n')
  const r = await execText(timeoutMs)(tools.powershell, encodePowerShell(full))
  if (r.code !== 0) throw new Error(`powershell failed with code ${r.code}`)
  return stripBom(r.stdout)
}

/** Steam and Epic games: listed once per run, like the Store apps; SteamPath through PowerShell in UTF-8. */
const games = new Games({
  fs: gamesFs,
  powershell: (script) => powershellText(script, QUICK_EXEC_TIMEOUT_MS),
  programData: process.env['ProgramData'],
  log
})

const readShortcut: ProgramsFs['readShortcut'] = (lnk) => ({ target: shell.readShortcutLink(lnk).target })

/** The Start menu: its game shortcuts answer from the game lists of this run. */
const programsFs: ProgramsFs = {
  listFiles,
  readShortcut,
  readUrl: (p) => games.readUrl(p, false)
}

/** A drop or a picked file: a game shortcut reads the launchers' files again (the game may be new). */
const dropReaders: DropReaders = {
  readShortcut,
  readUrl: (p) => games.readUrl(p, true)
}

/** Store apps once per run. */
const storeApps = new StoreAppsCache(async () => {
  try {
    return await powershellText(STORE_APPS_SCRIPT, STORE_APPS_TIMEOUT_MS)
  } catch (e) {
    log(`programs: Store apps: ${errText(e)}`)
    throw e
  }
})

const iconFs: IconFs = {
  stat: async (p) => {
    try {
      return { mtimeMs: (await stat(p)).mtimeMs }
    } catch {
      return null
    }
  },
  read: async (p) => {
    try {
      return await readFile(p)
    } catch {
      return null
    }
  },
  write: async (p, b) => {
    await mkdir(dirname(p), { recursive: true })
    await writeFile(p, b)
  }
}

const icons = new IconCache(
  join(app.getPath('userData'), 'icons'),
  async (exe) => {
    const img = await app.getFileIcon(exe, { size: 'normal' })
    if (img.isEmpty()) return null
    const png = img.toPNG()
    return png.length === 0 ? null : { png }
  },
  iconFs
)

// ---- snapshot ----

function updateState(): UpdateState {
  return updater?.state() ?? { kind: 'idle' }
}

function snapshot(): Snapshot {
  return { link: link.status(), state: link.state(), settings, language, install, update: updateState() }
}

function send(channel: string, data: unknown): void {
  if (win !== null && !win.isDestroyed()) win.webContents.send(channel, data)
}

function pushSnapshot(): void {
  send(IPC.snapshotChanged, snapshot())
}

link.on('status', (s) => {
  log(`link: ${describeLink(s)}`)
  install = installAfterLink(install, s)
  if (s.kind === 'ready' && firstReady) {
    const state = link.state()
    if (state !== null) {
      firstReady = false
      autoEnable.onReady(state, settings.enableOnStart)
    }
  }
  tray?.update(s, link.state())
  pushSnapshot()
})

link.on('state', (state: HelperState) => {
  if (state.phase !== lastPhase) {
    lastPhase = state.phase
    log(`state: ${state.phase}${state.reason === null ? '' : ` (${state.reason.code})`}`)
  }
  autoEnable.onState(state)
  tray?.update(link.status(), state)
  pushSnapshot()
})

link.on('activity', (a) => send(IPC.activity, a))

// ---- settings: saves run one after another on a chain ----

function saveSettings(s: WindowSettings): Promise<void> {
  saving = saving
    .then(() => settingsFile.save(s))
    .catch((e: unknown) => log(`settings: cannot save: ${errText(e)}`))
  return saving
}

function applyOpenAtLogin(openAtLogin: boolean): void {
  if (!app.isPackaged) {
    log('settings: open at login is applied only in the built app')
    return
  }
  app.setLoginItemSettings(loginItemSettings(openAtLogin, process.execPath))
}

/** The language in force: the setting against the languages of Windows. */
function resolveLang(): Lang {
  return resolveLanguage(settings.language, app.getPreferredSystemLanguages())
}

async function setSettings(patch: Partial<WindowSettings>): Promise<WindowSettings> {
  settings = { ...settings, ...patch }
  if (patch.openAtLogin !== undefined) applyOpenAtLogin(settings.openAtLogin)
  if (patch.language !== undefined) {
    language = resolveLang()
    tray?.setLanguage(language, link.status(), link.state())
  }
  pushSnapshot()
  await saveSettings(settings)
  return settings
}

// ---- updates of the window (stage 2 part 2) ----

/** The updater; its states go to the page in the snapshot. */
function createUpdater(): Updater {
  const { feed, refused } = feedFrom(process.env['SPLITLANE_UPDATE_FEED'])
  if (refused !== undefined) log(`update: ${refused}`)
  const version = app.getVersion()
  return new Updater({
    appVersion: version,
    feed,
    canUpdate:
      canInstall(app.isPackaged, process.execPath, process.env['ProgramFiles']) ||
      (!app.isPackaged && feed.origin === 'test'),
    hop: (url, signal) =>
      hopWith(
        net.request({ url, method: 'GET', redirect: 'manual' }) as unknown as RequestLike,
        `SplitLane/${version}`,
        signal
      ),
    tempDir: join(app.getPath('temp'), 'SplitLane-update'),
    fs: nodeFs(),
    // the elevated installer is not waited for: powershell exits once the prompt is answered
    runInstaller: async (exe) => {
      const r = await execText()(tools.powershell, encodePowerShell(runInstallerScript(exe)))
      const outcome = installerOutcome(r.code, r.stderr)
      if (typeof outcome === 'object') log(`update: the installer did not start: ${outcome.error}`)
      return outcome
    },
    settings: {
      get: () => settings,
      set: async (patch) => {
        await setSettings(patch)
      }
    },
    lastInstallFile: join(process.env['ProgramData'] ?? 'C:\\ProgramData', 'SplitLane', 'export', 'last-install.txt'),
    clock: {
      now: () => Date.now(),
      setTimeout: (fn, ms) => setTimeout(fn, ms),
      clearTimeout: (h) => clearTimeout(h as NodeJS.Timeout)
    },
    log: (line) => log(`update: ${line}`),
    changed: () => pushSnapshot()
  })
}

/** A manual check from the tray: the window opens on the connection screen, where its result shows. */
function checkUpdatesFromTray(): void {
  void updater?.check(true)
  showWindow()
  send(IPC.openSettings, null)
}

// ---- install (spec 4.3) ----

async function runInstall(): Promise<void> {
  if (!canInstall(app.isPackaged, process.execPath, process.env['ProgramFiles'])) {
    // a copy outside the install folder never starts the Windows prompt
    log('setup: install refused: the window is not installed')
    install = { kind: 'done', outcome: { kind: 'refused', reason: NOT_INSTALLED_REASON } }
    pushSnapshot()
    return
  }
  install = { kind: 'running' }
  pushSnapshot()
  let outcome: InstallOutcome
  try {
    outcome = await installHelper({
      exec: execText(), // waits for the UAC prompt as long as the user takes
      tools,
      sourceDir: helperSourceDir({
        isPackaged: app.isPackaged,
        execPath: process.execPath,
        appPath: app.getAppPath()
      }),
      tempDir: app.getPath('temp'),
      readText: async (p) => stripBom(await readFile(p, 'utf8')),
      exists,
      remove: (p) => rm(p, { force: true })
    })
  } catch (e) {
    outcome = { kind: 'failed', detail: errText(e) }
  }
  log(`setup: install ${outcome.kind}`)
  install = { kind: 'done', outcome }
  pushSnapshot()
  if (outcome.kind !== 'ok') return
  try {
    await runTask(exec, tools)
  } catch (e) {
    log(`setup: ${errText(e)}`) // the link runs the task again when the pipe does not show up
  }
  link.retry()
}

function startInstall(): Promise<void> {
  installing ??= runInstall().finally(() => {
    installing = null
  })
  return installing
}

// ---- dialogs ----

/** The picked file as a program, or its name for the notice «Не программа: …» (a network path, say). */
async function pickProgram(): Promise<Dropped | null> {
  const options = {
    properties: ['openFile' as const],
    // .url: the shortcuts of Steam and Epic games
    filters: [{ name: mainText(language).programsFilter, extensions: ['exe', 'url'] }]
  }
  const r = win !== null ? await dialog.showOpenDialog(win, options) : await dialog.showOpenDialog(options)
  if (r.canceled || r.filePaths.length === 0) return null
  return pickedFile(r.filePaths[0], dropReaders)
}

async function saveDiagnostics(): Promise<{ saved: boolean; error?: HelperError }> {
  let archive: string
  try {
    archive = exportPath(await link.call<unknown>('diagnostics.export', undefined, EXPORT_TIMEOUT_MS))
  } catch (e) {
    return { saved: false, error: toHelperError(e) }
  }
  const options = {
    defaultPath: join(app.getPath('downloads'), win32.basename(archive)),
    filters: [{ name: mainText(language).zipFilter, extensions: ['zip'] }]
  }
  const r = win !== null ? await dialog.showSaveDialog(win, options) : await dialog.showSaveDialog(options)
  if (r.canceled || !r.filePath) return { saved: false }
  try {
    await copyFile(archive, r.filePath)
    return { saved: true }
  } catch (e) {
    return { saved: false, error: { code: 'internal', message: errText(e) } }
  }
}

// ---- window and quit ----

function showWindow(): void {
  if (win === null || win.isDestroyed()) return
  if (win.isMinimized()) win.restore()
  win.show()
  win.focus()
}

/** "Выход": helper.shutdown (it disables splitting and exits), up to 30 s for its answer, then quit. */
async function quit(): Promise<void> {
  if (exiting) return
  exiting = true
  autoEnable.stop()
  log('quit: sending helper.shutdown')
  await link.shutdown(SHUTDOWN_TIMEOUT_MS)
  app.quit()
}

function createWindow(): void {
  const w = new BrowserWindow({
    width: 1100,
    height: 720,
    minWidth: 860,
    minHeight: 560,
    show: false,
    backgroundColor: '#0e1014',
    title: 'SplitLane',
    icon: windowIcon(),
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      sandbox: true,
      nodeIntegration: false
    }
  })
  win = w

  w.webContents.setWindowOpenHandler(() => ({ action: 'deny' }))
  w.webContents.on('will-navigate', (event) => event.preventDefault())
  w.once('ready-to-show', () => {
    if (!startHidden) w.show()
  })
  // closing hides to the tray; the helper and splitting go on (spec 10)
  w.on('close', (event) => {
    if (allowClose) return
    event.preventDefault()
    w.hide()
  })
  w.on('session-end', () => {
    allowClose = true
  })
  w.on('closed', () => {
    if (win === w) win = null
  })
  // a crashed page would leave the window blank: it is reloaded once (pageCrash.ts)
  const crash = new PageCrashReload(() => Date.now())
  w.webContents.on('render-process-gone', (_event, details) => {
    log(`window: the page process is gone (${details.reason}, exit code ${details.exitCode})`)
    if (allowClose || w.isDestroyed() || details.reason === 'clean-exit') return
    if (!crash.onGone(details.reason)) {
      log('window: the page died again soon after its reload: not reloaded again')
      return
    }
    log('window: reloading the page')
    w.webContents.reload()
  })

  const devUrl = process.env['ELECTRON_RENDERER_URL']
  if (!app.isPackaged && devUrl) {
    const url = new URL(devUrl)
    url.searchParams.set('lang', resolveLang())
    void w.loadURL(url.toString())
  } else {
    // the page reads `lang` before its first render: the splash is already in the language in force
    void w.loadFile(join(__dirname, '../renderer/index.html'), { query: { lang: resolveLang() } })
  }
}

async function main(): Promise<void> {
  settings = await settingsFile.load()
  language = resolveLang()
  // the saved autostart again, for the exe of this start (the unpacked folder may have moved)
  const item = startupLoginItem({ isPackaged: app.isPackaged, execPath: process.execPath }, settings.openAtLogin)
  try {
    if (item !== null) app.setLoginItemSettings(item)
  } catch (e) {
    log(`settings: cannot apply open at login: ${errText(e)}`)
  }
  registerIpc({
    ipc: ipcMain,
    accept: (e) => win !== null && !win.isDestroyed() && e.sender === win.webContents,
    link,
    snapshot,
    userCommand: () => autoEnable.stop(),
    installHelper: startInstall,
    // in development the setup screen works with dist\helper, so the window counts as installed
    appInfo: async () => ({
      version: app.getVersion(),
      installed: canInstall(app.isPackaged, process.execPath, process.env['ProgramFiles'])
    }),
    setSettings,
    installedPrograms: async () => {
      const [start, store, played] = await Promise.all([
        installedPrograms(programsFs, startMenuDirs(process.env), language),
        storeApps.get(),
        games.installed()
      ])
      return mergePrograms(language, start, store, played)
    },
    pickProgram,
    resolveDropped: (paths) => resolveDropped(paths, dropReaders),
    icon: (exe) => icons.get(exe),
    openExternal: (url) => shell.openExternal(url),
    saveDiagnostics,
    quit,
    updateState,
    checkUpdates: async () => {
      await updater?.check(true)
    },
    installUpdate: async () => {
      await updater?.update()
    },
    cancelUpdate: () => updater?.cancel(),
    updateLater: () => updater?.later(),
    dismissUpdate: () => updater?.dismiss(),
    log
  })
  tray = new AppTray({
    open: showWindow,
    settings: () => {
      showWindow()
      send(IPC.openSettings, null)
    },
    toggle: (to) => {
      autoEnable.stop()
      void runCommand(link, { method: to === 'enable' ? 'routing.enable' : 'routing.disable' }).then((r) => {
        if (!r.ok) log(`tray: ${to}: ${r.error.code}`)
      })
    },
    checkUpdates: checkUpdatesFromTray,
    quit: () => void quit()
  }, language)
  tray.update(link.status(), link.state())
  // the page is dark only: Windows then draws a dark title bar and frame too
  nativeTheme.themeSource = 'dark'
  createWindow()
  updater = createUpdater()
  void updater.start().catch((e: unknown) => log(`update: cannot start: ${errText(e)}`))
  log(`window started${startHidden ? ' hidden' : ''}`)
  link.start()
}

if (forbidden !== null) {
  // app.exit(1) above ends the process; nothing starts in the meantime
} else if (!app.requestSingleInstanceLock()) {
  app.quit()
} else {
  Menu.setApplicationMenu(null)
  app.on('second-instance', () => showWindow())
  app.on('before-quit', () => {
    allowClose = true
  })
  // the window only hides; it closes for good when the app quits
  app.on('window-all-closed', () => {
    if (allowClose) app.quit()
  })
  app.on('will-quit', () => {
    updater?.stop()
    link.stop()
    tray?.destroy()
    tray = null
  })
  void app.whenReady().then(main).catch((e: unknown) => {
    log(`startup failed: ${errText(e)}`)
    app.quit()
  })
}
