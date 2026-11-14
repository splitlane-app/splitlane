import type { LinkStatus } from '../shared/status'
import { Banner } from './components/Banner'
import { Board } from './components/Board'
import { Placeholder } from './components/Placeholder'
import type { Key } from './i18n/ru'
import { t } from './i18n/t'
import { useApp, useServices } from './services'

/** The links that have a screen of their own instead of the board (task 13 gives them their content). */
const LINK_SCREENS: Partial<Record<LinkStatus['kind'], Key>> = {
  'setup-needed': 'screen.setup',
  'other-user': 'screen.otherUser',
  'helper-older': 'screen.updateHelper',
  'helper-newer': 'screen.updateApp'
}

/**
 * The screen by the link and the state: the splash until there is a state (with the banner of a link that is
 * restarting or not answering), the screens of the link, the connection screen, the board. A link that restarts
 * or does not answer keeps the board of the last state, with its banner.
 */
export function App(): React.JSX.Element {
  const { store } = useServices()
  const link = useApp((s) => s.snapshot?.link.kind ?? 'connecting')
  const hasState = useApp((s) => (s.snapshot?.state ?? null) !== null)
  const screen = useApp((s) => s.ui.screen)

  const own = LINK_SCREENS[link]
  if (own !== undefined) return <Placeholder title={t(own)} />
  if (link === 'connecting' || !hasState) {
    return (
      <Placeholder title={t('screen.splash')}>
        <Banner />
      </Placeholder>
    )
  }
  if (screen === 'connection') {
    return (
      <Placeholder title={t('screen.connection')}>
        <button type="button" onClick={() => store.getState().showConnection(false)}>
          {t('screen.back')}
        </button>
      </Placeholder>
    )
  }
  return <Board />
}
