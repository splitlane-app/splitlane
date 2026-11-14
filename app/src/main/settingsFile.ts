import { mkdir, readFile, rename, writeFile } from 'node:fs/promises'
import { dirname } from 'node:path'
import { DEFAULT_SETTINGS, parseSettings, type WindowSettings } from '../shared/settings'

export class SettingsFile {
  constructor(private readonly path: string) {}

  async load(): Promise<WindowSettings> {
    try {
      return parseSettings(await readFile(this.path, 'utf8'))
    } catch {
      return { ...DEFAULT_SETTINGS }
    }
  }

  async save(s: WindowSettings): Promise<void> {
    await mkdir(dirname(this.path), { recursive: true })
    const tmp = `${this.path}.tmp`
    await writeFile(tmp, JSON.stringify(s, null, 2), 'utf8')
    await rename(tmp, this.path)
  }
}
