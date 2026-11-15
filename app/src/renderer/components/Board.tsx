import { useMemo } from 'react'
import type { Lane as LaneName } from '../../shared/protocol'
import { BoardDnd } from '../dnd/BoardDnd'
import { boardView } from '../model/board'
import { makeDefault } from '../model/layout'
import { useApp, useServices } from '../services'
import { effectiveLayout } from '../store/appStore'
import { AddPanel } from './AddPanel'
import { Banner } from './Banner'
import styles from './Board.module.css'
import { Header } from './Header'
import { Lane } from './Lane'
import { Notice } from './Notice'
import { ProgramCard } from './ProgramCard'
import { UpdateBanner } from './UpdateBanner'

/**
 * The main window (spec 9.1): the header, the state banner and the two lanes side by side, VPN on the left; the
 * cards drag between them (9.2).
 */
export function Board(): React.JSX.Element {
  const { store } = useServices()
  const layout = useApp(effectiveLayout)
  const activity = useApp((s) => s.activity)
  const addresses = useApp((s) => s.snapshot?.state?.addresses ?? null)
  const search = useApp((s) => s.search)
  const addOpen = useApp((s) => s.ui.addOpen)
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
    <BoardDnd>
      <div className={styles.board}>
        <Header />
        <div className={styles.bannerSlot}>
          <Banner />
          <UpdateBanner />
        </div>
        <div className={styles.content}>
          {view !== null && (
            <main className={styles.lanes}>
              <Lane vm={view.vpn} onMakeDefault={() => setDefault('vpn')} />
              <Lane vm={view.direct} onMakeDefault={() => setDefault('direct')} />
            </main>
          )}
          {addOpen && <AddPanel />}
        </div>
        <ProgramCard />
        <Notice />
      </div>
    </BoardDnd>
  )
}
