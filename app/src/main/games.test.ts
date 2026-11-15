import { expect, test, vi } from 'vitest'
import {
  appManifest,
  decodeText,
  epicGame,
  gameExe,
  gameLink,
  Games,
  libraryFolders,
  parseKeyValues,
  STEAM_PATH_SCRIPT,
  steamPathOf,
  urlOfShortcut,
  type GamesFs
} from './games'

// ---- a fake disk: files by path (text, or the size of a binary file), case-insensitive like Windows ----

type FakeFile = string | number

function fakeFs(files: Record<string, FakeFile>): GamesFs & { reads: string[] } {
  const byKey = new Map<string, { path: string; file: FakeFile }>()
  for (const [path, file] of Object.entries(files)) byKey.set(path.toLowerCase(), { path, file })
  const reads: string[] = []
  return {
    reads,
    async readBytes(path) {
      reads.push(path)
      const f = byKey.get(path.toLowerCase())
      if (f === undefined) throw new Error(`ENOENT: ${path}`)
      return typeof f.file === 'string' ? new TextEncoder().encode(f.file) : new Uint8Array(f.file)
    },
    async readDir(dir) {
      const prefix = dir.toLowerCase().replace(/\\+$/, '') + '\\'
      const out = new Map<string, boolean>()
      for (const [key, { path }] of byKey) {
        if (!key.startsWith(prefix)) continue
        const rest = path.slice(prefix.length)
        const i = rest.indexOf('\\')
        const name = i < 0 ? rest : rest.slice(0, i)
        out.set(name, i >= 0 || out.get(name) === true)
      }
      if (out.size === 0) throw new Error(`ENOENT: ${dir}`)
      return [...out].map(([name, dir]) => ({ name, dir }))
    },
    async fileSize(path) {
      const f = byKey.get(path.toLowerCase())
      if (f === undefined) return null
      return typeof f.file === 'string' ? new TextEncoder().encode(f.file).length : f.file
    }
  }
}

/** The PowerShell runner: prints SteamPath (null: the value is not there), or fails. */
function fakePowerShell(steamPath: string | null | Error) {
  return vi.fn(async (script: string): Promise<string> => {
    if (script !== STEAM_PATH_SCRIPT) throw new Error('unexpected script')
    if (steamPath instanceof Error) throw steamPath
    return steamPath === null ? '' : `\ufeff${steamPath}\r\n`
  })
}

const ROOT = 'C:\\Program Files (x86)\\Steam'
const LIB = 'D:\\Games\\SteamLibrary'

const LIBRARY_VDF = [
  '"libraryfolders"',
  '{',
  '\t"0"',
  '\t{',
  '\t\t"path"\t\t"C:\\\\Program Files (x86)\\\\Steam"',
  '\t\t"label"\t\t""',
  '\t\t"apps"',
  '\t\t{',
  '\t\t\t"77"\t\t"1024"',
  '\t\t}',
  '\t}',
  '\t"1"',
  '\t{',
  '\t\t"path"\t\t"D:\\\\Games\\\\SteamLibrary"',
  '\t\t"label"\t\t"Big \\"fast\\" disk"',
  '\t\t"apps"',
  '\t\t{',
  '\t\t\t"4242"\t\t"2048"',
  '\t\t}',
  '\t}',
  '}',
  ''
].join('\n')

function manifest(appId: string, name: string, installDir: string, stateFlags: string | null = '4'): string {
  return [
    '"AppState"',
    '{',
    `\t"appid"\t\t"${appId}"`,
    '\t"Universe"\t\t"1"',
    `\t"name"\t\t"${name}"`,
    ...(stateFlags === null ? [] : [`\t"StateFlags"\t\t"${stateFlags}"`]),
    `\t"installdir"\t\t"${installDir}"`,
    '\t"UserConfig"',
    '\t{',
    '\t\t"language"\t\t"russian"',
    '\t}',
    '}',
    ''
  ].join('\r\n')
}

const STEAM_FILES: Record<string, FakeFile> = {
  [`${ROOT}\\steamapps\\libraryfolders.vdf`]: LIBRARY_VDF,
  // Nebula Drift (77) in the Steam folder itself: one exe at the top
  [`${ROOT}\\steamapps\\appmanifest_77.acf`]: manifest('77', 'Nebula Drift', 'Nebula Drift'),
  [`${ROOT}\\steamapps\\common\\Nebula Drift\\Nebula.exe`]: 4000,
  [`${ROOT}\\steamapps\\common\\Nebula Drift\\data\\big.pak`]: 900000,
  // the shared redistributables of Steam: nothing but installers under _CommonRedist, never a game
  [`${ROOT}\\steamapps\\appmanifest_228980.acf`]: manifest('228980', 'Steamworks Common Redistributables', 'Steamworks Shared'),
  [`${ROOT}\\steamapps\\common\\Steamworks Shared\\_CommonRedist\\DotNet\\4.8\\ndp48-x86-x64-allos-enu.exe`]: 95000,
  [`${ROOT}\\steamapps\\common\\Steamworks Shared\\_CommonRedist\\PhysX\\9.13\\PhysX-9.13.exe`]: 90000,
  // Звёздная тактика (4242) in the second library: helpers at the top, two exes one level down
  [`${LIB}\\steamapps\\appmanifest_4242.acf`]: manifest('4242', 'Звёздная тактика', 'Starfall'),
  [`${LIB}\\steamapps\\common\\Starfall\\unins000.exe`]: 70000,
  [`${LIB}\\steamapps\\common\\Starfall\\UnityCrashHandler64.exe`]: 80000,
  [`${LIB}\\steamapps\\common\\Starfall\\readme.txt`]: 10,
  [`${LIB}\\steamapps\\common\\Starfall\\_CommonRedist\\DirectX\\Jun2010\\DXSETUP.exe`]: 90000,
  [`${LIB}\\steamapps\\common\\Starfall\\_CommonRedist\\DotNet\\4.8\\ndp48-x86-x64-allos-enu.exe`]: 95000,
  [`${LIB}\\steamapps\\common\\Starfall\\bin\\tool.exe`]: 100,
  [`${LIB}\\steamapps\\common\\Starfall\\bin\\starfall.exe`]: 9000,
  [`${LIB}\\steamapps\\common\\Starfall\\bin\\x64\\deeper.exe`]: 99999,
  // Helpers Only (91): nothing but installers and anti-cheat in their own folders
  [`${LIB}\\steamapps\\appmanifest_91.acf`]: manifest('91', 'Helpers Only', 'Helpers Only'),
  [`${LIB}\\steamapps\\common\\Helpers Only\\redist\\vcredist_x64.exe`]: 5000,
  [`${LIB}\\steamapps\\common\\Helpers Only\\EasyAntiCheat\\EasyAntiCheat_EOS_Setup.exe`]: 5000,
  [`${LIB}\\steamapps\\common\\Helpers Only\\BattlEye\\BEService_x64.exe`]: 5000,
  // Half Done (92): the first download still runs
  [`${LIB}\\steamapps\\appmanifest_92.acf`]: manifest('92', 'Half Done', 'Half Done', '1026'),
  [`${LIB}\\steamapps\\common\\Half Done\\HalfDone.exe`]: 5000,
  // a broken manifest is skipped
  [`${LIB}\\steamapps\\appmanifest_13.acf`]: '"AppState" { "name" ',
  [`${LIB}\\steamapps\\libraryfolders.vdf`]: '"libraryfolders" {}'
}

const REG_ROOT = 'c:/program files (x86)/steam'

// ---- KeyValues ----

test('parseKeyValues reads libraryfolders.vdf with two libraries and an appmanifest', () => {
  const vdf = parseKeyValues(LIBRARY_VDF)
  expect(libraryFolders(vdf)).toEqual([ROOT, LIB])
  const lib = vdf['libraryfolders']
  if (typeof lib !== 'object') throw new Error('object expected')
  const one = lib['1']
  if (typeof one !== 'object') throw new Error('object expected')
  expect(one['label']).toBe('Big "fast" disk')

  const acf = parseKeyValues(manifest('4242', 'Звёздная тактика', 'Starfall'))
  expect(appManifest(acf)).toEqual({ appId: '4242', name: 'Звёздная тактика', installDir: 'Starfall' })

  // comments, unquoted tokens, keys in any case; the old flat format of libraryfolders.vdf
  const old = parseKeyValues(
    '// written by Steam\n"LibraryFolders"\n{\n\t"TimeNextStatsReport"\t"1700000000"\n\t"1"\t"E:\\\\SteamLibrary\\\\"\n}\n'
  )
  expect(libraryFolders(old)).toEqual(['E:\\SteamLibrary'])
  expect(appManifest(parseKeyValues('AppState { appid 5 name Five INSTALLDIR five }'))).toEqual({
    appId: '5',
    name: 'Five',
    installDir: 'five'
  })

  // a manifest without its folder, or with a folder that leaves steamapps\common, gives nothing
  expect(appManifest(parseKeyValues('"AppState" { "appid" "6" "name" "Six" }'))).toBeNull()
  expect(appManifest(parseKeyValues('"AppState" { "appid" "6" "name" "Six" "installdir" "..\\\\.." }'))).toBeNull()
  expect(() => parseKeyValues('"AppState" { "name" ')).toThrow()
  expect(() => parseKeyValues('"AppState" { "name" "x" } }')).toThrow()
})

test('appManifest takes only fully installed games (StateFlags with 4)', () => {
  const flags = (f: string | null) => appManifest(parseKeyValues(manifest('8', 'Eight', 'Eight', f)))
  expect(flags('4')).not.toBeNull()
  expect(flags('6')).not.toBeNull() // installed, an update waits
  expect(flags('1542')).not.toBeNull() // installed, an update runs
  expect(flags(null)).not.toBeNull() // no flags: the exe decides
  expect(flags('1026')).toBeNull() // the first download runs
  expect(flags('2')).toBeNull()
  expect(flags('soon')).toBeNull()
})

// ---- the Steam folder ----

test('steamPathOf reads the SteamPath that PowerShell prints, garbled text is none', () => {
  expect(/^[\x20-\x7e]*$/.test(STEAM_PATH_SCRIPT)).toBe(true)
  expect(STEAM_PATH_SCRIPT).toContain('HKCU:\\Software\\Valve\\Steam')
  expect(STEAM_PATH_SCRIPT).toContain('SteamPath')

  expect(steamPathOf('\ufeffc:/program files (x86)/steam\r\n')).toBe('c:\\program files (x86)\\steam')
  expect(steamPathOf('d:/игры/steam/\r\n')).toBe('d:\\игры\\steam')
  expect(steamPathOf('')).toBeNull()
  expect(steamPathOf('steam\r\n')).toBeNull()
  // a non-ASCII path decoded in the wrong code page
  expect(steamPathOf('d:/\ufffd\ufffd\ufffd/steam\r\n')).toBeNull()
})

// ---- the executable of a game ----

test('gameExe takes the shallowest exe, skips helpers and breaks a tie by size', async () => {
  const fs = fakeFs(STEAM_FILES)
  expect(await gameExe(fs, `${LIB}\\steamapps\\common\\Starfall`)).toBe(`${LIB}\\steamapps\\common\\Starfall\\bin\\starfall.exe`)
  expect(await gameExe(fs, `${ROOT}\\steamapps\\common\\Nebula Drift`)).toBe(`${ROOT}\\steamapps\\common\\Nebula Drift\\Nebula.exe`)
  // everything in helper folders: nothing
  expect(await gameExe(fs, `${LIB}\\steamapps\\common\\Helpers Only`)).toBeNull()
  expect(await gameExe(fs, `${ROOT}\\steamapps\\common\\Steamworks Shared`)).toBeNull()
  expect(await gameExe(fs, `${LIB}\\steamapps\\common\\Missing`)).toBeNull()

  // five folders down is still searched, six is not
  const deep = fakeFs({ 'D:\\G\\a\\b\\c\\d\\e\\five.exe': 1, 'D:\\H\\a\\b\\c\\d\\e\\f\\six.exe': 1 })
  expect(await gameExe(deep, 'D:\\G')).toBe('D:\\G\\a\\b\\c\\d\\e\\five.exe')
  expect(await gameExe(deep, 'D:\\H')).toBeNull()

  // the largest; on equal sizes the first by name
  const sizes = fakeFs({ 'D:\\L\\small.exe': 10, 'D:\\L\\large.exe': 20 })
  expect(await gameExe(sizes, 'D:\\L')).toBe('D:\\L\\large.exe')
  const tie = fakeFs({ 'D:\\T\\b.exe': 10, 'D:\\T\\a.exe': 10 })
  expect(await gameExe(tie, 'D:\\T')).toBe('D:\\T\\a.exe')

  // a Source 2 game: its developer console is larger than the game, and is passed over
  const source2 = fakeFs({ 'D:\\S\\game\\bin\\win64\\vconsole2.exe': 5000, 'D:\\S\\game\\bin\\win64\\orbit.exe': 3000 })
  expect(await gameExe(source2, 'D:\\S')).toBe('D:\\S\\game\\bin\\win64\\orbit.exe')
})

test('gameExe matches helpers by whole names: games named like them stay', async () => {
  // «crash» in a game's own name is no crash reporter
  const crash = fakeFs({ 'D:\\C\\Crashlands.exe': 100, 'D:\\C\\CrashHandler64.exe': 500, 'D:\\C\\crashpad_handler.exe': 600 })
  expect(await gameExe(crash, 'D:\\C')).toBe('D:\\C\\Crashlands.exe')
  // a folder or an exe with «report» or «setup» in its name
  const report = fakeFs({ 'D:\\R\\Report Studio\\ReportStudio.exe': 100, 'D:\\R\\Setup Tools\\data.bin': 1 })
  expect(await gameExe(report, 'D:\\R')).toBe('D:\\R\\Report Studio\\ReportStudio.exe')
  const directx = fakeFs({ 'D:\\X\\DirectXRacer.exe': 100, 'D:\\X\\DXSETUP.exe': 900, 'D:\\X\\unins000.exe': 900 })
  expect(await gameExe(directx, 'D:\\X')).toBe('D:\\X\\DirectXRacer.exe')
  // the helper folders still are skipped, whatever lies in them
  const folders = fakeFs({
    'D:\\F\\__Installer\\installer.exe': 900,
    'D:\\F\\vcredist\\x.exe': 900,
    'D:\\F\\DirectX\\y.exe': 900,
    'D:\\F\\dotnet\\z.exe': 900,
    'D:\\F\\game\\run.exe': 10
  })
  expect(await gameExe(folders, 'D:\\F')).toBe('D:\\F\\game\\run.exe')
  // when every exe of the shallowest level looks like a helper, the shallowest exe after all
  const onlyHelpers = fakeFs({ 'D:\\O\\setup.exe': 50, 'D:\\O\\UnityCrashHandler64.exe': 40, 'D:\\O\\bin\\deeper.exe': 99 })
  expect(await gameExe(onlyHelpers, 'D:\\O')).toBe('D:\\O\\bin\\deeper.exe')
  const nothingElse = fakeFs({ 'D:\\P\\setup.exe': 50, 'D:\\P\\UnityCrashHandler64.exe': 40 })
  expect(await gameExe(nothingElse, 'D:\\P')).toBe('D:\\P\\setup.exe')
})

test('gameExe prefers the exe named after the game folder or the game', async () => {
  const fs = fakeFs({
    'D:\\N\\Orbit Racer\\Launcher.exe': 9000,
    'D:\\N\\Orbit Racer\\OrbitRacer.exe': 3000,
    'D:\\W\\Orbit Racer\\Launcher.exe': 9000,
    'D:\\W\\Orbit Racer\\orbit.exe': 3000,
    'D:\\U\\Ultra\\Launcher.exe': 9000,
    'D:\\U\\Ultra\\Звездолёт.exe': 3000,
    'D:\\Z\\Zed\\Launcher.exe': 9000,
    'D:\\Z\\Zed\\other.exe': 3000
  })
  // the folder name without spaces and signs
  expect(await gameExe(fs, 'D:\\N\\Orbit Racer', ['Orbit Racer', 'Orbit Racer: Deluxe'])).toBe('D:\\N\\Orbit Racer\\OrbitRacer.exe')
  // a word of the name
  expect(await gameExe(fs, 'D:\\W\\Orbit Racer', ['Orbit Racer', 'Orbit Racer'])).toBe('D:\\W\\Orbit Racer\\orbit.exe')
  // the game's name, Cyrillic too
  expect(await gameExe(fs, 'D:\\U\\Ultra', ['Ultra', 'Звездолёт'])).toBe('D:\\U\\Ultra\\Звездолёт.exe')
  // no match: the largest
  expect(await gameExe(fs, 'D:\\Z\\Zed', ['Zed', 'Zed'])).toBe('D:\\Z\\Zed\\Launcher.exe')
})

// ---- .url shortcuts ----

test('urlOfShortcut and gameLink read Steam, Epic, web and broken shortcuts', () => {
  const steam = '[{000214A0-0000-0000-C000-000000000046}]\r\nProp3=19,0\r\n[InternetShortcut]\r\nIDList=\r\nIconIndex=0\r\nURL=steam://rungameid/4242\r\nIconFile=C:\\Steam\\steam\\games\\x.ico\r\n'
  expect(urlOfShortcut(steam)).toBe('steam://rungameid/4242')
  expect(gameLink('steam://rungameid/4242')).toEqual({ store: 'steam', appId: '4242' })
  expect(gameLink('steam://run/77')).toEqual({ store: 'steam', appId: '77' })
  expect(gameLink('STEAM://RunGameID/77/')).toEqual({ store: 'steam', appId: '77' })

  const epic = '[InternetShortcut]\nURL=com.epicgames.launcher://apps/Peony?action=launch&silent=true\nIconIndex=0\n'
  expect(urlOfShortcut(epic)).toBe('com.epicgames.launcher://apps/Peony?action=launch&silent=true')
  expect(gameLink('com.epicgames.launcher://apps/Peony?action=launch&silent=true')).toEqual({ store: 'epic', id: 'Peony' })
  expect(gameLink('com.epicgames.launcher://apps/ns1%3Ac0ffee%3APeony?action=launch')).toEqual({
    store: 'epic',
    id: 'ns1:c0ffee:Peony'
  })

  // a web link is a shortcut but no game; garbage is neither
  expect(urlOfShortcut('[InternetShortcut]\nURL=https://example.net/\n')).toBe('https://example.net/')
  expect(gameLink('https://example.net/')).toBeNull()
  expect(gameLink('steam://open/friends')).toBeNull()
  expect(gameLink('steam://rungameid/')).toBeNull()
  expect(gameLink('com.epicgames.launcher://store/p/peony')).toBeNull()
  expect(urlOfShortcut('URL=steam://rungameid/4242\n')).toBeNull() // outside [InternetShortcut]
  expect(urlOfShortcut('[InternetShortcut]\nIconIndex=0\n')).toBeNull()
  expect(urlOfShortcut('\u0000\u0001 garbage')).toBeNull()
})

test('decodeText reads UTF-8, ANSI and UTF-16 files', () => {
  expect(decodeText(new Uint8Array([0xef, 0xbb, 0xbf, 0x41, 0xd0, 0x96]))).toBe('AЖ')
  expect(decodeText(new TextEncoder().encode('Игра'))).toBe('Игра')
  // not UTF-8 (an ANSI file): the ASCII part survives
  expect(decodeText(new Uint8Array([0x55, 0x52, 0x4c, 0x3d, 0xc8, 0x0a]))).toMatch(/^URL=.\n$/)
  expect(decodeText(new Uint8Array([0xff, 0xfe, 0x41, 0x00, 0x16, 0x04]))).toBe('AЖ')
})

// ---- Epic ----

const EPIC_DIR = 'C:\\ProgramData\\Epic\\EpicGamesLauncher\\Data\\Manifests'
const LUMEN = 'C:\\Program Files\\Epic Games\\LumenRush'

function epicItem(fields: Record<string, unknown>): string {
  return JSON.stringify(
    {
      FormatVersion: 0,
      bIsIncompleteInstall: false,
      LaunchCommand: '',
      DisplayName: 'Lumen Rush',
      InstallLocation: LUMEN,
      LaunchExecutable: 'Binaries/Win64/LumenRush.exe',
      AppName: 'Peony',
      CatalogNamespace: 'ns1',
      CatalogItemId: 'c0ffee',
      ...fields
    },
    null,
    '\t'
  )
}

const EPIC_FILES: Record<string, FakeFile> = {
  [`${EPIC_DIR}\\0A1B.item`]: epicItem({}),
  [`${LUMEN}\\Binaries\\Win64\\LumenRush.exe`]: 5000,
  // a download in progress
  [`${EPIC_DIR}\\0A1C.item`]: epicItem({
    DisplayName: 'Half There',
    InstallLocation: 'C:\\Program Files\\Epic Games\\HalfThere',
    LaunchExecutable: 'HalfThere.exe',
    AppName: 'Tulip',
    bIsIncompleteInstall: true
  }),
  ['C:\\Program Files\\Epic Games\\HalfThere\\HalfThere.exe']: 10,
  // the game folder was deleted by hand
  [`${EPIC_DIR}\\0A1D.item`]: epicItem({
    DisplayName: 'Gone',
    InstallLocation: 'C:\\Program Files\\Epic Games\\Gone',
    LaunchExecutable: 'Gone.exe',
    AppName: 'Lily'
  }),
  // an add-on has nothing to launch
  [`${EPIC_DIR}\\0A1E.item`]: epicItem({ DisplayName: 'Lumen Rush Soundtrack', LaunchExecutable: '', AppName: 'Daisy' }),
  [`${EPIC_DIR}\\0A1F.item`]: 'not json',
  [`${EPIC_DIR}\\Pending\\x.item`]: epicItem({})
}

test('epicGame reads a manifest of the Epic launcher', () => {
  expect(epicGame(epicItem({}))).toEqual({
    appName: 'Peony',
    catalogItemId: 'c0ffee',
    namespace: 'ns1',
    program: { exe: `${LUMEN}\\Binaries\\Win64\\LumenRush.exe`, name: 'Lumen Rush' }
  })
  expect(epicGame(epicItem({ bIsIncompleteInstall: true }))).toBeNull()
  expect(epicGame(epicItem({ LaunchExecutable: '' }))).toBeNull()
  expect(epicGame(epicItem({ LaunchExecutable: '..\\..\\Windows\\notepad.exe' }))).toBeNull()
  expect(epicGame(epicItem({ InstallLocation: 'Epic Games\\LumenRush' }))).toBeNull()
  expect(epicGame(epicItem({ DisplayName: 7 }))).toBeNull()
  expect(epicGame('[]')).toBeNull()
  expect(epicGame('{')).toBeNull()
})

// ---- the games of this PC ----

function games(files: Record<string, FakeFile>, steamPath: string | null | Error = REG_ROOT) {
  const fs = fakeFs(files)
  const powershell = fakePowerShell(steamPath)
  const log = vi.fn<(m: string) => void>()
  const g = new Games({ fs, powershell, programData: 'C:\\ProgramData', log })
  return { g, fs, powershell, log }
}

const NEBULA = { exe: `${ROOT}\\steamapps\\common\\Nebula Drift\\Nebula.exe`, name: 'Nebula Drift' }
const STARFALL = { exe: `${LIB}\\steamapps\\common\\Starfall\\bin\\starfall.exe`, name: 'Звёздная тактика' }
const LUMEN_RUSH = { exe: `${LUMEN}\\Binaries\\Win64\\LumenRush.exe`, name: 'Lumen Rush' }

test('Games lists Steam and Epic games once per run', async () => {
  const { g, fs, powershell } = games({ ...STEAM_FILES, ...EPIC_FILES })
  const [a, b] = await Promise.all([g.installed(), g.installed()])
  const expected = [NEBULA, STARFALL, LUMEN_RUSH]
  expect(a).toEqual(expected)
  expect(b).toEqual(expected)
  const reads = fs.reads.length
  expect(await g.installed()).toEqual(expected)
  expect(powershell).toHaveBeenCalledTimes(1)
  expect(fs.reads).toHaveLength(reads)
})

test('Games: a source that cannot be read is empty and logged', async () => {
  // no Steam at all (no registry value, no folder) and no Epic manifests
  const none = games({}, null)
  expect(await none.g.installed()).toEqual([])
  expect(none.log).toHaveBeenCalled()

  // Steam without Epic still lists Steam
  const steamOnly = games(STEAM_FILES)
  expect((await steamOnly.g.installed()).map((p) => p.name)).toEqual(['Nebula Drift', 'Звёздная тактика'])

  // no ProgramData: no Epic, no error
  const fs = fakeFs(EPIC_FILES)
  const g = new Games({ fs, powershell: fakePowerShell(null), programData: undefined, log: () => {} })
  expect(await g.installed()).toEqual([])
})

test('Games find the Steam folder: the registry, else the usual folder', async () => {
  const names = async (steamPath: string | null | Error, files: Record<string, FakeFile>) =>
    (await games(files, steamPath).g.installed()).map((p) => p.name)

  // a non-ASCII Steam folder read correctly is used
  const cyrillic: Record<string, FakeFile> = {
    'D:\\Игры\\Steam\\steamapps\\appmanifest_31.acf': manifest('31', 'Comet', 'Comet'),
    'D:\\Игры\\Steam\\steamapps\\common\\Comet\\Comet.exe': 10
  }
  expect(await names('d:/игры/steam', cyrillic)).toEqual(['Comet'])

  // a garbled registry value, a folder that is not there, no value, a failed PowerShell: the usual folder
  expect(await names('d:/\ufffd\ufffd\ufffd\ufffd/steam', STEAM_FILES)).toEqual(['Nebula Drift', 'Звёздная тактика'])
  expect(await names('e:/gone/steam', STEAM_FILES)).toEqual(['Nebula Drift', 'Звёздная тактика'])
  expect(await names(null, STEAM_FILES)).toEqual(['Nebula Drift', 'Звёздная тактика'])
  expect(await names(new Error('powershell failed with code 1'), STEAM_FILES)).toEqual(['Nebula Drift', 'Звёздная тактика'])

  // a registry folder that holds no game: the usual folder is a library too
  const empty: Record<string, FakeFile> = { ...STEAM_FILES, 'E:\\Steam\\steamapps\\libraryfolders.vdf': '"libraryfolders" {}' }
  expect(await names('e:/steam', empty)).toEqual(['Nebula Drift'])
  const fresh = games(empty, 'e:/steam')
  expect(await fresh.g.program({ store: 'steam', appId: '77' }, true)).toEqual(NEBULA)
})

test('Games resolve Steam and Epic links, fresh or from the list of this run', async () => {
  const { g, powershell } = games({ ...STEAM_FILES, ...EPIC_FILES })

  expect(await g.program({ store: 'steam', appId: '4242' }, true)).toEqual(STARFALL)
  expect(await g.program({ store: 'steam', appId: '77' }, true)).toEqual(NEBULA)
  // no manifest, a manifest with nothing to run, a download in progress: unresolved
  expect(await g.program({ store: 'steam', appId: '5555' }, true)).toBeNull()
  expect(await g.program({ store: 'steam', appId: '91' }, true)).toBeNull()
  expect(await g.program({ store: 'steam', appId: '92' }, true)).toBeNull()
  expect(await g.program({ store: 'steam', appId: '228980' }, true)).toBeNull()

  expect(await g.program({ store: 'epic', id: 'Peony' }, true)).toEqual(LUMEN_RUSH)
  expect(await g.program({ store: 'epic', id: 'peony' }, true)).toEqual(LUMEN_RUSH)
  expect(await g.program({ store: 'epic', id: 'ns1:c0ffee:Peony' }, true)).toEqual(LUMEN_RUSH)
  expect(await g.program({ store: 'epic', id: 'ns9:c0ffee:Other' }, true)).toEqual(LUMEN_RUSH) // by the catalog item
  expect(await g.program({ store: 'epic', id: 'Tulip' }, true)).toBeNull() // incomplete
  expect(await g.program({ store: 'epic', id: 'Rose' }, true)).toBeNull()

  // from the list: the same answers; the Steam folder of the registry is read once per run
  expect(await g.program({ store: 'steam', appId: '4242' }, false)).toEqual(STARFALL)
  expect(await g.program({ store: 'steam', appId: '5555' }, false)).toBeNull()
  expect(await g.program({ store: 'epic', id: 'ns1:c0ffee:Peony' }, false)).toEqual(LUMEN_RUSH)
  expect(powershell).toHaveBeenCalledTimes(1)
})

test('Games.readUrl tells a game program, an unresolved game and no game apart', async () => {
  const files: Record<string, FakeFile> = {
    ...STEAM_FILES,
    ...EPIC_FILES,
    'C:\\Users\\Ann\\Desktop\\Звёздная тактика.url': '[InternetShortcut]\r\nURL=steam://rungameid/4242\r\n',
    'C:\\Users\\Ann\\Desktop\\Lumen Rush.url': '[InternetShortcut]\r\nURL=com.epicgames.launcher://apps/Peony?action=launch&silent=true\r\n',
    'C:\\Users\\Ann\\Desktop\\Old Game.url': '[InternetShortcut]\r\nURL=steam://rungameid/5555\r\n',
    'C:\\Users\\Ann\\Desktop\\Site.url': '[InternetShortcut]\r\nURL=https://example.net/\r\n',
    'C:\\Users\\Ann\\Desktop\\Broken.url': 'garbage',
    'C:\\Users\\Ann\\Desktop\\Huge.url': 2_000_000
  }
  const { g } = games(files)
  expect(await g.readUrl('C:\\Users\\Ann\\Desktop\\Звёздная тактика.url', true)).toEqual(STARFALL)
  expect(await g.readUrl('C:\\Users\\Ann\\Desktop\\Lumen Rush.url', false)).toEqual(LUMEN_RUSH)
  expect(await g.readUrl('C:\\Users\\Ann\\Desktop\\Old Game.url', true)).toBe('game')
  expect(await g.readUrl('C:\\Users\\Ann\\Desktop\\Site.url', true)).toBeNull()
  expect(await g.readUrl('C:\\Users\\Ann\\Desktop\\Broken.url', true)).toBeNull()
  expect(await g.readUrl('C:\\Users\\Ann\\Desktop\\Huge.url', true)).toBeNull()
  expect(await g.readUrl('C:\\Users\\Ann\\Desktop\\Missing.url', true)).toBeNull()
})
