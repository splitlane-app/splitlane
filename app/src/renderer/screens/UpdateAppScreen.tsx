import { ScreenFrame } from '../components/ScreenFrame'
import { UpdateBanner } from '../components/UpdateBanner'
import { t } from '../i18n/t'
import { updateBusy } from '../model/update'
import { useApp, useServices } from '../services'
import styles from './screens.module.css'

/** The helper speaks a newer protocol than the window (spec 5.1). */
export function UpdateAppScreen(): React.JSX.Element {
  const { bridge } = useServices()
  const update = useApp((s) => s.snapshot?.update ?? null)
  return (
    <ScreenFrame title={t('screen.updateApp')}>
      <UpdateBanner />
      <p className={styles.lead}>{t('screen.updateAppLead')}</p>
      <div className={styles.actions}>
        <button
          type="button"
          className={styles.button}
          disabled={update !== null && updateBusy(update)}
          onClick={() => {
            bridge.checkUpdates().catch(() => {})
          }}
        >
          {t('update.check')}
        </button>
      </div>
    </ScreenFrame>
  )
}
