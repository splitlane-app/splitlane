export interface Program {
  exe: string
  name: string
}

/** Why a file is no program: a shortcut of a game that is not found on this PC, or anything else. */
export type SkipReason = 'game' | 'notProgram'

export type Dropped = { kind: 'program'; program: Program } | { kind: 'skipped'; name: string; reason: SkipReason }
