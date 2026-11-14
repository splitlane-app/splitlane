import { ScreenFrame } from '../components/ScreenFrame'
import { t } from '../i18n/t'
import styles from './screens.module.css'

/** The pipe denies access: SplitLane on this computer is set up by another Windows user (spec 4.3). */
export function OtherUserScreen(): React.JSX.Element {
  return (
    <ScreenFrame title={t('screen.otherUser')}>
      <p className={styles.lead}>{t('screen.otherUserLead')}</p>
    </ScreenFrame>
  )
}
