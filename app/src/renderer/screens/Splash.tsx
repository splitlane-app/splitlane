import { Banner } from '../components/Banner'
import { ScreenFrame } from '../components/ScreenFrame'
import { t } from '../i18n/t'

/** The wait for the helper, with the banner of a link that is restarting or not answering. */
export function Splash(): React.JSX.Element {
  return (
    <ScreenFrame title={t('screen.splash')}>
      <Banner />
    </ScreenFrame>
  )
}
