import { describe, expect, it } from 'vitest'
import type { HelperState, Phase, ReasonCode } from '../shared/protocol'
import { AutoEnable } from './autoEnable'

function st(phase: Phase, code?: ReasonCode): HelperState {
  return { phase, reason: code ? { code, params: {} } : null } as HelperState
}

function setup() {
  let time = 0
  let calls = 0
  let timer: (() => void) | null = null
  let timers = 0
  const a = new AutoEnable({
    enable: async () => {
      calls++
    },
    setTimer: (fn, ms) => {
      expect(ms).toBe(10000)
      timers++
      timer = fn
      return () => {
        timer = null
      }
    },
    now: () => time
  })
  return {
    a,
    calls: () => calls,
    timers: () => timers,
    fire: () => {
      const f = timer
      timer = null
      f?.()
    },
    pending: () => timer !== null,
    at: (t: number) => {
      time = t
    }
  }
}

describe('AutoEnable', () => {
  it('enables once when the setting is on and the phase is off', () => {
    const s = setup()
    s.a.onReady(st('off'), true)
    expect(s.calls()).toBe(1)
    s.a.onReady(st('off'), true)
    expect(s.calls()).toBe(1)
  })

  it('does not enable when the setting is off or the phase is not off', () => {
    const s = setup()
    s.a.onReady(st('off'), false)
    expect(s.calls()).toBe(0)
    const t = setup()
    t.a.onReady(st('on'), true)
    t.a.onReady(st('off'), true)
    expect(t.calls()).toBe(0)
  })

  // The owner's PC, 2026-11-17: the window started at logon while the helper's start-up was still
  // giving the system exit node back (phase stopping, spec 5.7), and the setting did nothing.
  it('waits for the stopping of the helper start-up to end in off, then enables once', () => {
    const s = setup()
    s.a.onReady(st('stopping'), true)
    expect(s.calls()).toBe(0)
    s.a.onState(st('stopping', 'restore_pending'))
    expect(s.calls()).toBe(0)
    expect(s.a.retrying()).toBe(false)
    s.at(60_000)
    s.a.onState(st('off'))
    expect(s.calls()).toBe(1)
    s.a.onState(st('starting'))
    s.a.onState(st('on'))
    s.a.onState(st('off'))
    expect(s.calls()).toBe(1)
  })

  it('gives the wait up when stopping ends in another phase, lasts 2 minutes, or on stop', () => {
    const failed = setup()
    failed.a.onReady(st('stopping'), true)
    failed.a.onState(st('error', 'adapter_left'))
    failed.a.onState(st('off'))
    expect(failed.calls()).toBe(0)

    const late = setup()
    late.a.onReady(st('stopping'), true)
    late.at(120_000)
    late.a.onState(st('off'))
    expect(late.calls()).toBe(0)

    const stopped = setup()
    stopped.a.onReady(st('stopping'), true)
    stopped.a.stop()
    stopped.a.onState(st('off'))
    expect(stopped.calls()).toBe(0)
  })

  it('retries no_default_route after the wait as after a plain start', () => {
    const s = setup()
    s.a.onReady(st('stopping'), true)
    s.at(30_000)
    s.a.onState(st('off'))
    s.a.onState(st('error', 'no_default_route'))
    expect(s.pending()).toBe(true)
    s.at(40_000)
    s.fire()
    expect(s.calls()).toBe(2)
  })

  it('retries no_default_route every 10 s for 2 minutes', () => {
    const s = setup()
    s.a.onReady(st('off'), true)
    s.a.onState(st('error', 'no_default_route'))
    expect(s.pending()).toBe(true)
    s.a.onState(st('error', 'no_default_route'))
    expect(s.timers()).toBe(1)
    s.at(10000)
    s.fire()
    expect(s.calls()).toBe(2)
    s.a.onState(st('error', 'no_default_route'))
    expect(s.timers()).toBe(2)
    s.at(119000)
    s.fire()
    expect(s.calls()).toBe(3)
    s.at(120000)
    s.a.onState(st('error', 'no_default_route'))
    expect(s.timers()).toBe(2)
    expect(s.pending()).toBe(false)
  })

  it('stop cancels the retries', () => {
    const s = setup()
    s.a.onReady(st('off'), true)
    s.a.onState(st('error', 'no_default_route'))
    s.a.stop()
    expect(s.pending()).toBe(false)
    s.a.onState(st('error', 'no_default_route'))
    expect(s.timers()).toBe(1)
  })

  it('retrying() is true from the start-up attempt until success, a stop or the give-up', () => {
    const s = setup()
    expect(s.a.retrying()).toBe(false)
    s.a.onReady(st('off'), false)
    expect(s.a.retrying()).toBe(false)

    const t = setup()
    t.a.onReady(st('off'), true)
    expect(t.a.retrying()).toBe(true)
    t.a.onState(st('error', 'no_default_route'))
    expect(t.a.retrying()).toBe(true)
    t.fire()
    t.at(125_000)
    t.a.onState(st('error', 'no_default_route'))
    // gave up: the window of 2 minutes is over and no retry is waiting
    expect(t.a.retrying()).toBe(false)

    const u = setup()
    u.a.onReady(st('off'), true)
    u.a.onState(st('on'))
    expect(u.a.retrying()).toBe(false)
  })
})
