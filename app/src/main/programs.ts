import type { Dropped, Program } from '../shared/programs'

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
}

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

export async function installedPrograms(fs: ProgramsFs, dirs: string[]): Promise<Program[]> {
  const seen = new Set<string>()
  const out: Program[] = []
  for (const dir of dirs) {
    let files: string[]
    try {
      files = await fs.listFiles(dir)
    } catch {
      continue
    }
    for (const file of files) {
      if (!file.toLowerCase().endsWith('.lnk')) continue
      const link = safeRead((p) => fs.readShortcut(p), file)
      if (link === null || !isExePath(link.target)) continue
      const key = link.target.toLowerCase()
      if (seen.has(key)) continue
      seen.add(key)
      out.push({ exe: link.target, name: baseName(file).slice(0, -'.lnk'.length) })
    }
  }
  return out.sort((a, b) => a.name.localeCompare(b.name, 'ru'))
}

/** The file picked in the dialog as one dropped from Explorer: its program, or its name when it is none. */
export function pickedFile(path: string, readShortcut: ProgramsFs['readShortcut']): Dropped {
  return resolveDropped([path], readShortcut)[0] ?? { kind: 'skipped', name: baseName(path) }
}

export function resolveDropped(paths: string[], readShortcut: ProgramsFs['readShortcut']): Dropped[] {
  const seen = new Set<string>()
  const out: Dropped[] = []
  for (const path of paths) {
    const name = baseName(path)
    let exe: string | null = null
    let programName = ''
    if (DRIVE_PATH.test(path)) {
      const lower = path.toLowerCase()
      if (lower.endsWith('.exe')) {
        exe = path
      } else if (lower.endsWith('.lnk')) {
        const link = safeRead(readShortcut, path)
        if (link !== null && isExePath(link.target)) {
          exe = link.target
          programName = name.slice(0, -'.lnk'.length)
        }
      }
    }
    if (exe === null) {
      out.push({ kind: 'skipped', name })
      continue
    }
    const key = exe.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    out.push({ kind: 'program', program: { exe, name: programName } })
  }
  return out
}
