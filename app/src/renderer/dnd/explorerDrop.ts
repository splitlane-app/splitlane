import type { SplitLaneBridge } from '../../shared/ipc'
import type { Program, SkipReason } from '../../shared/programs'
import type { Lane } from '../../shared/protocol'
import { t } from '../i18n/t'
import { addPrograms } from '../model/layout'
import type { Services } from '../services'
import { errorText } from '../store/appStore'

/** A dropped or picked file that is no program, and why. */
export interface Skipped {
  name: string
  reason: SkipReason
}

/**
 * The files dropped from Explorer on lane (spec 9.2, 9.8): the main process makes programs of the .exe files, the
 * shortcuts to them and the shortcuts of installed Steam and Epic games, and names the rest. Only the paths cross
 * (the preload takes them with webUtils.getPathForFile); a drop with no files asks nothing. The caller pins the
 * programs in lane.
 */
export async function explorerDrop(
  files: File[],
  lane: Lane,
  bridge: Pick<SplitLaneBridge, 'resolveDropped'>
): Promise<{ programs: Program[]; skipped: Skipped[] }> {
  const programs: Program[] = []
  const skipped: Skipped[] = []
  if (files.length === 0) return { programs, skipped }
  for (const d of await bridge.resolveDropped(files)) {
    if (d.kind === 'program') programs.push(d.program)
    else skipped.push({ name: d.name, reason: d.reason })
  }
  return { programs, skipped }
}

/**
 * The notice for the files that are no programs: «Не программа: …», and for shortcuts of games that are not found
 * the hint to run the game (it then shows up in «Сейчас в сети»). Null when nothing was skipped.
 */
export function skippedNotice(skipped: Skipped[]): string | null {
  const other = skipped.filter((s) => s.reason !== 'game').map((s) => s.name)
  const games = skipped.filter((s) => s.reason === 'game').map((s) => s.name)
  const parts: string[] = []
  if (other.length > 0) parts.push(t('notice.notProgram', { names: other.join(', ') }))
  if (games.length > 0) parts.push(t(games.length === 1 ? 'notice.gameLink' : 'notice.gameLinks', { names: games.join(', ') }))
  return parts.length === 0 ? null : parts.join('; ')
}

/** Is it a drag of files (from Explorer), not of text or of a card of the board? */
export function hasFiles(data: DataTransfer | null): boolean {
  return data !== null && Array.from(data.types).includes('Files')
}

/**
 * A drop of files on a lane: the programs pinned there at once, a notice for the files that are not; a failure of
 * the bridge shows its notice.
 */
export async function dropFiles({ bridge, store }: Services, files: File[], lane: Lane): Promise<void> {
  let dropped: Awaited<ReturnType<typeof explorerDrop>>
  try {
    dropped = await explorerDrop(files, lane, bridge)
  } catch (e) {
    store.getState().showNotice(errorText({ code: 'internal', message: e instanceof Error ? e.message : String(e) }))
    return
  }
  const { programs, skipped } = dropped
  const notice = skippedNotice(skipped)
  if (notice !== null) store.getState().showNotice(notice)
  if (programs.length > 0) await store.getState().change((l, newId) => addPrograms(l, programs, lane, newId))
}
