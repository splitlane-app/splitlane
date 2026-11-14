export interface Program {
  exe: string
  name: string
}

export type Dropped = { kind: 'program'; program: Program } | { kind: 'skipped'; name: string }
