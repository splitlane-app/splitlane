import { useEffect } from 'react'
import { CloseIcon } from '../icons/builtin'
import { t } from '../i18n/t'
import { updateBannerModel } from '../model/update'
import { useApp } from '../services'
import styles from './UpdateBanner.module.css'
import { useUpdateActions } from './useUpdateActions'

/** How long the notice of a finished update stays before it closes by itself. */
const INSTALLED_MS = 10_000

/** The update banner under the state banner of the board; nothing when there is no offer or result to show. */
export function UpdateBanner(): React.JSX.Element | null {
  const update = useApp((s) => s.snapshot?.update ?? null)
  const { busy, run } = useUpdateActions()
  const installed = update?.kind === 'installed'

  // the success notice has nothing to decide: it closes by itself
  useEffect(() => {
    if (!installed) return
    const timer = setTimeout(() => void run('dismiss'), INSTALLED_MS)
    return () => clearTimeout(timer)
    // run is new on every render; the timer belongs to the notice, not to the render
  }, [installed])

  if (update === null) return null
  const banner = updateBannerModel(update)
  if (banner === null) return null

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
      {installed && (
        <button
          type="button"
          className={styles.close}
          aria-label={t('update.action.dismiss')}
          onClick={() => void run('dismiss')}
        >
          <CloseIcon />
        </button>
      )}
    </div>
  )
}
