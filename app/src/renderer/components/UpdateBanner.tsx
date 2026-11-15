import { useState } from 'react'
import { t } from '../i18n/t'
import { updateBannerModel, type UpdateAction } from '../model/update'
import { useApp, useServices } from '../services'
import styles from './UpdateBanner.module.css'

/** The update banner under the state banner of the board; nothing when there is no offer or result to show. */
export function UpdateBanner(): React.JSX.Element | null {
  const { bridge } = useServices()
  const update = useApp((s) => s.snapshot?.update ?? null)
  const [busy, setBusy] = useState(false)
  if (update === null) return null
  const banner = updateBannerModel(update)
  if (banner === null) return null

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
    setBusy(true)
    try {
      await perform(action)
    } catch {
      // the next snapshot shows what the main process did
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className={styles.banner} data-tone={banner.tone} role={banner.tone === 'info' ? 'status' : 'alert'}>
      <div className={styles.body}>
        <p className={styles.title}>
          <span className={styles.dot} aria-hidden="true" />
          {banner.text}
        </p>
      </div>
      {banner.actions.length > 0 && (
        <div className={styles.actions}>
          {banner.actions.map((action, i) => (
            <button
              key={action}
              type="button"
              className={i === 0 ? styles.primary : styles.ghost}
              disabled={busy}
              onClick={() => void run(action)}
            >
              {t(`update.action.${action}`)}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
