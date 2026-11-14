import type { Program } from '../../shared/programs'
import { LIMITS, packageValue, utf8Bytes, type AppRule, type HelperState, type Lane } from '../../shared/protocol'
import type { CardVM } from './board'

/** What lanes.set sends: every pinned rule and the default lane. */
export interface Layout {
  apps: AppRule[]
  defaultLane: Lane
}

// The operations below never change their input; one that changes nothing returns its input itself.
// An operation that pins a program takes the id of the new rule from newId, called once per new rule in
// a fixed order, so that the caller can hand out the same ids every time the operation runs again on a
// newer layout. A layout that has a rule of that id already (the helper took it, maybe normalized) gets
// only the lane changed on that rule: the helper's version of every other field stays.

export function layoutOf(state: HelperState): Layout {
  return { apps: state.apps, defaultLane: state.defaultLane }
}

/** The longest start of s that fits maxBytes of UTF-8, cut between characters. */
function clip(s: string, maxBytes: number): string {
  if (utf8Bytes(s) <= maxBytes) return s
  let out = ''
  let bytes = 0
  for (const ch of s) {
    const n = utf8Bytes(ch)
    if (bytes + n > maxBytes) break
    out += ch
    bytes += n
  }
  return out
}

/** A new rule of a program (spec 8.1): the helper fills value, and name when the program has none. */
export function newRule(p: Program, lane: Lane, id: string): AppRule {
  return {
    id,
    name: clip(p.name, LIMITS.maxNameBytes),
    exe: p.exe,
    matcher: { kind: packageValue(p.exe) === null ? 'folder' : 'package', value: '' },
    lane
  }
}

/** The rule of the "Windows services" tile. */
function systemRule(lane: Lane, id: string): AppRule {
  return { id, name: '', exe: '', matcher: { kind: 'system', value: '' }, lane }
}

function setLane(l: Layout, ruleId: string, lane: Lane): Layout {
  const i = l.apps.findIndex((r) => r.id === ruleId)
  if (i < 0 || l.apps[i].lane === lane) return l
  const apps = l.apps.slice()
  apps[i] = { ...apps[i], lane }
  return { ...l, apps }
}

const sameExe = (exe: string) => (r: AppRule) =>
  r.matcher.kind !== 'system' && r.exe.toLowerCase() === exe.toLowerCase()
const isSystem = (r: AppRule): boolean => r.matcher.kind === 'system'

/**
 * Pins a program as rule: the rule of the same id when the layout has it, else the program's own rule
 * (found by same), else rule added at the end — unless that goes over the helper's limits.
 */
function place(l: Layout, rule: AppRule, same: (r: AppRule) => boolean): Layout {
  if (l.apps.some((r) => r.id === rule.id)) return setLane(l, rule.id, rule.lane)
  const own = l.apps.find(same)
  if (own !== undefined) return setLane(l, own.id, rule.lane)
  if (l.apps.length >= LIMITS.maxApps || utf8Bytes(rule.exe) > LIMITS.maxPathBytes) return l
  return { ...l, apps: [...l.apps, rule] }
}

/** The card goes to lane: a pinned one changes its lane, an unpinned one is pinned there (spec 9.2). */
export function moveTo(l: Layout, card: CardVM, lane: Lane, newId: () => string): Layout {
  if (card.kind === 'unattributed') return l
  if (card.ruleId !== null) return setLane(l, card.ruleId, lane)
  const id = newId()
  if (card.kind === 'system') return place(l, systemRule(lane, id), isSystem)
  if (card.exe === '') return l
  // not a shortcut: the name is left to the helper
  return place(l, newRule({ exe: card.exe, name: '' }, lane, id), sameExe(card.exe))
}

/** An unpinned card is pinned in the default lane; a pinned one stays as it is. */
export function pin(l: Layout, card: CardVM, newId: () => string): Layout {
  return card.ruleId !== null ? l : moveTo(l, card, l.defaultLane, newId)
}

export function unpin(l: Layout, ruleId: string): Layout {
  if (!l.apps.some((r) => r.id === ruleId)) return l
  return { ...l, apps: l.apps.filter((r) => r.id !== ruleId) }
}

/** Another default lane: the pinned rules stay, the unpinned programs follow the default lane. */
export function makeDefault(l: Layout, lane: Lane): Layout {
  return l.defaultLane === lane ? l : { ...l, defaultLane: lane }
}

/** "Вся папка программы" (spec 9.4): folder or file, with an empty value for the helper to fill. */
export function setWholeFolder(l: Layout, ruleId: string, whole: boolean): Layout {
  const i = l.apps.findIndex((r) => r.id === ruleId)
  if (i < 0) return l
  const { kind } = l.apps[i].matcher
  const want = whole ? 'folder' : 'file'
  if ((kind !== 'folder' && kind !== 'file') || kind === want) return l
  const apps = l.apps.slice()
  apps[i] = { ...apps[i], matcher: { kind: want, value: '' } }
  return { ...l, apps }
}

/** Programs from the add panel or Explorer pinned in lane; one pinned already (same exe) changes its lane. */
export function addPrograms(l: Layout, programs: Program[], lane: Lane, newId: () => string): Layout {
  let out = l
  for (const p of programs) out = place(out, newRule(p, lane, newId()), sameExe(p.exe))
  return out
}
