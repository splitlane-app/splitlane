import { useMemo } from 'react'
import type { Lane as LaneName } from '../../shared/protocol'
import { boardView } from '../model/board'
import { makeDefault } from '../model/layout'
import { useApp, useServices } from '../services'
import { effectiveLayout } from '../store/appStore'
import { Banner } from './Banner'
import styles from './Board.module.css'
import { Header } from './Header'
import { Lane } from './Lane'
import { Notice } from './Notice'

/** The main window (spec 9.1): the header, the state banner and the two lanes side by side, VPN on the left. */
export function Board(): React.JSX.Element {
  const { store } = useServices()
  const layout = useApp(effectiveLayout)
  const activity = useApp((s) => s.activity)
  const addresses = useApp((s) => s.snapshot?.state?.addresses ?? null)
  const search = useApp((s) => s.search)
  const view = useMemo(
    () =>
      layout === null || addresses === null
        ? null
        : boardView(layout.apps, layout.defaultLane, activity, addresses, search),
    [layout, activity, addresses, search]
  )
  const setDefault = (lane: LaneName): void => {
    void store.getState().change((l) => makeDefault(l, lane))
  }

  return (
    <div className={styles.board}>
      <Header />
      <div className={styles.bannerSlot}>
        <Banner />
      </div>
      {view !== null && (
        <main className={styles.lanes}>
          <Lane vm={view.vpn} onMakeDefault={() => setDefault('vpn')} />
          <Lane vm={view.direct} onMakeDefault={() => setDefault('direct')} />
        </main>
      )}
      <Notice />
    </div>
  )
}
