import { win32 } from 'node:path'
import type { Program } from '../shared/programs'

/**
 * Games of Steam and Epic (spec 9.3): their launchers put `.url` shortcuts, not `.lnk`, in the Start menu and on the
 * desktop, so the games are read from the launchers' own files. A folder rule on the exe found here covers its folder
 * and the folders inside (spec 8.2), which is what routes the game. No electron here: the disk and PowerShell come in.
 */

// ---- the disk, as the games need it ----

export interface DirEntry {
  name: string
  dir: boolean
}

export interface GamesFs {
  /** The bytes of a file; throws when it cannot be read. */
  readBytes(path: string): Promise<Uint8Array>
  /** The files and folders in a folder (other entries left out); throws when it cannot be read. */
  readDir(path: string): Promise<DirEntry[]>
  /** The size of a file in bytes, or null when there is no such file. */
  fileSize(path: string): Promise<number | null>
}

const DRIVE_PATH = /^[A-Za-z]:\\/

function errText(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

function trimSlash(p: string): string {
  const t = p.replace(/\\+$/, '')
  return /^[A-Za-z]:$/.test(t) ? `${t}\\` : t
}

function child(dir: string, name: string): string {
  return dir.endsWith('\\') ? dir + name : `${dir}\\${name}`
}

function sameText(a: string, b: string): boolean {
  return a.toLowerCase() === b.toLowerCase()
}

/** Paths without repeats (case-insensitive), the first spelling kept. */
function uniquePaths(paths: string[]): string[] {
  const seen = new Set<string>()
  return paths.filter((p) => {
    const key = p.toLowerCase()
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

// ---- text files ----

const UTF8 = new TextDecoder('utf-8', { fatal: true })
const UTF16 = new TextDecoder('utf-16le')
const ANSI = new TextDecoder('latin1')

/** A text file in UTF-8 (with or without a BOM), UTF-16 with a BOM, or ANSI: the ASCII part always survives. */
export function decodeText(bytes: Uint8Array): string {
  if (bytes.length >= 2 && bytes[0] === 0xff && bytes[1] === 0xfe) return UTF16.decode(bytes.subarray(2))
  const body = bytes.length >= 3 && bytes[0] === 0xef && bytes[1] === 0xbb && bytes[2] === 0xbf ? bytes.subarray(3) : bytes
  try {
    return UTF8.decode(body)
  } catch {
    return ANSI.decode(body)
  }
}

// ---- Valve KeyValues (libraryfolders.vdf, appmanifest_*.acf) ----

export type KvValue = string | KvObject
export interface KvObject {
  [key: string]: KvValue
}

type Token = { kind: 'open' } | { kind: 'close' } | { kind: 'text'; text: string }

const ESCAPES: Record<string, string> = { '\\': '\\', '"': '"', n: '\n', t: '\t' }
const MAX_KV_DEPTH = 32

function tokenize(s: string): Token[] {
  const out: Token[] = []
  let i = 0
  while (i < s.length) {
    const c = s[i]
    if (/\s/.test(c) || c === '\ufeff') {
      i++
    } else if (c === '/' && s[i + 1] === '/') {
      const nl = s.indexOf('\n', i)
      i = nl < 0 ? s.length : nl + 1
    } else if (c === '{' || c === '}') {
      out.push({ kind: c === '{' ? 'open' : 'close' })
      i++
    } else if (c === '[') {
      // a condition such as [$WIN32] after a value: ignored
      const end = s.indexOf(']', i)
      if (end < 0) throw new Error('KeyValues: unterminated condition')
      i = end + 1
    } else if (c === '"') {
      let text = ''
      i++
      for (;;) {
        if (i >= s.length) throw new Error('KeyValues: unterminated string')
        const ch = s[i]
        if (ch === '"') {
          i++
          break
        }
        const esc = ch === '\\' ? ESCAPES[s[i + 1]] : undefined
        if (esc !== undefined) {
          text += esc
          i += 2
        } else {
          text += ch
          i++
        }
      }
      out.push({ kind: 'text', text })
    } else {
      // an unquoted token runs up to a space, a brace or a quote
      let j = i
      while (j < s.length && !/[\s{}"]/.test(s[j])) j++
      out.push({ kind: 'text', text: s.slice(i, j) })
      i = j
    }
  }
  return out
}

/** Valve's KeyValues text: keys lower-cased, `\\` `\"` `\n` `\t` unescaped. Throws on a broken file. */
export function parseKeyValues(text: string): KvObject {
  const tokens = tokenize(text)
  let i = 0
  const object = (depth: number): KvObject => {
    if (depth > MAX_KV_DEPTH) throw new Error('KeyValues: nested too deep')
    const obj: KvObject = Object.create(null) as KvObject
    for (;;) {
      const key = tokens[i++]
      if (key === undefined) {
        if (depth > 0) throw new Error('KeyValues: missing }')
        return obj
      }
      if (key.kind === 'close') {
        if (depth === 0) throw new Error('KeyValues: unexpected }')
        return obj
      }
      if (key.kind === 'open') throw new Error('KeyValues: unexpected {')
      const value = tokens[i++]
      if (value === undefined || value.kind === 'close') throw new Error(`KeyValues: no value for "${key.text}"`)
      obj[key.text.toLowerCase()] = value.kind === 'open' ? object(depth + 1) : value.text
    }
  }
  return object(0)
}

/** The library folders of libraryfolders.vdf: `"0" { "path" "…" }` now, `"1" "…"` in old files. */
export function libraryFolders(vdf: KvObject): string[] {
  const root = vdf['libraryfolders']
  if (typeof root !== 'object') return []
  const out: string[] = []
  for (const [key, v] of Object.entries(root)) {
    const p = typeof v === 'object' ? v['path'] : /^\d+$/.test(key) ? v : undefined
    if (typeof p === 'string' && DRIVE_PATH.test(p)) out.push(trimSlash(p))
  }
  return out
}

/** A plain folder name: nothing that leads out of steamapps\common. */
function folderName(s: string): boolean {
  return s.trim() !== '' && s !== '.' && s !== '..' && !/[\\/:]/.test(s)
}

/** StateFlags of an appmanifest: bit 4 is «fully installed»; without the value the exe on disk decides. */
const INSTALLED_FLAG = 4

/** Id, name and folder (under steamapps\common) of an appmanifest_<id>.acf of a fully installed game. */
export function appManifest(acf: KvObject): { appId: string; name: string; installDir: string } | null {
  const state = acf['appstate']
  if (typeof state !== 'object') return null
  const { appid, name, installdir, stateflags } = state
  if (typeof appid !== 'string' || !/^\d+$/.test(appid)) return null
  if (typeof name !== 'string' || name.trim() === '') return null
  if (typeof installdir !== 'string' || !folderName(installdir)) return null
  if (stateflags !== undefined) {
    if (typeof stateflags !== 'string' || !/^\d+$/.test(stateflags)) return null
    if ((Number(stateflags) & INSTALLED_FLAG) === 0) return null // a first download that still runs
  }
  return { appId: appid, name: name.trim(), installDir: installdir }
}

// ---- Steam ----

export const DEFAULT_STEAM_ROOT = 'C:\\Program Files (x86)\\Steam'
const MANIFEST_FILE = /^appmanifest_(\d+)\.acf$/i

/**
 * SteamPath of the registry (`c:/program files (x86)/steam`), printed by PowerShell in UTF-8: reg.exe prints in the
 * OEM code page and would garble a non-ASCII folder. ASCII only.
 */
export const STEAM_PATH_SCRIPT =
  "(Get-ItemProperty -LiteralPath 'HKCU:\\Software\\Valve\\Steam' -ErrorAction SilentlyContinue).SteamPath"

/** The Steam folder in the output of STEAM_PATH_SCRIPT; null when there is none or it came garbled. */
export function steamPathOf(stdout: string): string | null {
  const p = stdout.replace(/^\ufeff/, '').trim().replace(/\//g, '\\')
  if (!DRIVE_PATH.test(p) || p.includes('\ufffd')) return null
  return trimSlash(p)
}

// the installers, crash reporters, anti-cheat services and browser helpers shipped next to a game, by whole names
// (a game called «Crashlands» stays); vconsole is the developer console of Source 2 games, larger than the game's own
// exe beside it (CS2: vconsole2.exe and cs2.exe)
const HELPER_EXES: readonly RegExp[] = [
  /^unins/,
  /crash_?handler|crashpad|crash_?report(er)?|errorreporter/,
  /^vc_?redist/,
  /^dxsetup/,
  /^setup\.exe$/,
  /^vconsole/,
  /easyanticheat/,
  /^be_?service/,
  /launcherhelper/,
  /cefprocess/,
  /webhelper/
]
// folders that hold only installers and anti-cheat, by exact name
const HELPER_DIRS: ReadonlySet<string> = new Set([
  '_commonredist',
  'redist',
  '_redist',
  'redistributables',
  'vcredist',
  'directx',
  'dotnet',
  'easyanticheat',
  'battleye',
  '__installer'
])
const MAX_EXE_DEPTH = 5
const MIN_WORD = 3

function helperExe(name: string): boolean {
  const lower = name.toLowerCase()
  return HELPER_EXES.some((re) => re.test(lower))
}

/** Letters and digits only, lower-cased: «Orbit Racer: Deluxe» → «orbitracerdeluxe». */
function plainName(s: string): string {
  return s.toLowerCase().replace(/[^\p{L}\p{N}]/gu, '')
}

/** What an exe of the game may be called: the folder and game names whole, and their words of 3 letters and more. */
function gameNames(names: readonly string[]): Set<string> {
  const out = new Set<string>()
  for (const n of names) {
    const whole = plainName(n)
    if (whole !== '') out.add(whole)
    for (const word of n.split(/[^\p{L}\p{N}]+/u)) {
      if (word.length >= MIN_WORD) out.add(plainName(word))
    }
  }
  return out
}

/** The exe of one level: one named after the game, else the largest; on equal sizes the first by path. */
async function pickExe(fs: GamesFs, exes: string[], names: Set<string>): Promise<string> {
  const sorted = [...exes].sort((a, b) => (a.toLowerCase() < b.toLowerCase() ? -1 : a.toLowerCase() > b.toLowerCase() ? 1 : 0))
  const named = sorted.filter((p) => names.has(plainName(win32.basename(p).replace(/\.exe$/i, ''))))
  const pool = named.length > 0 ? named : sorted
  const sizes = await Promise.all(pool.map(async (p) => (await fs.fileSize(p)) ?? -1))
  let best = 0
  for (let k = 1; k < pool.length; k++) if (sizes[k] > sizes[best]) best = k
  return pool[best]
}

/**
 * The exe of a game folder: the shallowest .exe (up to five folders down) that is not a helper (installer,
 * redistributable, crash reporter, anti-cheat, by whole names); helper folders are not searched. Of several on that
 * level the one named after the game (`names`: its folder and name) wins, else the largest. When every exe found looks
 * like a helper, the shallowest of them after all. Null when there is none.
 */
export async function gameExe(fs: GamesFs, folder: string, names: readonly string[] = []): Promise<string | null> {
  const wanted = gameNames(names)
  let level = [trimSlash(folder)]
  let firstHelpers: string[] | null = null
  for (let depth = 0; depth <= MAX_EXE_DEPTH && level.length > 0; depth++) {
    const listed = await Promise.all(
      level.map(async (dir) => {
        try {
          return { dir, entries: await fs.readDir(dir) }
        } catch {
          return { dir, entries: [] }
        }
      })
    )
    const exes: string[] = []
    const helpers: string[] = []
    const next: string[] = []
    for (const { dir, entries } of listed) {
      for (const e of entries) {
        const p = child(dir, e.name)
        if (e.dir) {
          if (!HELPER_DIRS.has(e.name.toLowerCase())) next.push(p)
        } else if (e.name.toLowerCase().endsWith('.exe')) {
          ;(helperExe(e.name) ? helpers : exes).push(p)
        }
      }
    }
    if (exes.length > 0) return pickExe(fs, exes, wanted)
    if (firstHelpers === null && helpers.length > 0) firstHelpers = helpers
    level = next
  }
  return firstHelpers === null ? null : pickExe(fs, firstHelpers, wanted)
}

/** The game of appmanifest_<appId>.acf in a library: its exe and name, or null when it is missing or has no exe. */
async function steamGameIn(fs: GamesFs, library: string, appId: string): Promise<Program | null> {
  let manifest: ReturnType<typeof appManifest>
  try {
    manifest = appManifest(parseKeyValues(decodeText(await fs.readBytes(child(library, `steamapps\\appmanifest_${appId}.acf`)))))
  } catch {
    return null
  }
  if (manifest === null) return null
  const folder = child(library, `steamapps\\common\\${manifest.installDir}`)
  const exe = await gameExe(fs, folder, [manifest.installDir, manifest.name])
  return exe === null ? null : { exe, name: manifest.name }
}

export interface GamesDeps {
  fs: GamesFs
  /** Runs a PowerShell script that prints UTF-8 and gives its output; throws when it fails. */
  powershell(script: string): Promise<string>
  /** %ProgramData% (the Epic manifests live there); undefined when not set. */
  programData: string | undefined
  log(message: string): void
}

/** The libraries of a Steam folder: those of its libraryfolders.vdf and itself; null when its steamapps is not there. */
async function librariesOf(fs: GamesFs, root: string): Promise<string[] | null> {
  let listed: string[] = []
  try {
    listed = libraryFolders(parseKeyValues(decodeText(await fs.readBytes(child(root, 'steamapps\\libraryfolders.vdf')))))
  } catch {
    try {
      await fs.readDir(child(root, 'steamapps'))
    } catch {
      return null
    }
  }
  return uniquePaths([...listed, root])
}

function hasPath(paths: readonly string[], p: string): boolean {
  return paths.some((x) => sameText(x, p))
}

interface SteamGame {
  appId: string
  program: Program
}

async function steamGamesIn(fs: GamesFs, libraries: readonly string[]): Promise<SteamGame[]> {
  const out: SteamGame[] = []
  const seen = new Set<string>()
  for (const library of libraries) {
    let entries: DirEntry[]
    try {
      entries = await fs.readDir(child(library, 'steamapps'))
    } catch {
      continue // a library on a drive that is not connected now
    }
    const ids = entries.flatMap((e) => {
      const m = e.dir ? null : MANIFEST_FILE.exec(e.name)
      return m === null || seen.has(m[1]) ? [] : [m[1]]
    })
    const found = await Promise.all(ids.map(async (appId) => ({ appId, program: await steamGameIn(fs, library, appId) })))
    for (const { appId, program } of found) {
      if (program === null || seen.has(appId)) continue
      seen.add(appId)
      out.push({ appId, program })
    }
  }
  return out
}

// ---- Epic ----

export interface EpicGame {
  appName: string
  catalogItemId: string
  namespace: string
  program: Program
}

function text(x: unknown): string {
  return typeof x === 'string' ? x : ''
}

/** A manifest (*.item) of the Epic launcher: the game and its exe, or null for an unfinished install or no exe. */
export function epicGame(json: string): EpicGame | null {
  let data: unknown
  try {
    data = JSON.parse(json)
  } catch {
    return null
  }
  if (typeof data !== 'object' || data === null || Array.isArray(data)) return null
  const o = data as Record<string, unknown>
  if (o.bIsIncompleteInstall === true) return null
  const name = o.DisplayName
  const location = o.InstallLocation
  const launch = o.LaunchExecutable
  if (typeof name !== 'string' || name.trim() === '') return null
  if (typeof location !== 'string' || !DRIVE_PATH.test(location)) return null
  if (typeof launch !== 'string' || launch.trim() === '') return null
  const base = win32.normalize(location)
  const prefix = base.endsWith('\\') ? base : `${base}\\`
  const exe = win32.join(base, launch)
  // the exe lies inside the install folder
  if (!exe.toLowerCase().startsWith(prefix.toLowerCase()) || !exe.toLowerCase().endsWith('.exe')) return null
  return {
    appName: text(o.AppName),
    catalogItemId: text(o.CatalogItemId),
    namespace: text(o.CatalogNamespace),
    program: { exe, name: name.trim() }
  }
}

function epicManifestsDir(programData: string | undefined): string | null {
  if (programData === undefined || !DRIVE_PATH.test(programData)) return null
  return child(trimSlash(programData), 'Epic\\EpicGamesLauncher\\Data\\Manifests')
}

/** The installed Epic games whose exe is there; throws when the manifest folder cannot be read. */
async function readEpicGames(d: GamesDeps): Promise<EpicGame[]> {
  const dir = epicManifestsDir(d.programData)
  if (dir === null) return []
  const items = (await d.fs.readDir(dir)).filter((e) => !e.dir && e.name.toLowerCase().endsWith('.item'))
  const games = await Promise.all(
    items.map(async (e) => {
      try {
        const g = epicGame(decodeText(await d.fs.readBytes(child(dir, e.name))))
        return g !== null && (await d.fs.fileSize(g.program.exe)) !== null ? g : null
      } catch {
        return null
      }
    })
  )
  return games.filter((g): g is EpicGame => g !== null)
}

/** `<appName>`, or `<namespace>:<catalogItemId>:<appName>`: the app name first, then the catalog item. */
function findEpic(games: EpicGame[], id: string): EpicGame | undefined {
  const parts = id.split(':')
  if (parts.length === 3) {
    const [, item, appName] = parts
    return (
      games.find((g) => appName !== '' && sameText(g.appName, appName)) ??
      games.find((g) => item !== '' && sameText(g.catalogItemId, item))
    )
  }
  return games.find((g) => sameText(g.appName, id))
}

// ---- .url shortcuts ----

export type GameLink = { store: 'steam'; appId: string } | { store: 'epic'; id: string }

const MAX_URL_FILE_BYTES = 64 * 1024
const STEAM_LINK = /^steam:\/\/(?:rungameid|run)\/(\d{1,20})(?:[/?#]|$)/i
const EPIC_LINK = /^com\.epicgames\.launcher:\/\/apps\/([^/?#]+)/i

/** The URL of an Internet Shortcut (`[InternetShortcut]`, `URL=…`), or null when the text is none. */
export function urlOfShortcut(text: string): string | null {
  let inSection = false
  for (const raw of text.split(/\r\n|\r|\n/)) {
    const line = raw.trim()
    if (line.startsWith('[')) {
      inSection = /^\[InternetShortcut\]$/i.test(line)
      continue
    }
    if (!inSection) continue
    const m = /^URL\s*=(.*)$/i.exec(line)
    if (m !== null) {
      const url = m[1].trim()
      return url === '' ? null : url
    }
  }
  return null
}

/** `steam://rungameid/<appid>`, `steam://run/<appid>`, `com.epicgames.launcher://apps/<id>`; anything else is null. */
export function gameLink(url: string): GameLink | null {
  const u = url.trim()
  const steam = STEAM_LINK.exec(u)
  if (steam !== null) return { store: 'steam', appId: steam[1] }
  const epic = EPIC_LINK.exec(u)
  if (epic === null) return null
  let id: string
  try {
    id = decodeURIComponent(epic[1])
  } catch {
    return null
  }
  return id === '' ? null : { store: 'epic', id }
}

// ---- the games of this PC ----

export class Games {
  private steam: Promise<SteamGame[]> | null = null
  private epic: Promise<EpicGame[]> | null = null
  private registryRoot: Promise<string | null> | null = null

  constructor(private readonly d: GamesDeps) {}

  /** The Steam and Epic games, read once per window run; a store that cannot be read gives none (logged). */
  async installed(): Promise<Program[]> {
    const [steam, epic] = await Promise.all([this.steamList(), this.epicList()])
    return [...steam.map((g) => g.program), ...epic.map((g) => g.program)]
  }

  /**
   * The program of a game link, null when the game is not found. `fresh` reads the launchers' files again (a drop:
   * the game may be newer than the list); otherwise the lists of this run answer.
   */
  async program(link: GameLink, fresh: boolean): Promise<Program | null> {
    try {
      if (link.store === 'steam') {
        if (!fresh) return (await this.steamList()).find((g) => g.appId === link.appId)?.program ?? null
        const libraries = await this.steamLibraries()
        const extra = hasPath(libraries, DEFAULT_STEAM_ROOT) ? [] : [DEFAULT_STEAM_ROOT]
        for (const library of [...libraries, ...extra]) {
          const p = await steamGameIn(this.d.fs, library, link.appId)
          if (p !== null) return p
        }
        return null
      }
      const games = fresh ? await readEpicGames(this.d) : await this.epicList()
      return findEpic(games, link.id)?.program ?? null
    } catch (e) {
      this.d.log(`programs: ${link.store} link: ${errText(e)}`)
      return null
    }
  }

  /** A `.url` file: the program of its game, 'game' for a game that is not found, null for no game shortcut. Never throws. */
  async readUrl(path: string, fresh: boolean): Promise<Program | 'game' | null> {
    let link: GameLink | null
    try {
      const size = await this.d.fs.fileSize(path)
      if (size === null || size > MAX_URL_FILE_BYTES) return null
      const url = urlOfShortcut(decodeText(await this.d.fs.readBytes(path)))
      link = url === null ? null : gameLink(url)
    } catch {
      return null
    }
    if (link === null) return null
    return (await this.program(link, fresh)) ?? 'game'
  }

  /** SteamPath of the registry, read once per run; null when there is none, it came garbled or PowerShell failed. */
  private steamRegistryRoot(): Promise<string | null> {
    this.registryRoot ??= this.d.powershell(STEAM_PATH_SCRIPT).then(
      (out) => {
        const root = steamPathOf(out)
        if (root === null && out.trim() !== '') this.d.log('programs: Steam: SteamPath of the registry is not usable')
        return root
      },
      (e: unknown) => {
        this.d.log(`programs: Steam: cannot read SteamPath: ${errText(e)}`)
        return null
      }
    )
    return this.registryRoot
  }

  /** The libraries of the Steam folder of the registry; when its steamapps is not there, of the usual folder. */
  private async steamLibraries(): Promise<string[]> {
    const registry = await this.steamRegistryRoot()
    for (const root of uniquePaths(registry === null ? [DEFAULT_STEAM_ROOT] : [registry, DEFAULT_STEAM_ROOT])) {
      const libraries = await librariesOf(this.d.fs, root)
      if (libraries !== null) return libraries
    }
    this.d.log('programs: Steam: no Steam folder found')
    return []
  }

  /** The games of all libraries; when they hold none, the usual Steam folder is tried as one more library. */
  private async readSteamGames(): Promise<SteamGame[]> {
    const libraries = await this.steamLibraries()
    const games = await steamGamesIn(this.d.fs, libraries)
    if (games.length > 0 || hasPath(libraries, DEFAULT_STEAM_ROOT)) return games
    return steamGamesIn(this.d.fs, [DEFAULT_STEAM_ROOT])
  }

  private steamList(): Promise<SteamGame[]> {
    this.steam ??= this.readSteamGames().catch((e: unknown) => {
      this.d.log(`programs: Steam: ${errText(e)}`)
      return []
    })
    return this.steam
  }

  private epicList(): Promise<EpicGame[]> {
    this.epic ??= readEpicGames(this.d).catch((e: unknown) => {
      this.d.log(`programs: Epic: ${errText(e)}`)
      return []
    })
    return this.epic
  }
}
