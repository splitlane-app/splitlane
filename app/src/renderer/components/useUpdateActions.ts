import { useState } from 'react'
import { type UpdateAction } from '../model/update'
import { useServices } from '../services'

/**
 * What the buttons of an update state do, shared by the banner and the status line of «Подключение»: `run` calls the
 * bridge, `busy` holds the buttons of the actions that wait for the main process.
 */
export function useUpdateActions(): { busy: boolean; run(action: UpdateAction): Promise<void> } {
  const { bridge } = useServices()
  const [busy, setBusy] = useState(false)

  const perform = (action: UpdateAction): Promise<unknown> => {
    switch (action) {
      case 'install':
      case 'retry':
        return bridge.installUpdate()
      case 'notes':
        return bridge.openUpdateNotes()
      case 'later':
        return bridge.updateLater()
      case 'cancel':
        return bridge.cancelUpdate()
      case 'dismiss':
        return bridge.dismissUpdate()
    }
  }
  const run = async (action: UpdateAction): Promise<void> => {
    // the main process answers at once, but the buttons never wait for a download: «Отменить» must work
    if (action === 'install' || action === 'retry' || action === 'cancel') {
      perform(action).catch(() => {})
      return
    }
    setBusy(true)
    try {
      await perform(action)
    } catch {
      // the next snapshot shows what the main process did
    } finally {
      setBusy(false)
    }
  }
  return { busy, run }
}
