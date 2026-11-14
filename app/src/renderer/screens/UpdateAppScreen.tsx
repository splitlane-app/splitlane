import { ScreenFrame } from '../components/ScreenFrame'
import { t } from '../i18n/t'
import styles from './screens.module.css'

/** The helper speaks a newer protocol than the window (spec 5.1). */
export function UpdateAppScreen(): React.JSX.Element {
  return (
    <ScreenFrame title={t('screen.updateApp')}>
      <p className={styles.lead}>{t('screen.updateAppLead')}</p>
    </ScreenFrame>
  )
}
