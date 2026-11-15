import { describe, expect, it } from 'vitest'
import { DEFAULT_SETTINGS, parseSettings } from './settings'

describe('settings', () => {
  it('parseSettings fills defaults and ignores junk', () => {
    expect(parseSettings('{')).toEqual(DEFAULT_SETTINGS)
    expect(parseSettings('[1]')).toEqual(DEFAULT_SETTINGS)
    expect(parseSettings('null')).toEqual(DEFAULT_SETTINGS)
    expect(parseSettings('{"openAtLogin":true,"enableOnStart":"yes","x":1}')).toEqual({
      openAtLogin: true,
      enableOnStart: false,
      connectedOnce: false,
      checkUpdates: true,
      pendingUpdate: null
    })
    expect(parseSettings('{"connectedOnce":true}').connectedOnce).toBe(true)
  })

  it('parseSettings reads checkUpdates and pendingUpdate with defaults', () => {
    expect(DEFAULT_SETTINGS.checkUpdates).toBe(true)
    expect(DEFAULT_SETTINGS.pendingUpdate).toBeNull()
    expect(parseSettings('{}')).toMatchObject({ checkUpdates: true, pendingUpdate: null })
    expect(parseSettings('{"checkUpdates":false,"pendingUpdate":"1.2.3"}')).toMatchObject({
      checkUpdates: false,
      pendingUpdate: '1.2.3'
    })
    expect(parseSettings('{"checkUpdates":"no"}').checkUpdates).toBe(true)
    for (const bad of ['"v1.2.3"', '"1.2"', '"01.2.3"', '5', 'true', '"1.2.3 "']) {
      expect(parseSettings(`{"pendingUpdate":${bad}}`).pendingUpdate).toBeNull()
    }
  })
})
