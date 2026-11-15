import { mkdtemp, readdir, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { DEFAULT_SETTINGS } from '../shared/settings'
import { SettingsFile } from './settingsFile'

describe('SettingsFile', () => {
  it('saves atomically and loads back', async () => {
    const dir = await mkdtemp(join(tmpdir(), 'splitlane-'))
    try {
      const path = join(dir, 'nested', 'settings.json')
      const f = new SettingsFile(path)
      expect(await f.load()).toEqual(DEFAULT_SETTINGS)
      const s = { openAtLogin: true, enableOnStart: true, connectedOnce: false, checkUpdates: false, pendingUpdate: '1.2.3', language: 'en' as const }
      await f.save(s)
      expect(await f.load()).toEqual(s)
      expect(await readdir(join(dir, 'nested'))).toEqual(['settings.json'])
      expect(JSON.parse(await readFile(path, 'utf8'))).toEqual(s)
      await writeFile(path, 'garbage')
      expect(await f.load()).toEqual(DEFAULT_SETTINGS)
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })
})
