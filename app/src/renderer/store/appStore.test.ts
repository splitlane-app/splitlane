import { describe, expect, it } from 'vitest'
import { LINK_CLOSED, type Command, type CommandResult, type Snapshot } from '../../shared/ipc'
import type { Activity, ActivityApp, AppRule, HelperState, Lane } from '../../shared/protocol'
import { DEFAULT_SETTINGS } from '../../shared/settings'
import type { LinkStatus } from '../../shared/status'
import { t } from '../i18n/t'
import { boardView, type CardVM } from '../model/board'
import { addPrograms, moveTo, pin, type Layout } from '../model/layout'
import { createAppStore, effectiveLayout } from './appStore'

const GAME = 'C:\\Games\\Game\\game.exe'
const MAIL = 'C:\\Apps\\Mail\\mail.exe'
const BROWSER = 'C:\\Apps\\Browser\\browser.exe'
const EDITOR_DIR = 'D:\\Tools\\Editor\\'
const EDITOR = `${EDITOR_DIR}editor.exe`
const TOOL = 'D:\\Tools\\Tool\\tool.exe'
const DESKTOP = 'C:\\Users\\Anna\\Desktop\\'
const NOTES = `${DESKTOP}notes.exe`

const ID1 = '6f1c2a4e-0b7d-4c3a-9e21-5d8f7a1b2c01'
const ID2 = '6f1c2a4e-0b7d-4c3a-9e21-5d8f7a1b2c02'
const ID3 = '6f1c2a4e-0b7d-4c3a-9e21-5d8f7a1b2c03'

const ready: LinkStatus = { kind: 'ready', hello: { protocol: 2, helper: '0.1.0', core: '1.14.4' } }

function state(over: Partial<HelperState> = {}): HelperState {
  return {
    phase: 'on',
    reason: null,
    allDirect: false,
    defaultLane: 'vpn',
    apps: [],
    addresses: { vpn: '198.51.100.7', direct: '203.0.113.9' },
    connector: 'tailscale',
    tailscale: {
      backend: 'running',
      authUrl: null,
      deviceName: 'pc',
      exitNodes: [{ id: '100.64.0.2', name: 'node-a', online: true }],
      selectedExitNode: '100.64.0.2',
      path: 'direct'
    },
    proxy: { servers: [], selected: null, subscriptions: [] },
    systemTailscale: { installed: false, exitNodeOn: false, restoreOnDisable: false },
    ...over
  }
}

function snap(over: Partial<HelperState> = {}, link: LinkStatus = ready): Snapshot {
  return { link, state: state(over), settings: DEFAULT_SETTINGS, language: 'ru', install: { kind: 'idle' }, update: { kind: 'idle' } }
}

function rule(id: string, name: string, exe: string, lane: Lane): AppRule {
  const dir = exe.slice(0, exe.lastIndexOf('\\') + 1)
  return { id, name, exe, matcher: { kind: 'folder', value: dir }, lane }
}

function entry(key: string, over: Partial<ActivityApp> = {}): ActivityApp {
  return { key, kind: 'app', name: '', exe: '', ruleId: null, lane: 'vpn', down: 0, up: 0, connections: 0, ...over }
}

const tiles = (): ActivityApp[] => [
  entry('system', { kind: 'system', down: 3 }),
  entry('unattributed', { kind: 'unattributed' })
]

function activity(apps: ActivityApp[]): Activity {
  return { lanes: { vpn: { down: 0, up: 0 }, direct: { down: 0, up: 0 } }, apps }
}

/** A bridge whose lanes.set stays unanswered until the test answers it, as the helper does for up to 3 s. */
function fakeBridge() {
  const calls: { command: Command; answer: (r: CommandResult) => void }[] = []
  const command = (c: Command): Promise<CommandResult> =>
    new Promise((answer) => {
      calls.push({ command: c, answer })
    })
  const sent = (i: number): Layout => {
    const c = calls[i].command
    if (c.method !== 'lanes.set') throw new Error(`call ${i} is ${c.method}`)
    return { apps: c.apps, defaultLane: c.defaultLane }
  }
  return { calls, command, sent }
}

function ids(): () => string {
  const list = [ID1, ID2, ID3]
  let i = 0
  return () => list[i++]
}

const flush = (): Promise<void> => new Promise((resolve) => setTimeout(resolve, 0))

type Store = ReturnType<typeof createAppStore>

function layout(store: Store): Layout {
  const l = effectiveLayout(store.getState())
  if (l === null) throw new Error('no layout')
  return l
}

const lanes = (l: Layout): Lane[] => l.apps.map((r) => r.lane)

function board(store: Store): ReturnType<typeof boardView> {
  const s = store.getState()
  const l = layout(store)
  return boardView(l.apps, l.defaultLane, s.activity, s.snapshot?.state?.addresses ?? { vpn: null, direct: null }, s.search)
}

function cardOf(store: Store, key: string): CardVM {
  const b = board(store)
  const card = [...b.vpn.pinned, ...b.vpn.unpinned, ...b.direct.pinned, ...b.direct.unpinned].find((c) => c.key === key)
  if (card === undefined) throw new Error(`no card ${key}`)
  return card
}

function setup() {
  const bridge = fakeBridge()
  const store = createAppStore({ command: bridge.command, newId: ids() })
  return { bridge, store }
}

describe('appStore', () => {
  it('applies a change at once and sends lanes.set', async () => {
    const { bridge, store } = setup()
    const game = rule('r1', 'Игра', GAME, 'vpn')
    const mail = rule('r2', 'Почта', MAIL, 'vpn')
    store.getState().setSnapshot(snap({ apps: [game, mail] }))

    const card = cardOf(store, 'r1')
    const done = store.getState().change((l, id) => moveTo(l, card, 'direct', id))

    expect(layout(store).apps).toEqual([{ ...game, lane: 'direct' }, mail])
    expect(board(store).direct.pinned.map((c) => c.key)).toEqual(['r1'])
    expect(bridge.calls.map((c) => c.command)).toEqual([
      { method: 'lanes.set', apps: [{ ...game, lane: 'direct' }, mail], defaultLane: 'vpn' }
    ])

    // the helper saves the layout, publishes its state, then answers
    store.getState().setSnapshot(snap({ apps: [{ ...game, lane: 'direct' }, mail] }))
    bridge.calls[0].answer({ ok: true })
    await done
    expect(store.getState().pending).toEqual([])
    expect(layout(store).apps).toEqual([{ ...game, lane: 'direct' }, mail])
    expect(store.getState().notice).toBeNull()

    // a change that changes nothing is not sent
    const moved = cardOf(store, 'r1')
    await store.getState().change((l, id) => moveTo(l, moved, 'direct', id))
    expect(bridge.calls).toHaveLength(1)
  })

  it('queues rapid moves in order', async () => {
    const { bridge, store } = setup()
    const a = rule('ra', 'Альфа', GAME, 'vpn')
    const b = rule('rb', 'Бета', MAIL, 'vpn')
    const c = rule('rc', 'Гамма', BROWSER, 'vpn')
    store.getState().setSnapshot(snap({ apps: [a, b, c] }))

    const cards = ['ra', 'rb', 'rc'].map((k) => cardOf(store, k))
    const done = cards.map((card) => store.getState().change((l, id) => moveTo(l, card, 'direct', id)))

    // all three on screen at once, one lanes.set in flight
    expect(lanes(layout(store))).toEqual(['direct', 'direct', 'direct'])
    expect(store.getState().pending).toHaveLength(3)
    expect(bridge.calls).toHaveLength(1)
    expect(lanes(bridge.sent(0))).toEqual(['direct', 'vpn', 'vpn'])

    store.getState().setSnapshot(snap({ apps: [{ ...a, lane: 'direct' }, b, c] }))
    bridge.calls[0].answer({ ok: true })
    await flush()
    expect(bridge.calls).toHaveLength(2)
    expect(lanes(bridge.sent(1))).toEqual(['direct', 'direct', 'vpn'])
    expect(lanes(layout(store))).toEqual(['direct', 'direct', 'direct'])

    store.getState().setSnapshot(snap({ apps: [{ ...a, lane: 'direct' }, { ...b, lane: 'direct' }, c] }))
    bridge.calls[1].answer({ ok: true })
    await flush()
    expect(bridge.calls).toHaveLength(3)
    expect(lanes(bridge.sent(2))).toEqual(['direct', 'direct', 'direct'])

    const all = [a, b, c].map((r) => ({ ...r, lane: 'direct' as const }))
    store.getState().setSnapshot(snap({ apps: all }))
    bridge.calls[2].answer({ ok: true })
    await Promise.all(done)
    expect(bridge.calls).toHaveLength(3)
    expect(store.getState().pending).toEqual([])
    expect(layout(store).apps).toEqual(all)
  })

  it('a rejected move rolls back itself and the moves queued after it', async () => {
    const { bridge, store } = setup()
    const a = rule('ra', 'Альфа', GAME, 'vpn')
    const b = rule('rb', 'Бета', MAIL, 'vpn')
    const c = rule('rc', 'Гамма', BROWSER, 'vpn')
    store.getState().setSnapshot(snap({ apps: [a, b, c] }))

    const cards = ['ra', 'rb', 'rc'].map((k) => cardOf(store, k))
    const done = cards.map((card) => store.getState().change((l, id) => moveTo(l, card, 'direct', id)))

    store.getState().setSnapshot(snap({ apps: [{ ...a, lane: 'direct' }, b, c] }))
    bridge.calls[0].answer({ ok: true })
    await flush()
    expect(lanes(layout(store))).toEqual(['direct', 'direct', 'direct'])

    bridge.calls[1].answer({ ok: false, error: { code: 'apply_failed', message: 'connections left on the old lane' } })
    await Promise.all(done)

    expect(bridge.calls).toHaveLength(2) // the third move is never sent
    expect(store.getState().pending).toEqual([])
    expect(lanes(layout(store))).toEqual(['direct', 'vpn', 'vpn'])
    expect(store.getState().notice).toBe(t('error.apply_failed'))

    store.getState().dismissNotice()
    expect(store.getState().notice).toBeNull()
  })

  it('rule_rejected names the programs in the notice', async () => {
    const { bridge, store } = setup()
    const game = rule('r1', 'Игра', GAME, 'vpn')
    store.getState().setSnapshot(snap({ apps: [game] }))
    store.getState().setActivity(activity([entry(EDITOR_DIR, { name: 'Редактор', exe: EDITOR, down: 10 }), ...tiles()]))

    const gameCard = cardOf(store, 'r1')
    const editorCard = cardOf(store, EDITOR_DIR)
    const systemCard = cardOf(store, 'system')
    const done = store.getState().change((l, id) => {
      const moved = moveTo(moveTo(moveTo(l, gameCard, 'direct', id), editorCard, 'direct', id), systemCard, 'direct', id)
      return addPrograms(moved, [{ exe: TOOL, name: '' }], 'direct', id)
    })
    expect(bridge.sent(0).apps.map((r) => r.id)).toEqual(['r1', ID1, ID2, ID3])

    bridge.calls[0].answer({
      ok: false,
      error: {
        code: 'rule_rejected',
        message: 'rules rejected',
        rules: [
          { id: 'r1', reason: 'duplicate' },
          { id: ID1, reason: 'too_broad' },
          { id: ID2, reason: 'protected' },
          { id: ID3, reason: 'too_broad' }
        ]
      }
    })
    await done

    expect(store.getState().notice).toBe(
      [
        `Игра: ${t('reject.duplicate')}`,
        `Редактор: ${t('reject.too_broad')}`,
        `Службы Windows: ${t('reject.protected')}`,
        `tool: ${t('reject.too_broad')}`
      ].join('; ')
    )
    expect(layout(store).apps).toEqual([game])
  })

  it("takes the helper's normalized rule by id", async () => {
    const { bridge, store } = setup()
    store.getState().setSnapshot(snap({ apps: [] }))
    store.getState().setActivity(activity([entry(DESKTOP, { name: 'Заметки', exe: NOTES, down: 300 }), ...tiles()]))

    const card = cardOf(store, DESKTOP)
    const done = store.getState().change((l, id) => moveTo(l, card, 'direct', id))
    const asSent: AppRule = { id: ID1, name: '', exe: NOTES, matcher: { kind: 'folder', value: '' }, lane: 'direct' }
    expect(bridge.sent(0).apps).toEqual([asSent])
    // on screen at once, once: pinned with the program's name, gone from the unpinned ones
    expect(board(store).direct.pinned.map((c) => [c.key, c.name])).toEqual([[ID1, 'Заметки']])
    expect(board(store).vpn.unpinned.map((c) => c.key)).toEqual(['system', 'unattributed'])

    // the helper makes it a file rule (the desktop is a shared folder), names it and publishes its state
    // before it answers
    const normalized: AppRule = {
      id: ID1,
      name: 'Заметки',
      exe: NOTES,
      matcher: { kind: 'file', value: NOTES },
      lane: 'direct'
    }
    store.getState().setSnapshot(snap({ apps: [normalized] }))
    expect(layout(store).apps).toEqual([normalized])
    expect(board(store).direct.pinned.map((c) => [c.key, c.name])).toEqual([[ID1, 'Заметки']])
    expect(board(store).vpn.unpinned.map((c) => c.key)).toEqual(['system', 'unattributed'])

    bridge.calls[0].answer({ ok: true })
    await done
    expect(store.getState().pending).toEqual([])
    expect(layout(store).apps).toEqual([normalized])

    // the next change sends the helper's rule as it is, but for the lane
    const pinned = cardOf(store, ID1)
    void store.getState().change((l, id) => moveTo(l, pinned, 'vpn', id))
    expect(bridge.sent(1).apps).toEqual([{ ...normalized, lane: 'vpn' }])
  })

  it('rolls back a pending layout when the link drops', async () => {
    const { bridge, store } = setup()
    const game = rule('r1', 'Игра', GAME, 'vpn')
    store.getState().setSnapshot(snap({ apps: [game] }))
    store.getState().setActivity(activity([entry(EDITOR_DIR, { name: 'Редактор', exe: EDITOR }), ...tiles()]))

    const gameCard = cardOf(store, 'r1')
    const editorCard = cardOf(store, EDITOR_DIR)
    const first = store.getState().change((l, id) => moveTo(l, gameCard, 'direct', id))
    const second = store.getState().change((l, id) => pin(l, editorCard, id))
    const editorRule: AppRule = { id: ID1, name: '', exe: EDITOR, matcher: { kind: 'folder', value: '' }, lane: 'vpn' }
    expect(layout(store).apps).toEqual([{ ...game, lane: 'direct' }, editorRule])

    // the helper restarts: the pipe closes before it answers; the main process keeps the last state
    store.getState().setSnapshot(snap({ apps: [game] }, { kind: 'restarting', attempt: 1 }))
    bridge.calls[0].answer({ ok: false, error: LINK_CLOSED })
    await Promise.all([first, second])

    expect(bridge.calls).toHaveLength(1)
    expect(store.getState().pending).toEqual([])
    expect(layout(store).apps).toEqual([game])
    expect(store.getState().notice).toBe(t('error.internal'))

    // after the restart state.get has the first move: the helper had saved it before the pipe closed
    store.getState().setSnapshot(snap({ apps: [{ ...game, lane: 'direct' }] }))
    expect(layout(store).apps).toEqual([{ ...game, lane: 'direct' }])
  })

  it('keeps 30 history points per key', () => {
    const { store } = setup()
    store.getState().setSnapshot(snap())
    for (let i = 1; i <= 35; i++) {
      const apps = [entry('k1', { down: i, up: 1 })]
      if (i === 1) apps.push(entry('k2', { down: 5 }))
      store.getState().setActivity(activity(apps))
      if (i === 10) expect(store.getState().history.k2).toEqual([5, 0, 0, 0, 0, 0, 0, 0, 0, 0])
    }
    const k1 = store.getState().history.k1
    expect(k1).toHaveLength(30)
    expect(k1[0]).toBe(7)
    expect(k1[29]).toBe(36)
    // a key gone from the activity decays to zeros and is dropped after 30 of them
    expect(store.getState().history).not.toHaveProperty('k2')
  })

  it('drops the activity outside the live phases', () => {
    const { store } = setup()
    store.getState().setSnapshot(snap())
    store.getState().setActivity(activity([entry('k1', { down: 9 })]))
    store.getState().setSnapshot(snap({ phase: 'degraded' }))
    expect(store.getState().activity).not.toBeNull()
    store.getState().setSnapshot(snap({ phase: 'off' }))
    expect(store.getState().activity).toBeNull()
    expect(store.getState().history).toEqual({})
    // a late event sent before the phase changed is not kept
    store.getState().setActivity(activity([entry('k1', { down: 9 })]))
    expect(store.getState().activity).toBeNull()
    expect(store.getState().history).toEqual({})
  })

  it('keeps the screen state of the page', () => {
    const { store } = setup()
    const s = store.getState
    expect(s().ui).toEqual({ addOpen: false, cardKey: null, screen: 'board' })
    s().openAdd(true)
    s().openCard('r1')
    s().showConnection(true)
    s().setSearch('игра')
    expect(s().ui).toEqual({ addOpen: true, cardKey: 'r1', screen: 'connection' })
    expect(s().search).toBe('игра')
    s().showConnection(false)
    s().openCard(null)
    s().openAdd(false)
    expect(s().ui).toEqual({ addOpen: false, cardKey: null, screen: 'board' })
    s().showNotice('Не программа: readme.txt')
    expect(s().notice).toBe('Не программа: readme.txt')
    expect(effectiveLayout(s())).toBeNull() // no state yet
  })
})
