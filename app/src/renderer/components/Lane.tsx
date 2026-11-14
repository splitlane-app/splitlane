import { useId, useState } from 'react'
import type { Lane as LaneName } from '../../shared/protocol'
import { t } from '../i18n/t'
import { selectedExitNodeName } from '../model/banners'
import type { LaneVM } from '../model/board'
import { formatSpeed, maskAddress } from '../model/format'
import { useApp, useServices } from '../services'
import { isLivePhase } from '../store/appStore'
import { AppCard } from './AppCard'
import styles from './Lane.module.css'

/** The total speed of a lane: «↓ 1,3 МБ/с · ↑ 90,0 КБ/с», each part only when there is traffic that way. */
function LaneSpeed({ down, up }: { down: number; up: number }): React.JSX.Element {
  const parts: string[] = []
  if (down > 0) parts.push(t('speed.down', { speed: formatSpeed(down) }))
  if (up > 0) parts.push(t('speed.up', { speed: formatSpeed(up) }))
  return <span className={styles.speed}>{parts.join(t('sep'))}</span>
}

/**
 * What the lane means for a program (spec 9.1): the VPN lane names the server and its address; the direct one
 * shows the real address masked (the first two numbers), the whole of it while pressed.
 */
function Sees({ lane, address }: { lane: LaneName; address: string | null }): React.JSX.Element {
  const server = useApp((s) => (s.snapshot?.state ? selectedExitNodeName(s.snapshot.state) : null))
  const [shown, setShown] = useState(false)
  if (lane === 'vpn') {
    return (
      <p className={styles.sees}>
        {t('lane.vpnSees', { name: server ?? t('banner.serverFallback') })}
        {address !== null && (
          <>
            {t('sep')}
            <span className={styles.address}>{address}</span>
          </>
        )}
      </p>
    )
  }
  return (
    <p className={styles.sees}>
      {t('lane.directSees')}
      {address !== null && (
        <>
          {t('sep')}
          <button
            type="button"
            className={styles.reveal}
            aria-pressed={shown}
            title={shown ? undefined : t('lane.showAddress')}
            onClick={() => setShown((v) => !v)}
          >
            {shown ? address : maskAddress(address)}
          </button>
        </>
      )}
    </p>
  )
}

/**
 * One lane of the board (spec 9.1): the title in the lane's colour with the total speed, «основная» or
 * «сделать основной», the explanation line, the pinned cards; in the default lane the line «остальные программы»
 * with the unpinned cards and the special tiles below it — only while the core runs (8.4), an explanation
 * otherwise; in the other lane the zone to drop programs on.
 */
export function Lane({ vm, onMakeDefault }: { vm: LaneVM; onMakeDefault(): void }): React.JSX.Element {
  const { store } = useServices()
  const live = useApp((s) => isLivePhase(s.snapshot?.state?.phase))
  const titleId = useId()
  const open = (key: string): void => store.getState().openCard(key)

  return (
    <section className={styles.lane} data-lane={vm.lane} aria-labelledby={titleId}>
      <header className={styles.head}>
        <h2 id={titleId} className={styles.title}>
          {t(vm.lane === 'vpn' ? 'lane.vpn' : 'lane.direct')}
        </h2>
        {vm.isDefault && <span className={styles.badge}>{t('lane.main')}</span>}
        <span className={styles.grow} />
        {!vm.isDefault && (
          <button type="button" className={styles.make} onClick={onMakeDefault}>
            {t('lane.makeMain')}
          </button>
        )}
        <LaneSpeed down={vm.down} up={vm.up} />
      </header>
      <Sees lane={vm.lane} address={vm.address} />
      <div className={styles.body}>
        {vm.pinned.length > 0 && (
          <div className={styles.cards}>
            {vm.pinned.map((card) => (
              <AppCard key={card.key} card={card} onOpen={open} />
            ))}
          </div>
        )}
        {vm.isDefault ? (
          <>
            <div className={styles.line} role="separator">
              <span>{t('lane.others')}</span>
            </div>
            {live ? (
              <div className={styles.cards}>
                {vm.unpinned.map((card) => (
                  <AppCard key={card.key} card={card} onOpen={open} />
                ))}
              </div>
            ) : (
              <p className={styles.note}>{t('lane.othersOff')}</p>
            )}
          </>
        ) : (
          <div className={styles.drop}>{t('lane.dropHint')}</div>
        )}
      </div>
    </section>
  )
}
