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
      connectedOnce: false
    })
    expect(parseSettings('{"connectedOnce":true}').connectedOnce).toBe(true)
  })
})
