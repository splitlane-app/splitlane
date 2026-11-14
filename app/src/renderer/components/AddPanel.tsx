import { useDraggable } from '@dnd-kit/react'
import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import type { Program } from '../../shared/programs'
import type { Lane } from '../../shared/protocol'
import { ADD_ROW, CARD, CARD_SENSORS } from '../dnd/BoardDnd'
import { CloseIcon } from '../icons/builtin'
import { t } from '../i18n/t'
import { installedRows, onlineRows, type AddRow } from '../model/addRows'
import { boardView, type CardVM } from '../model/board'
import { addPrograms, moveTo, pin } from '../model/layout'
import { useApp, useServices } from '../services'
import { effectiveLayout, isLivePhase } from '../store/appStore'
import styles from './AddPanel.module.css'
import { ExeIcon } from './ExeIcon'
import { useDialog } from './useDialog'

type Tab = 'online' | 'installed'
const TABS: readonly Tab[] = ['online', 'installed']

const laneName = (lane: Lane): string => t(lane === 'vpn' ? 'lane.vpn' : 'lane.direct')

/** The part of a path after the last separator's folder: «Папка\файл.exe», enough to tell programs apart. */
function shortPath(exe: string): string {
  const parts = exe.split('\\')
  return parts.slice(-2).join('\\')
}

interface RowProps {
  row: AddRow
  /** An online row is the card of the board; the others are programs. */
  card?: CardVM
  onAct(row: AddRow, action: AddRow['actions'][number], card: CardVM | undefined): void
}

/**
 * A row of the panel: the program (it can be dragged onto a lane) and its buttons. A button of the lane the
 * program is in already is disabled.
 */
function Row({ row, card, onAct }: RowProps): React.JSX.Element {
  const program: Program = { exe: row.exe, name: row.name }
  const { ref, handleRef } = useDraggable({
    id: `add:${row.key}`,
    data: card ?? program,
    type: card !== undefined ? CARD : ADD_ROW,
    sensors: CARD_SENSORS
  })
  return (
    <li ref={ref} className={styles.row} data-add-row={row.key}>
      <div ref={handleRef} className={styles.info} title={row.exe}>
        <ExeIcon exe={row.exe} />
        <span className={styles.text}>
          <span className={styles.name}>{row.name}</span>
          <span className={styles.path}>
            {row.currentLane !== null
              ? t(row.pinned ? 'add.pinnedIn' : 'add.here', { lane: laneName(row.currentLane) })
              : shortPath(row.exe)}
          </span>
        </span>
      </div>
      <div className={styles.buttons}>
        {row.actions.map((a) => (
          <button
            key={a}
            type="button"
            className={styles.act}
            data-lane={a === 'pin' ? undefined : a}
            disabled={a !== 'pin' && row.currentLane === a && row.pinned}
            aria-label={`${t(a === 'pin' ? 'pin' : a === 'vpn' ? 'lane.vpn' : 'lane.direct')}: ${row.name}`}
            onClick={() => onAct(row, a, card)}
          >
            {t(a === 'pin' ? 'pin' : a === 'vpn' ? 'lane.vpn' : 'lane.direct')}
          </button>
        ))}
      </div>
    </li>
  )
}

/**
 * The «+ Добавить» panel (spec 9.3), docked beside the lanes so that a row can be dragged onto either of them. «Сейчас в
 * сети»: the unpinned programs of the live activity (nothing while the routing is off). «Установленные»: the programs
 * of the Start menu and the Store, asked of the main process once each time the panel opens. «Выбрать файл…» picks
 * an .exe and shows it at the top, so the lane is chosen explicitly.
 */
export function AddPanel(): React.JSX.Element {
  const { store, bridge } = useServices()
  const layout = useApp(effectiveLayout)
  const activity = useApp((s) => s.activity)
  const addresses = useApp((s) => s.snapshot?.state?.addresses ?? null)
  const live = useApp((s) => isLivePhase(s.snapshot?.state?.phase))
  const [tab, setTab] = useState<Tab>('online')
  const [installed, setInstalled] = useState<Program[] | 'failed' | null>(null)
  const [picked, setPicked] = useState<Program | null>(null)
  const ref = useRef<HTMLElement | null>(null)
  const close = (): void => store.getState().openAdd(false)
  const dialog = useDialog(ref, close)

  useEffect(() => {
    if (tab !== 'installed' || installed !== null) return
    let current = true
    bridge.installedPrograms().then(
      (list) => current && setInstalled(list),
      () => current && setInstalled('failed')
    )
    return () => {
      current = false
    }
  }, [tab, installed, bridge])

  const view = useMemo(
    () =>
      layout === null || addresses === null
        ? null
        : boardView(layout.apps, layout.defaultLane, activity, addresses, ''),
    [layout, addresses, activity]
  )
  const online = useMemo(() => (view === null || !live ? [] : onlineRows(view)), [view, live])
  const cards = useMemo(() => {
    const map = new Map<string, CardVM>()
    for (const c of view?.vpn.unpinned ?? []) map.set(c.key, c)
    for (const c of view?.direct.unpinned ?? []) map.set(c.key, c)
    return map
  }, [view])
  const installedList = useMemo(
    () => (layout === null || !Array.isArray(installed) ? [] : installedRows(installed, layout)),
    [layout, installed]
  )
  const pickedRow = useMemo(
    () => (layout === null || picked === null ? null : installedRows([picked], layout)[0]),
    [layout, picked]
  )

  const act = (row: AddRow, action: AddRow['actions'][number], card: CardVM | undefined): void => {
    if (card !== undefined) {
      void store
        .getState()
        .change((l, newId) => (action === 'pin' ? pin(l, card, newId) : moveTo(l, card, action, newId)))
    } else if (action !== 'pin') {
      void store.getState().change((l, newId) => addPrograms(l, [{ exe: row.exe, name: row.name }], action, newId))
    }
  }

  const pick = (): void => {
    bridge.pickProgram().then(
      (p) => {
        if (p !== null) setPicked(p)
      },
      () => {}
    )
  }

  const onTabKey = (e: KeyboardEvent<HTMLButtonElement>): void => {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
    e.preventDefault()
    const next = TABS[(TABS.indexOf(tab) + (e.key === 'ArrowRight' ? 1 : TABS.length - 1)) % TABS.length]
    setTab(next)
    document.getElementById(`add-tab-${next}`)?.focus()
  }

  return (
    <aside ref={ref} className={styles.panel} role="dialog" aria-modal="false" aria-label={t('add.title')} {...dialog}>
      <header className={styles.head}>
        <h2 className={styles.title}>{t('add.title')}</h2>
        <button type="button" className={styles.close} aria-label={t('add.close')} onClick={close}>
          <CloseIcon />
        </button>
      </header>

      <div className={styles.tabs} role="tablist" aria-label={t('add.tabs')}>
        {TABS.map((id) => (
          <button
            key={id}
            id={`add-tab-${id}`}
            type="button"
            role="tab"
            aria-selected={tab === id}
            aria-controls="add-tabpanel"
            tabIndex={tab === id ? 0 : -1}
            className={styles.tab}
            onClick={() => setTab(id)}
            onKeyDown={onTabKey}
          >
            {t(id === 'online' ? 'add.tabOnline' : 'add.tabInstalled')}
          </button>
        ))}
      </div>

      <button type="button" className={styles.pick} onClick={pick}>
        {t('add.pick')}
      </button>
      {pickedRow !== null && (
        <section className={styles.picked} aria-label={t('add.picked')}>
          <ul className={styles.list}>
            <Row row={pickedRow} onAct={act} />
          </ul>
        </section>
      )}

      <div id="add-tabpanel" role="tabpanel" aria-labelledby={`add-tab-${tab}`} className={styles.body}>
        {tab === 'online' ? (
          !live ? (
            <p className={styles.note}>{t('add.onlineOff')}</p>
          ) : online.length === 0 ? (
            <p className={styles.note}>{t('add.onlineEmpty')}</p>
          ) : (
            <ul className={styles.list}>
              {online.map((row) => (
                <Row key={row.key} row={row} card={cards.get(row.key)} onAct={act} />
              ))}
            </ul>
          )
        ) : installed === null ? (
          <p className={styles.note}>{t('add.installedLoading')}</p>
        ) : installed === 'failed' ? (
          <p className={styles.note}>{t('add.installedFailed')}</p>
        ) : installedList.length === 0 ? (
          <p className={styles.note}>{t('add.installedEmpty')}</p>
        ) : (
          <ul className={styles.list}>
            {installedList.map((row) => (
              <Row key={row.key} row={row} onAct={act} />
            ))}
          </ul>
        )}
      </div>
      <p className={styles.hint}>{t('add.dragHint')}</p>
    </aside>
  )
}
