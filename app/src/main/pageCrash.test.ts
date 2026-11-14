import { expect, test } from 'vitest'
import { PageCrashReload, RELOAD_QUIET_MS } from './pageCrash'

test('a crashed page is reloaded once, not in a loop', () => {
  let now = 1_000
  const guard = new PageCrashReload(() => now)
  // the window closing ends its page cleanly: nothing to do
  expect(guard.onGone('clean-exit')).toBe(false)
  expect(guard.onGone('crashed')).toBe(true)
  // the reloaded page dies again at once: it stays as it is
  now += 5_000
  expect(guard.onGone('crashed')).toBe(false)
  expect(guard.onGone('oom')).toBe(false)
  // a crash long after the reload is a new one
  now += RELOAD_QUIET_MS
  expect(guard.onGone('killed')).toBe(true)
  expect(guard.onGone('crashed')).toBe(false)
})
