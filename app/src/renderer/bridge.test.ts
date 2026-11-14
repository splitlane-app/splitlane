import { expect, test } from 'vitest'
import type { Snapshot, SplitLaneBridge } from '../shared/ipc'
import type { Activity } from '../shared/protocol'
import { connectBridge, IconCache } from './bridge'
import { createAppStore } from './store/appStore'
import { activity, entry, fakeBridge, snap } from './testing'

/** A bridge whose pushes and initial snapshot the test sends by hand. */
function pushBridge() {
  let pushSnapshot: (s: Snapshot) => void = () => {}
  let pushActivity: (a: Activity) => void = () => {}
  let answer: (s: Snapshot) => void = () => {}
  const { bridge } = fakeBridge({
    snapshot: () => new Promise<Snapshot>((resolve) => (answer = resolve)),
    onSnapshot: (cb) => {
      pushSnapshot = cb
      return () => (pushSnapshot = () => {})
    },
    onActivity: (cb) => {
      pushActivity = cb
      return () => (pushActivity = () => {})
    }
  })
  return {
    bridge: bridge as SplitLaneBridge,
    pushSnapshot: (s: Snapshot) => pushSnapshot(s),
    pushActivity: (a: Activity) => pushActivity(a),
    answer: (s: Snapshot) => answer(s)
  }
}

const flush = (): Promise<void> => new Promise((r) => setTimeout(r, 0))

test('connectBridge takes the initial snapshot and every push at once', async () => {
  const b = pushBridge()
  const store = createAppStore({ command: (c) => b.bridge.command(c) })
  connectBridge(b.bridge, store)

  const first = snap({ phase: 'off' })
  b.answer(first)
  await flush()
  expect(store.getState().snapshot).toBe(first)

  const pushed = snap({ phase: 'on' })
  b.pushSnapshot(pushed)
  // synchronously: the layout queue relies on the state arriving before the answer of lanes.set
  expect(store.getState().snapshot).toBe(pushed)

  const a = activity([entry('C:\\Games\\Game\\', { down: 5 })])
  b.pushActivity(a)
  expect(store.getState().activity).toBe(a)
})

test('connectBridge ignores an initial snapshot that comes after a push', async () => {
  const b = pushBridge()
  const store = createAppStore({ command: (c) => b.bridge.command(c) })
  connectBridge(b.bridge, store)

  const pushed = snap({ phase: 'on' })
  b.pushSnapshot(pushed)
  b.answer(snap({ phase: 'off' }))
  await flush()
  expect(store.getState().snapshot).toBe(pushed)
})

test('connectBridge drops the activity while the link is not ready', () => {
  const b = pushBridge()
  const store = createAppStore({ command: (c) => b.bridge.command(c) })
  const stop = connectBridge(b.bridge, store)

  b.pushSnapshot(snap({ phase: 'on' }))
  b.pushActivity(activity([entry('C:\\Games\\Game\\', { down: 5 })]))
  expect(store.getState().activity).not.toBeNull()

  b.pushSnapshot({ ...snap({ phase: 'on' }), link: { kind: 'restarting', attempt: 1 } })
  expect(store.getState().activity).toBeNull()
  expect(store.getState().history).toEqual({})
  b.pushActivity(activity([entry('C:\\Games\\Game\\', { down: 5 })]))
  expect(store.getState().activity).toBeNull()

  stop()
  b.pushSnapshot(snap({ phase: 'off' }))
  expect(store.getState().snapshot?.state?.phase).toBe('on')
})

test('IconCache asks for each program once and keeps only image data', async () => {
  const asked: string[] = []
  const icons = new IconCache(async (exe) => {
    asked.push(exe)
    return exe.endsWith('a.exe') ? 'data:image/png;base64,AAAA' : 'file:///C:/x.png'
  })
  let changes = 0
  icons.subscribe(() => changes++)
  icons.request('C:\\A\\a.exe')
  icons.request('c:\\a\\A.EXE')
  icons.request('C:\\B\\b.exe')
  icons.request('')
  await flush()
  expect(asked).toEqual(['C:\\A\\a.exe', 'C:\\B\\b.exe'])
  expect(icons.get('C:\\A\\a.exe')).toBe('data:image/png;base64,AAAA')
  expect(icons.get('C:\\B\\b.exe')).toBeNull()
  expect(changes).toBe(1)
})
