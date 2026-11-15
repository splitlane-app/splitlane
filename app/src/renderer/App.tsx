import { useEffect } from 'react'
import { Board } from './components/Board'
import { screenFor } from './model/connect'
import { ConnectionScreen, ConnectScreen } from './screens/ConnectScreen'
import { OtherUserScreen } from './screens/OtherUserScreen'
import { SetupScreen } from './screens/SetupScreen'
import { Splash } from './screens/Splash'
import { UpdateAppScreen } from './screens/UpdateAppScreen'
import { useApp, useLang } from './services'

/**
 * The screen by the link and the state (model/connect.ts): the splash until there is a state, the screens of the
 * link, the connect screen, the connection screen, the board. A link that restarts or does not answer keeps the
 * screen of the last state, with its banner. Every snapshot renders the screen again (no component below is
 * memoized; the memos of texts take the language as a dependency), so a new language shows at once; it is also the
 * `lang` of the document.
 */
export function App(): React.JSX.Element {
  const snapshot = useApp((s) => s.snapshot)
  const uiScreen = useApp((s) => s.ui.screen)
  const lang = useLang()
  useEffect(() => {
    document.documentElement.lang = lang
  }, [lang])

  switch (snapshot === null ? 'splash' : screenFor(snapshot, uiScreen)) {
    case 'setup':
      return <SetupScreen update={false} />
    case 'update-helper':
      return <SetupScreen update />
    case 'update-app':
      return <UpdateAppScreen />
    case 'other-user':
      return <OtherUserScreen />
    case 'connect':
      return <ConnectScreen />
    case 'connection':
      return <ConnectionScreen />
    case 'board':
      return <Board />
    case 'splash':
      return <Splash />
  }
}
