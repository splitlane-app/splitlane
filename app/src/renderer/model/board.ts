import type { Activity, ActivityApp, AppRule, HelperState, Lane } from '../../shared/protocol'
import { currentLang, t } from '../i18n/t'

/** One card of the board: a pinned rule, an unpinned program of the activity or a special tile. */
export interface CardVM {
  /** The activity key: the rule id when pinned, the program folder or file, or "system"/"unattributed". */
  key: string
  ruleId: string | null
  kind: 'app' | 'system' | 'unattributed'
  name: string
  /** The file the icon is taken from; '' for the special tiles. */
  exe: string
  lane: Lane
  pinned: boolean
  down: number
  up: number
  connections: number
  movable: boolean
  /** Routed by a rule of the helper's own (spec 5.6): shown in that lane, never moved or pinned. */
  builtin: boolean
}

export interface LaneVM {
  lane: Lane
  isDefault: boolean
  /** In the order of the rules. */
  pinned: CardVM[]
  /** The programs that a built-in rule of the helper sends this way, by name; after the pinned ones. */
  builtin: CardVM[]
  /** Below the line, only in the default lane: by name, the special tiles last. */
  unpinned: CardVM[]
  down: number
  up: number
  address: string | null
}

/** The file name of a path: what tells two programs of the same description apart. */
export function fileName(path: string): string {
  return path.slice(Math.max(path.lastIndexOf('\\'), path.lastIndexOf('/')) + 1)
}

/** The file name without its extension: what the helper names a program after when it has no description. */
export function baseName(path: string): string {
  const file = fileName(path)
  const dot = file.lastIndexOf('.')
  return dot > 0 ? file.slice(0, dot) : file
}

const lower = (s: string): string => s.toLowerCase()

/**
 * The programs the helper never lets be pinned (`protected`), by file name, with the lane of their built-in rule:
 * the system Tailscale goes direct (spec 5.6), SplitLane's helper by the VPN (5.3); the core's own lane is not
 * known here.
 */
const PROTECTED: ReadonlyMap<string, Lane | null> = new Map<string, Lane | null>([
  ['tailscaled.exe', 'direct'],
  ['tailscale-ipn.exe', 'direct'],
  ['splitlane-helper.exe', 'vpn'],
  ['sing-box.exe', null]
])

/** The lane of the built-in rule of a protected program (null: not known); undefined for any other program. */
export function protectedLane(exe: string): Lane | null | undefined {
  return PROTECTED.get(lower(fileName(exe)))
}

/** Whether a program's name or path (its file name too) has the search, ignoring case; an empty search keeps all. */
export function matchesSearch(name: string, exe: string, search: string): boolean {
  const q = lower(search.trim())
  return q === '' || lower(name).includes(q) || lower(exe).includes(q)
}

/**
 * A stable order of programs: by name, then by file, then by key (the activity's own order follows the traffic); the
 * names compare by the rules of the current language.
 */
function byName(a: CardVM, b: CardVM): number {
  const lang = currentLang()
  return (
    a.name.localeCompare(b.name, lang) ||
    fileName(a.exe).localeCompare(fileName(b.exe), lang) ||
    (a.key < b.key ? -1 : a.key > b.key ? 1 : 0)
  )
}

/** The name a pinned rule shows: the dictionary name for system, else the rule's, its program's, its file's. */
function nameOf(rule: AppRule, entry: ActivityApp | undefined): string {
  if (rule.matcher.kind === 'system') return t('tile.system')
  return rule.name || entry?.name || baseName(rule.exe)
}

/** The program of a rule the activity does not know by the rule yet: an unpinned entry with the rule's file. */
function sameProgram(rule: AppRule, e: ActivityApp): boolean {
  if (e.kind !== 'app') return false
  return (rule.exe !== '' && lower(e.exe) === lower(rule.exe)) || (rule.matcher.value !== '' && lower(e.key) === lower(rule.matcher.value))
}

/** The name of a rule as its card shows it (for notices about rules that are not on the board any more). */
export function ruleName(rule: AppRule, activity: Activity | null): string {
  const entries = activity?.apps ?? []
  const entry = entries.find((e) => e.ruleId === rule.id) ?? entries.find((e) => e.ruleId === null && sameProgram(rule, e))
  return nameOf(rule, entry)
}

function speeds(e: ActivityApp | undefined): Pick<CardVM, 'down' | 'up' | 'connections'> {
  return { down: e?.down ?? 0, up: e?.up ?? 0, connections: e?.connections ?? 0 }
}

/**
 * The lane a built-in rule of the helper sends a program of no rule by, or null when it follows the main lane. The
 * helper puts such programs on its main lane, the lane of the special tiles in the same event (the default lane,
 * direct in «всё напрямую»): one on another lane goes by a rule of the helper's own; a protected file goes by its
 * built-in rule whatever the event says.
 */
function builtinLane(e: ActivityApp, mainLane: Lane): Lane | null {
  if (e.ruleId !== null) return null // a rule that is gone: the helper regroups it soon
  const known = protectedLane(e.exe)
  if (known !== undefined) return known ?? e.lane
  return e.lane !== mainLane ? e.lane : null
}

/**
 * The two lanes of the board (spec 8.4, 9.1). Pinned cards are the rules, with the speeds of the activity entry of
 * the same rule. Unpinned cards are the activity's programs of no rule, in the default lane, by name, with the special
 * tiles after them: "system" while no rule of kind system is pinned, and "unattributed", which never moves. The order
 * is stable: the activity, sorted by traffic, would move the cards under the pointer every second. A program that a
 * built-in rule of the helper routes (the system Tailscale, SplitLane itself) is a card of its own lane that never
 * moves. An activity entry of a program that a rule covers but the helper has not regrouped yet (the rule is on its
 * way, or was just normalized) is the rule's card, not a second one; an entry of a rule that is gone is an unpinned
 * card until the helper regroups it. The search keeps the cards whose name or path has it, ignoring case.
 */
export function boardView(
  apps: AppRule[],
  defaultLane: Lane,
  activity: Activity | null,
  addresses: HelperState['addresses'],
  search: string
): { vpn: LaneVM; direct: LaneVM } {
  const ruleIds = new Set(apps.map((r) => r.id))
  const byRule = new Map<string, ActivityApp>()
  const free: ActivityApp[] = []
  let systemEntry: ActivityApp | undefined
  let unattributedEntry: ActivityApp | undefined
  for (const e of activity?.apps ?? []) {
    if (e.ruleId !== null && ruleIds.has(e.ruleId)) {
      if (!byRule.has(e.ruleId)) byRule.set(e.ruleId, e)
    } else if (e.kind === 'system') {
      systemEntry ??= e
    } else if (e.kind === 'unattributed') {
      unattributedEntry ??= e
    } else {
      free.push(e)
    }
  }
  // the lane the helper put the programs of no rule on in this event (the tile is always there)
  const mainLane = unattributedEntry?.lane ?? defaultLane

  // programs of the activity that a rule covers already, by the rule's file or value
  const ruleExes = new Set(apps.filter((r) => r.exe !== '').map((r) => lower(r.exe)))
  const ruleValues = new Set(apps.filter((r) => r.matcher.value !== '').map((r) => lower(r.matcher.value)))
  const covered = new Map<string, ActivityApp>()
  const unpinnedEntries: ActivityApp[] = []
  for (const e of free) {
    if ((e.exe !== '' && ruleExes.has(lower(e.exe))) || ruleValues.has(lower(e.key))) {
      if (e.exe !== '' && !covered.has(lower(e.exe))) covered.set(lower(e.exe), e)
    } else {
      unpinnedEntries.push(e)
    }
  }

  const pinned = apps.map((r): CardVM => {
    const system = r.matcher.kind === 'system'
    const e = byRule.get(r.id) ?? (system ? systemEntry : covered.get(lower(r.exe)))
    return {
      key: r.id,
      ruleId: r.id,
      kind: system ? 'system' : 'app',
      name: nameOf(r, e),
      exe: system ? '' : r.exe,
      lane: r.lane,
      pinned: true,
      ...speeds(e),
      movable: true,
      builtin: false
    }
  })

  const programs: CardVM[] = []
  const builtin: CardVM[] = []
  for (const e of unpinnedEntries) {
    const fixed = builtinLane(e, mainLane)
    const card: CardVM = {
      key: e.key,
      ruleId: null,
      kind: 'app',
      name: e.name || baseName(e.exe),
      exe: e.exe,
      lane: fixed ?? defaultLane,
      pinned: false,
      ...speeds(e),
      movable: fixed === null,
      builtin: fixed !== null
    }
    ;(fixed === null ? programs : builtin).push(card)
  }
  programs.sort(byName)
  builtin.sort(byName)

  const tile = (kind: 'system' | 'unattributed', e: ActivityApp | undefined): CardVM => ({
    key: kind,
    ruleId: null,
    kind,
    name: t(kind === 'system' ? 'tile.system' : 'tile.unattributed'),
    exe: '',
    lane: defaultLane,
    pinned: false,
    ...speeds(e),
    movable: kind !== 'unattributed',
    builtin: false
  })
  const tiles = apps.some((r) => r.matcher.kind === 'system')
    ? [tile('unattributed', unattributedEntry)]
    : [tile('system', systemEntry), tile('unattributed', unattributedEntry)]

  const shown = (c: CardVM): boolean => matchesSearch(c.name, c.exe, search)
  const unpinned = [...programs, ...tiles].filter(shown)

  const lane = (l: Lane): LaneVM => ({
    lane: l,
    isDefault: l === defaultLane,
    pinned: pinned.filter((c) => c.lane === l && shown(c)),
    builtin: builtin.filter((c) => c.lane === l && shown(c)),
    unpinned: l === defaultLane ? unpinned : [],
    down: activity?.lanes[l].down ?? 0,
    up: activity?.lanes[l].up ?? 0,
    address: addresses[l]
  })
  return { vpn: lane('vpn'), direct: lane('direct') }
}
