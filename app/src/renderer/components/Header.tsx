import type { Phase } from '../../shared/protocol'
import { SearchIcon, SettingsIcon } from '../icons/builtin'
import type { Key } from '../i18n/ru'
import { t } from '../i18n/t'
import { sendCommand, useApp, useServices } from '../services'
import { isLivePhase } from '../store/appStore'
import { ConnectionPill } from './ConnectionPill'
import styles from './Header.module.css'
import { Toggle } from './Toggle'

const CAPTION: Record<Phase, Key> = {
  off: 'header.off',
  error: 'header.off',
  starting: 'header.starting',
  on: 'header.on',
  degraded: 'header.on',
  blocked: 'header.on',
  stopping: 'header.stopping'
}

/**
 * The header of the board (spec 9.1): name, routing switch, connection pill, search, «+ Добавить» and the gear that
 * opens «Подключение» with the settings of the window (as the pill does).
 */
export function Header(): React.JSX.Element {
  const services = useServices()
  const state = useApp((s) => s.snapshot?.state ?? null)
  const lastExitNode = useApp((s) => s.snapshot?.settings.lastExitNode ?? null)
  const linkReady = useApp((s) => s.snapshot?.link.kind === 'ready')
  const search = useApp((s) => s.search)
  const setupRunning = useApp((s) => s.vps !== null && s.vps.outcome === null)
  const { setSearch, openAdd, showConnection } = services.store.getState()

  const phase = state?.phase ?? null
  const on = phase === 'starting' || isLivePhase(phase)
  // a tunnel through the server being set up would cut the setup («Свой VPS»): the enable waits for its end
  const waitSetup = setupRunning && (phase === 'off' || phase === 'error')
  const toggle = (): void => {
    if (phase === null) return
    void sendCommand(
      services,
      phase === 'off' || phase === 'error' ? { method: 'routing.enable' } : { method: 'routing.disable' }
    )
  }

  return (
    <header className={styles.header}>
      <span className={styles.name}>{t('app.name')}</span>
      {state !== null && (
        <>
          <Toggle
            on={on}
            disabled={!linkReady || phase === 'stopping' || waitSetup}
            title={waitSetup ? t('vps.enableBlocked') : undefined}
            label={t('header.routing')}
            caption={t(CAPTION[state.phase])}
            onToggle={toggle}
          />
          <ConnectionPill state={state} last={lastExitNode} onOpen={() => showConnection(true)} />
        </>
      )}
      <span className={styles.grow} />
      <label className={styles.search}>
        <SearchIcon className={styles.searchIcon} />
        <input
          type="search"
          value={search}
          placeholder={t('header.search')}
          aria-label={t('header.search')}
          spellCheck={false}
          onChange={(e) => setSearch(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Escape' && search !== '') {
              e.preventDefault()
              setSearch('')
            }
          }}
        />
      </label>
      <button type="button" className={styles.add} onClick={() => openAdd(true)}>
        {t('header.add')}
      </button>
      <button
        type="button"
        className={styles.settings}
        aria-label={t('header.settings')}
        title={t('header.settings')}
        onClick={() => showConnection(true)}
      >
        <SettingsIcon />
      </button>
    </header>
  )
}
