import type { SplitLaneBridge } from '../../shared/ipc'
import type { Program } from '../../shared/programs'
import type { Lane } from '../../shared/protocol'
import { t } from '../i18n/t'
import { addPrograms } from '../model/layout'
import type { Services } from '../services'
import { errorText } from '../store/appStore'

/**
 * The files dropped from Explorer on lane (spec 9.2, 9.8): the main process makes programs of the .exe files and
 * the shortcuts to them, and names the rest. Only the paths cross (the preload takes them with
 * webUtils.getPathForFile); a drop with no files asks nothing. The caller pins the programs in lane.
 */
export async function explorerDrop(
  files: File[],
  lane: Lane,
  bridge: Pick<SplitLaneBridge, 'resolveDropped'>
): Promise<{ programs: Program[]; skipped: string[] }> {
  const programs: Program[] = []
  const skipped: string[] = []
  if (files.length === 0) return { programs, skipped }
  for (const d of await bridge.resolveDropped(files)) {
    if (d.kind === 'program') programs.push(d.program)
    else skipped.push(d.name)
  }
  return { programs, skipped }
}

/** Is it a drag of files (from Explorer), not of text or of a card of the board? */
export function hasFiles(data: DataTransfer | null): boolean {
  return data !== null && Array.from(data.types).includes('Files')
}

/**
 * A drop of files on a lane: the programs pinned there at once, «Не программа: …» for the files that are not;
 * a failure of the bridge shows its notice.
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
  if (skipped.length > 0) store.getState().showNotice(t('notice.notProgram', { names: skipped.join(', ') }))
  if (programs.length > 0) await store.getState().change((l, newId) => addPrograms(l, programs, lane, newId))
}
