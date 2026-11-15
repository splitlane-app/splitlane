import { expect, test } from 'vitest'
import { FORBIDDEN_SWITCHES, forbiddenSwitch } from './switches'

const has =
  (...names: string[]) =>
  (name: string): boolean =>
    names.includes(name)

test('every switch that opens the window to a debugger or to code of its own is forbidden', () => {
  expect([...FORBIDDEN_SWITCHES]).toEqual([
    'remote-debugging-port',
    'remote-debugging-pipe',
    'remote-allow-origins',
    'inspect',
    'inspect-brk',
    'inspect-port',
    'js-flags'
  ])
  for (const name of FORBIDDEN_SWITCHES) {
    expect(forbiddenSwitch(has(name))).toBe(name)
  }
  expect(forbiddenSwitch(has('hidden', 'inspect-brk', 'js-flags'))).toBe('inspect-brk')
})

test('a start without those switches goes on', () => {
  expect(forbiddenSwitch(has())).toBeNull()
  // the window's own switch and the ones Windows or electron-builder pass
  expect(forbiddenSwitch(has('hidden', 'updated', 'remote-debugging', 'inspector'))).toBeNull()
})
