import type { Lang } from '../shared/i18n'
import type { Dropped, Program, SkipReason } from '../shared/programs'

export function startMenuDirs(env: Record<string, string | undefined>): string[] {
  const tail = '\\Microsoft\\Windows\\Start Menu\\Programs'
  const dirs: string[] = []
  for (const key of ['ProgramData', 'AppData']) {
    const base = env[key]
    if (base !== undefined && base !== '') dirs.push(base + tail)
  }
  return dirs
}

export interface ProgramsFs {
  /** Lists files under a folder, recursively. */
  listFiles(dir: string): Promise<string[]>
  readShortcut(lnk: string): { target: string } | null
  /** A `.url` file: the program of its game, 'game' for a game that is not found, null for no game shortcut. */
  readUrl(path: string): Promise<Program | 'game' | null>
}

/** What a drop from Explorer (or the file dialog) reads of the shortcuts. */
export type DropReaders = Pick<ProgramsFs, 'readShortcut' | 'readUrl'>

const DRIVE_PATH = /^[A-Za-z]:\\/

function isExePath(p: string): boolean {
  return DRIVE_PATH.test(p) && p.toLowerCase().endsWith('.exe')
}

function baseName(p: string): string {
  const i = p.lastIndexOf('\\')
  return i < 0 ? p : p.slice(i + 1)
}

function safeRead(fs: Pick<ProgramsFs, 'readShortcut'>['readShortcut'], lnk: string): { target: string } | null {
  try {
    return fs(lnk)
  } catch {
    return null
  }
}

/** The game of a `.url` file as a program (its exe a drive path), 'game', or null; a failure is null. */
async function safeUrl(readUrl: ProgramsFs['readUrl'], path: string): Promise<Program | 'game' | null> {
  try {
    const r = await readUrl(path)
    return r === 'game' || r === null || isExePath(r.exe) ? r : null
  } catch {
    return null
  }
}

/** The programs of the Start-menu shortcuts: `.lnk` to an exe (named after the shortcut) and game `.url` files. */
export async function installedPrograms(fs: ProgramsFs, dirs: string[], lang: Lang): Promise<Program[]> {
  const seen = new Set<string>()
  const out: Program[] = []
  const add = (p: Program): void => {
    const key = p.exe.toLowerCase()
    if (seen.has(key)) return
    seen.add(key)
    out.push(p)
  }
  for (const dir of dirs) {
    let files: string[]
    try {
      files = await fs.listFiles(dir)
    } catch {
      continue
    }
    for (const file of files) {
      const lower = file.toLowerCase()
      if (lower.endsWith('.lnk')) {
        const link = safeRead((p) => fs.readShortcut(p), file)
        if (link !== null && isExePath(link.target)) add({ exe: link.target, name: baseName(file).slice(0, -'.lnk'.length) })
      } else if (lower.endsWith('.url')) {
        const game = await safeUrl((p) => fs.readUrl(p), file)
        if (game !== null && game !== 'game') add({ exe: game.exe, name: game.name })
      }
    }
  }
  return out.sort((a, b) => a.name.localeCompare(b.name, lang))
}

/** The file picked in the dialog as one dropped from Explorer: its program, or its name when it is none. */
export async function pickedFile(path: string, readers: DropReaders): Promise<Dropped> {
  return (await resolveDropped([path], readers))[0] ?? { kind: 'skipped', name: baseName(path), reason: 'notProgram' }
}

/**
 * Files dropped from Explorer: an .exe is the program, a `.lnk` its exe target (named after the shortcut), a game
 * `.url` the game's exe (named after the game). The rest is skipped by name: a game shortcut whose game is not found
 * as 'game' (without `.url`), anything else as 'notProgram'.
 */
export async function resolveDropped(paths: string[], readers: DropReaders): Promise<Dropped[]> {
  const seen = new Set<string>()
  const out: Dropped[] = []
  for (const path of paths) {
    const name = baseName(path)
    let program: Program | null = null
    let reason: SkipReason = 'notProgram'
    if (DRIVE_PATH.test(path)) {
      const lower = path.toLowerCase()
      if (lower.endsWith('.exe')) {
        program = { exe: path, name: '' }
      } else if (lower.endsWith('.lnk')) {
        const link = safeRead(readers.readShortcut, path)
        if (link !== null && isExePath(link.target)) program = { exe: link.target, name: name.slice(0, -'.lnk'.length) }
      } else if (lower.endsWith('.url')) {
        const game = await safeUrl(readers.readUrl, path)
        if (game === 'game') reason = 'game'
        else if (game !== null) program = { exe: game.exe, name: game.name }
      }
    }
    if (program === null) {
      out.push({ kind: 'skipped', name: reason === 'game' ? name.slice(0, -'.url'.length) : name, reason })
      continue
    }
    const key = program.exe.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    out.push({ kind: 'program', program })
  }
  return out
}
