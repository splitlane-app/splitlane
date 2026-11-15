import { useEffect, useId, useMemo, useRef } from 'react'
import type { Lane } from '../../shared/protocol'
import { otherLane } from '../dnd/dropTarget'
import { CloseIcon } from '../icons/builtin'
import { t } from '../i18n/t'
import { boardView, fileName, type CardVM } from '../model/board'
import { formatSpeed } from '../model/format'
import { moveTo, pin, setWholeFolder, unpin } from '../model/layout'
import { useApp, useServices } from '../services'
import { effectiveLayout, isLivePhase, type LayoutChange } from '../store/appStore'
import { ExeIcon } from './ExeIcon'
import styles from './ProgramCard.module.css'
import { Toggle } from './Toggle'
import { useDialog } from './useDialog'

/** The most connections the helper reports (and the card lists) for one program. */
const MAX_CONNECTIONS = 500

const laneName = (lane: Lane): string => t(lane === 'vpn' ? 'lane.vpn' : 'lane.direct')

function stateText(card: CardVM): string {
  if (card.builtin) return t(card.lane === 'vpn' ? 'program.stateBuiltinVpn' : 'program.stateBuiltinDirect')
  if (!card.pinned) return t('program.stateUnpinned')
  return t(card.lane === 'vpn' ? 'program.statePinnedVpn' : 'program.statePinnedDirect')
}

/**
 * The program card (spec 9.4), a dialog over the board for the card whose key is `ui.cardKey`: the name, the icon,
 * the program's file and the state, the buttons that change its lane or unpin it, what counts as this program (the «Вся папка
 * программы» switch, the path, the files seen) and its connections now. While it is open the page subscribes to the
 * activity detail of its key (8.4).
 */
export function ProgramCard(): React.JSX.Element | null {
  const key = useApp((s) => s.ui.cardKey)
  if (key === null) return null
  return <CardDialog key={key} cardKey={key} />
}

function CardDialog({ cardKey }: { cardKey: string }): React.JSX.Element | null {
  const { store, bridge } = useServices()
  const layout = useApp(effectiveLayout)
  const activity = useApp((s) => s.activity)
  const addresses = useApp((s) => s.snapshot?.state?.addresses ?? null)
  const live = useApp((s) => isLivePhase(s.snapshot?.state?.phase))
  const ref = useRef<HTMLDivElement | null>(null)
  const titleId = useId()
  const close = (): void => store.getState().openCard(null)
  const dialog = useDialog(ref, close)

  useEffect(() => {
    bridge.setActivityDetail(cardKey).catch(() => {})
    return () => {
      bridge.setActivityDetail(null).catch(() => {})
    }
  }, [bridge, cardKey])

  const last = useRef<CardVM | null>(null)
  const cards = useMemo(() => {
    if (layout === null || addresses === null) return null
    const view = boardView(layout.apps, layout.defaultLane, activity, addresses, '')
    return [
      ...view.vpn.pinned,
      ...view.direct.pinned,
      ...view.vpn.builtin,
      ...view.direct.builtin,
      ...view.vpn.unpinned,
      ...view.direct.unpinned
    ]
  }, [layout, addresses, activity])
  const found = cards?.find((c) => c.key === cardKey)
  if (found !== undefined) last.current = found
  // the program under another key (pinned, moved, unpinned from here): the card follows it
  const prev = last.current
  const successor =
    cards === null || found !== undefined || prev === null
      ? undefined
      : cards.find(
          (c) =>
            c.key !== cardKey &&
            (prev.kind === 'system'
              ? c.kind === 'system'
              : c.kind === 'app' && prev.exe !== '' && c.exe.toLowerCase() === prev.exe.toLowerCase())
        )
  // an unpinned program that went quiet keeps its card; a pinned one that is gone for good closes it
  const vanished = cards !== null && found === undefined && successor === undefined && (prev === null || prev.pinned)
  const successorKey = successor?.key
  useEffect(() => {
    if (successorKey !== undefined) store.getState().openCard(successorKey)
    else if (vanished) store.getState().openCard(null)
  }, [successorKey, vanished, store])

  const card = found ?? prev
  if (card === null || layout === null) return null

  const rule = card.ruleId === null ? undefined : layout.apps.find((r) => r.id === card.ruleId)
  const kind = rule?.matcher.kind
  const detail = activity?.detail?.key === cardKey ? activity.detail : undefined
  const connections = detail?.connections.slice(0, MAX_CONNECTIONS) ?? []
  const change = (op: LayoutChange): void => {
    void store.getState().change(op)
  }
  const target = otherLane(card.lane)
  const path = rule !== undefined ? rule.matcher.value : card.kind === 'app' ? card.key : ''
  const canSwitch = kind === 'folder' || kind === 'file'
  const file = card.kind === 'app' ? fileName(card.exe) : ''

  return (
    <div className={styles.backdrop} onMouseDown={(e) => e.target === e.currentTarget && close()}>
      <div
        ref={ref}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className={styles.dialog}
        data-lane={card.lane}
        {...dialog}
      >
        <header className={styles.head}>
          <ExeIcon exe={card.exe} system={card.kind === 'system'} />
          <div className={styles.titles}>
            <h2 id={titleId} className={styles.name}>
              {card.name}
            </h2>
            {file !== '' && <p className={styles.file}>{file}</p>}
            <p className={styles.state}>{stateText(card)}</p>
          </div>
          <button type="button" className={styles.close} aria-label={t('program.close')} onClick={close}>
            <CloseIcon />
          </button>
        </header>

        {card.movable && (
          <div className={styles.actions}>
            <button
              type="button"
              className={styles.action}
              data-lane={target}
              onClick={() => change((l, newId) => moveTo(l, card, target, newId))}
            >
              {t(target === 'vpn' ? 'move.vpn' : 'move.direct')}
            </button>
            {card.ruleId === null ? (
              <button type="button" className={styles.action} onClick={() => change((l, newId) => pin(l, card, newId))}>
                {t('pin')}
              </button>
            ) : (
              <button
                type="button"
                className={styles.action}
                onClick={() => {
                  const id = card.ruleId
                  if (id !== null) change((l) => unpin(l, id))
                }}
              >
                {t('unpin')}
              </button>
            )}
          </div>
        )}

        {card.kind === 'app' && (
          <section className={styles.block} aria-label={t('program.identity')}>
            <h3 className={styles.blockTitle}>{t('program.identity')}</h3>
            {rule !== undefined && (
              <div className={styles.switchRow}>
                <Toggle
                  on={kind === 'folder'}
                  disabled={!canSwitch}
                  label={t('program.wholeFolder')}
                  caption={t('program.wholeFolder')}
                  onToggle={() => {
                    const id = rule.id
                    change((l) => setWholeFolder(l, id, kind !== 'folder'))
                  }}
                />
                <span className={styles.hint}>
                  {!canSwitch
                    ? t('program.wholeFolderFixed')
                    : kind === 'folder'
                      ? t('program.wholeFolderOn')
                      : t('program.wholeFolderOff')}
                </span>
              </div>
            )}
            {path !== '' && (
              <p className={styles.path}>
                <span className={styles.label}>{t('program.path')}</span>
                <span className={styles.value}>{path}</span>
              </p>
            )}
            <div className={styles.exes}>
              <span className={styles.label}>{t('program.exes')}</span>
              {detail !== undefined && detail.exes.length > 0 ? (
                <ul className={styles.list}>
                  {detail.exes.map((exe) => (
                    <li key={exe} className={styles.value}>
                      {exe}
                    </li>
                  ))}
                </ul>
              ) : (
                <span className={styles.hint}>{t('program.noExes')}</span>
              )}
            </div>
          </section>
        )}

        <section className={styles.block} aria-label={t('program.connections')}>
          <h3 className={styles.blockTitle}>{t('program.connections')}</h3>
          {connections.length > 0 ? (
            <div className={styles.tableWrap}>
              <table className={styles.table}>
                <thead>
                  <tr>
                    <th scope="col">{t('program.colHost')}</th>
                    <th scope="col">{t('program.colLane')}</th>
                    <th scope="col" className={styles.num}>
                      {t('program.colSpeed')}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {connections.map((c, i) => {
                    const speed = formatSpeed(c.down + c.up)
                    return (
                      <tr key={i}>
                        <td className={styles.host}>{c.host}</td>
                        <td data-lane={c.lane} className={styles.laneCell}>
                          {laneName(c.lane)}
                        </td>
                        <td className={styles.num} data-quiet={speed === ''}>
                          {speed !== '' ? speed : t('card.quiet')}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          ) : (
            <p className={styles.hint}>{t(live ? 'program.noConnections' : 'program.connectionsOff')}</p>
          )}
        </section>
      </div>
    </div>
  )
}
