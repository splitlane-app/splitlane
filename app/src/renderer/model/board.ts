import type { Activity, ActivityApp, AppRule, HelperState, Lane } from '../../shared/protocol'
import { t } from '../i18n/t'

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
}

export interface LaneVM {
  lane: Lane
  isDefault: boolean
  /** In the order of the rules. */
  pinned: CardVM[]
  /** Below the line, only in the default lane: the busiest first, the special tiles last. */
  unpinned: CardVM[]
  down: number
  up: number
  address: string | null
}

/** The file name without its extension: what the helper names a program after when it has no description. */
export function baseName(path: string): string {
  const file = path.slice(Math.max(path.lastIndexOf('\\'), path.lastIndexOf('/')) + 1)
  const dot = file.lastIndexOf('.')
  return dot > 0 ? file.slice(0, dot) : file
}

const lower = (s: string): string => s.toLowerCase()

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

const traffic = (c: CardVM): number => c.down + c.up

/**
 * The two lanes of the board (spec 8.4, 9.1). Pinned cards are the rules, with the speeds of the activity entry of
 * the same rule. Unpinned cards are the activity's programs of no rule, in the default lane, with the special tiles
 * after them: "system" while no rule of kind system is pinned, and "unattributed", which never moves. An activity
 * entry of a program that a rule covers but the helper has not regrouped yet (the rule is on its way, or was just
 * normalized) is the rule's card, not a second one; an entry of a rule that is gone is an unpinned card until the
 * helper regroups it. The search keeps the cards whose name or path has it, ignoring case.
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
      movable: true
    }
  })

  const programs = unpinnedEntries
    .map(
      (e): CardVM => ({
        key: e.key,
        ruleId: null,
        kind: 'app',
        name: e.name || baseName(e.exe),
        exe: e.exe,
        lane: defaultLane,
        pinned: false,
        ...speeds(e),
        movable: true
      })
    )
    .sort((a, b) => traffic(b) - traffic(a) || a.name.localeCompare(b.name, 'ru'))

  const tile = (kind: 'system' | 'unattributed', e: ActivityApp | undefined): CardVM => ({
    key: kind,
    ruleId: null,
    kind,
    name: t(kind === 'system' ? 'tile.system' : 'tile.unattributed'),
    exe: '',
    lane: defaultLane,
    pinned: false,
    ...speeds(e),
    movable: kind !== 'unattributed'
  })
  const tiles = apps.some((r) => r.matcher.kind === 'system')
    ? [tile('unattributed', unattributedEntry)]
    : [tile('system', systemEntry), tile('unattributed', unattributedEntry)]

  const q = lower(search.trim())
  const shown = (c: CardVM): boolean => q === '' || lower(c.name).includes(q) || lower(c.exe).includes(q)
  const unpinned = [...programs, ...tiles].filter(shown)

  const lane = (l: Lane): LaneVM => ({
    lane: l,
    isDefault: l === defaultLane,
    pinned: pinned.filter((c) => c.lane === l && shown(c)),
    unpinned: l === defaultLane ? unpinned : [],
    down: activity?.lanes[l].down ?? 0,
    up: activity?.lanes[l].up ?? 0,
    address: addresses[l]
  })
  return { vpn: lane('vpn'), direct: lane('direct') }
}
