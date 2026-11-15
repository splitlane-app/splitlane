import { describe, expect, it } from 'vitest'
import type { HelperState } from './protocol'
import { DEFAULT_SETTINGS, parseSettings, rememberExitNode } from './settings'

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
      pendingUpdate: null,
      language: 'system',
      lastExitNode: null
    })
    expect(parseSettings('{"connectedOnce":true}').connectedOnce).toBe(true)
  })

  it('parseSettings reads language with a default', () => {
    expect(DEFAULT_SETTINGS.language).toBe('system')
    expect(parseSettings('{}').language).toBe('system')
    expect(parseSettings('{"language":"en"}').language).toBe('en')
    expect(parseSettings('{"language":"ru"}').language).toBe('ru')
    expect(parseSettings('{"language":"system"}').language).toBe('system')
    for (const bad of ['"de"', '"EN"', '1', 'null', 'true', '{}']) {
      expect(parseSettings(`{"language":${bad}}`).language).toBe('system')
    }
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

  it('parseSettings reads lastExitNode and drops a malformed one', () => {
    expect(DEFAULT_SETTINGS.lastExitNode).toBeNull()
    expect(parseSettings('{"lastExitNode":{"id":"100.64.0.2","name":"node-a"}}').lastExitNode).toEqual({
      id: '100.64.0.2',
      name: 'node-a'
    })
    for (const bad of ['null', '1', '"x"', '{}', '{"id":"1"}', '{"id":"1","name":""}', '{"id":1,"name":"a"}', '{"id":"","name":"a"}']) {
      expect(parseSettings(`{"lastExitNode":${bad}}`).lastExitNode, bad).toBeNull()
    }
  })

  it('rememberExitNode returns a new value only when the selected node has a name that differs', () => {
    const st = (selected: string | null, nodes: { id: string; name: string }[]): HelperState =>
      ({
        tailscale: { selectedExitNode: selected, exitNodes: nodes.map((n) => ({ ...n, online: true })) }
      }) as unknown as HelperState
    const a = { id: '100.64.0.2', name: 'node-a' }
    expect(rememberExitNode(null, st('100.64.0.2', [a]))).toEqual(a)
    expect(rememberExitNode(a, st('100.64.0.2', [a]))).toBeNull()
    expect(rememberExitNode(a, st('100.64.0.2', [{ id: a.id, name: 'renamed' }]))).toEqual({ id: a.id, name: 'renamed' })
    // nothing to learn: no selection, an empty list (routing off), a node without a name
    expect(rememberExitNode(a, st(null, [a]))).toBeNull()
    expect(rememberExitNode(a, st('100.64.0.2', []))).toBeNull()
    expect(rememberExitNode(null, st('100.64.0.2', [{ id: a.id, name: '' }]))).toBeNull()
  })
})
