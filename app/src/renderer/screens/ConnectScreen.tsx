import { useEffect, useRef, useState } from 'react'
import type { Command } from '../../shared/ipc'
import type { ExitNode, HelperState } from '../../shared/protocol'
import type { WindowSettings } from '../../shared/settings'
import { Banner } from '../components/Banner'
import { ScreenFrame } from '../components/ScreenFrame'
import { Toggle } from '../components/Toggle'
import { t } from '../i18n/t'
import { selectedExitNodeName } from '../model/banners'
import { connectStep, type ConnectStep } from '../model/connect'
import { sendCommand, useApp, useServices } from '../services'
import styles from './screens.module.css'

/** Online nodes first, then by name. */
function sortedNodes(nodes: ExitNode[]): ExitNode[] {
  return [...nodes].sort((a, b) => Number(b.online) - Number(a.online) || a.name.localeCompare(b.name))
}

/** The exit nodes of the tailnet as a list; choosing one sends tailscale.setExitNode (the helper validates it). */
function ExitNodeList({ state }: { state: HelperState }): React.JSX.Element {
  const services = useServices()
  const { exitNodes, selectedExitNode } = state.tailscale
  if (exitNodes.length === 0) return <p className={styles.lead}>{t('connect.noExitNodes')}</p>
  return (
    <ul className={styles.nodes} aria-label={t('connect.exitNodes')}>
      {sortedNodes(exitNodes).map((n) => (
        <li key={n.id}>
          <button
            type="button"
            className={styles.node}
            aria-pressed={n.id === selectedExitNode}
            title={n.online ? undefined : t('connect.offlineWarn')}
            onClick={() => void sendCommand(services, { method: 'tailscale.setExitNode', id: n.id })}
          >
            <span>{n.name || n.id}</span>
            {!n.online && <span className={styles.offline}>{t('connect.offline')}</span>}
          </button>
        </li>
      ))}
    </ul>
  )
}

const STEP_OF: Record<ConnectStep, number> = { login: 0, waitLogin: 0, approval: 0, exitNode: 1, enable: 2 }
const STEP_KEYS = ['connect.step.login', 'connect.step.exitNode', 'connect.step.enable'] as const

/**
 * «Подключите VPN» (spec 9.5): the tiles of the connection methods and the three steps of Tailscale: log in, choose
 * the exit node, turn splitting on. Once routing is on the first time it records `connectedOnce`.
 */
export function ConnectScreen(): React.JSX.Element | null {
  const services = useServices()
  const { bridge, store } = services
  const state = useApp((s) => s.snapshot?.state ?? null)
  const connectedOnce = useApp((s) => s.snapshot?.settings.connectedOnce ?? false)
  const loginRequested = useApp((s) => s.loginRequested)
  const [busy, setBusy] = useState(false)
  const recorded = useRef(false)
  const authUrl = state?.tailscale.authUrl ?? null
  const phase = state?.phase ?? null

  const openLogin = (url: string): void => {
    const failed = (): void => store.getState().showNotice(t('connect.openFailed'))
    bridge
      .openLogin(url)
      .then((ok) => {
        if (!ok) failed()
      })
      .catch(failed)
  }

  // the link of a login this page sent goes to the browser by itself, once; any other link (the one the helper
  // keeps after a logout) waits for «Открыть ссылку снова». The request is taken from the store, so a second run
  // of the effect does not open it twice.
  useEffect(() => {
    if (authUrl === null || !loginRequested || !store.getState().takeLoginRequest()) return
    const failed = (): void => store.getState().showNotice(t('connect.openFailed'))
    bridge
      .openLogin(authUrl)
      .then((ok) => {
        if (!ok) failed()
      })
      .catch(failed)
  }, [authUrl, loginRequested, bridge, store])

  // the first time routing is on, the screen has done its job
  useEffect(() => {
    if (phase !== 'on' || connectedOnce || recorded.current) return
    recorded.current = true
    bridge.setSettings({ connectedOnce: true }).catch(() => {
      recorded.current = false
    })
  }, [phase, connectedOnce, bridge])

  if (state === null) return null
  const step = connectStep(state)
  const current = STEP_OF[step]

  const run = async (c: Command): Promise<void> => {
    setBusy(true)
    try {
      await sendCommand(services, c)
    } finally {
      setBusy(false)
    }
  }

  return (
    <ScreenFrame title={t('connect.title')} wide>
      <Banner withoutOff />
      <p className={styles.lead}>{t('connect.lead')}</p>
      <ul className={styles.tiles} aria-label={t('connect.methods')}>
        <li className={styles.tile} data-active="true">
          <strong>{t('connect.tailscale')}</strong>
          <span className={styles.tileNote}>{t('connect.tailscaleNote')}</span>
        </li>
        <li className={styles.tile} data-active="false" aria-disabled="true">
          <strong>{t('connect.key')}</strong>
          <span className={styles.tileNote}>{t('connect.soon')}</span>
        </li>
        <li className={styles.tile} data-active="false" aria-disabled="true">
          <strong>{t('connect.file')}</strong>
          <span className={styles.tileNote}>{t('connect.soon')}</span>
        </li>
      </ul>
      <ol className={styles.steps} aria-label={t('connect.steps')}>
        {STEP_KEYS.map((key, i) => (
          <li
            key={key}
            className={styles.step}
            data-state={i < current ? 'done' : i === current ? 'current' : 'todo'}
            aria-current={i === current ? 'step' : undefined}
          >
            <span className={styles.stepNum}>{i + 1}</span>
            {t(key)}
          </li>
        ))}
      </ol>

      {step === 'login' && (
        <div className={styles.section}>
          <p className={styles.lead}>{t('connect.loginHint')}</p>
          <button
            type="button"
            className={`${styles.button} ${styles.primary}`}
            disabled={busy}
            onClick={() => void run({ method: 'tailscale.login' })}
          >
            {t('connect.loginButton')}
          </button>
        </div>
      )}
      {step === 'waitLogin' && authUrl !== null && (
        <div className={styles.section}>
          <p className={styles.status} role="status">
            {t('connect.waitLogin')}
          </p>
          <button type="button" className={styles.button} onClick={() => openLogin(authUrl)}>
            {t('connect.openAgain')}
          </button>
        </div>
      )}
      {step === 'approval' && (
        <p className={styles.status} role="status">
          {t('connect.approval')}
        </p>
      )}
      {(step === 'exitNode' || step === 'enable') && (
        <div className={styles.section}>
          <p className={styles.lead}>{t('connect.exitNodeHint')}</p>
          <ExitNodeList state={state} />
        </div>
      )}
      {step === 'enable' && (
        <div className={styles.section}>
          {state.systemTailscale.exitNodeOn && (
            <p className={styles.warning} role="note">
              {t('connect.systemWarn')}
            </p>
          )}
          <button
            type="button"
            className={`${styles.button} ${styles.primary}`}
            disabled={busy || phase === 'starting'}
            onClick={() => void run({ method: 'routing.enable' })}
          >
            {phase === 'starting' ? t('connect.enabling') : t('connect.enable')}
          </button>
        </div>
      )}
    </ScreenFrame>
  )
}

const live = (phase: HelperState['phase']): boolean => phase !== 'off' && phase !== 'error'

/**
 * «Подключение» (spec 9.5, 9.6), opened from the connection pill: the device, the exit node and its change,
 * logout (only while routing is off) and the two settings of the window.
 */
export function ConnectionScreen(): React.JSX.Element | null {
  const services = useServices()
  const { bridge, store } = services
  const state = useApp((s) => s.snapshot?.state ?? null)
  const settings = useApp((s) => s.snapshot?.settings ?? null)
  if (state === null || settings === null) return null

  const save = (patch: Partial<WindowSettings>): void => {
    bridge.setSettings(patch).catch(() => {
      store.getState().showNotice(t('connection.saveFailed'))
    })
  }
  const canLogout = !live(state.phase)

  return (
    <ScreenFrame title={t('connection.title')} wide>
      <Banner />
      <div className={styles.row}>
        <span className={styles.label}>{t('connection.device')}</span>
        <span className={styles.value}>{state.tailscale.deviceName}</span>
      </div>
      <div className={styles.section}>
        <div className={styles.row}>
          <span className={styles.label}>{t('connection.exitNode')}</span>
          <span className={styles.value}>{selectedExitNodeName(state) ?? t('pill.noExitNode')}</span>
        </div>
        <ExitNodeList state={state} />
        {live(state.phase) && <p className={styles.lead}>{t('connection.changeNote')}</p>}
      </div>
      <div className={styles.section} role="group" aria-label={t('connection.settings')}>
        <Toggle
          on={settings.openAtLogin}
          label={t('connection.openAtLogin')}
          caption={t('connection.openAtLogin')}
          onToggle={() => save({ openAtLogin: !settings.openAtLogin })}
        />
        <Toggle
          on={settings.enableOnStart}
          label={t('connection.enableOnStart')}
          caption={t('connection.enableOnStart')}
          onToggle={() => save({ enableOnStart: !settings.enableOnStart })}
        />
      </div>
      <div className={styles.actions}>
        <button type="button" className={styles.button} onClick={() => store.getState().showConnection(false)}>
          {t('screen.back')}
        </button>
        <button
          type="button"
          className={styles.button}
          disabled={!canLogout}
          title={canLogout ? undefined : t('connection.logoutBlocked')}
          onClick={() => void sendCommand(services, { method: 'tailscale.logout' })}
        >
          {t('connection.logout')}
        </button>
      </div>
      {!canLogout && <p className={styles.lead}>{t('connection.logoutBlocked')}</p>}
    </ScreenFrame>
  )
}
