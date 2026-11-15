import { useEffect, useRef, useState } from 'react'
import type { Command } from '../../shared/ipc'
import type { ExitNode, HelperState } from '../../shared/protocol'
import type { LastExitNode, WindowSettings } from '../../shared/settings'
import { Banner } from '../components/Banner'
import { useUpdateActions } from '../components/useUpdateActions'
import { LanguageSelect } from '../components/LanguageSelect'
import { UpdateBanner } from '../components/UpdateBanner'
import { ScreenFrame } from '../components/ScreenFrame'
import { Toggle } from '../components/Toggle'
import { currentLang, t } from '../i18n/t'
import { connectStep, exitNodeChoices, type ConnectStep } from '../model/connect'
import { updateBusy, updateStatusActions, updateStatusLine } from '../model/update'
import { sendCommand, useApp, useServices } from '../services'
import styles from './screens.module.css'

/** Online nodes first, then by name in the order of the current language. */
function sortedNodes(nodes: ExitNode[]): ExitNode[] {
  const lang = currentLang()
  return [...nodes].sort((a, b) => Number(b.online) - Number(a.online) || a.name.localeCompare(b.name, lang))
}

/** How long a choice of the exit node waits for the next one before it is sent. */
const CHOICE_SETTLE_MS = 500

/**
 * The exit nodes of the tailnet as one radio group; choosing one sends tailscale.setExitNode (the helper validates
 * it). The selected node is always in it, named as far as the window knows (the list is empty while routing is off).
 */
function ExitNodeList({ state, last }: { state: HelperState; last: LastExitNode | null }): React.JSX.Element {
  const services = useServices()
  const { selectedExitNode } = state.tailscale
  const nodes = exitNodeChoices(state, last)
  // arrow keys move through native radios and change the choice on each press; every change restarts split
  // routing while it is live, so only a settled choice (the last one, after a pause) is sent
  const [pending, setPending] = useState<string | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(
    () => () => {
      if (timer.current !== null) clearTimeout(timer.current)
    },
    []
  )
  const choose = (id: string): void => {
    setPending(id)
    if (timer.current !== null) clearTimeout(timer.current)
    timer.current = setTimeout(() => {
      timer.current = null
      setPending(null)
      void sendCommand(services, { method: 'tailscale.setExitNode', id })
    }, CHOICE_SETTLE_MS)
  }
  const shown = pending ?? selectedExitNode
  if (nodes.length === 0) return <p className={styles.lead}>{t('connect.noExitNodes')}</p>
  return (
    <div className={styles.nodes} role="radiogroup" aria-label={t('connect.exitNodes')}>
      {sortedNodes(nodes).map((n) => (
        <label key={n.id} className={styles.node} title={n.online ? undefined : t('connect.offlineWarn')}>
          <input
            type="radio"
            name="exit-node"
            className={styles.nodeRadio}
            checked={n.id === shown}
            onChange={() => choose(n.id)}
          />
          <span className={styles.nodeName}>{n.name || n.id}</span>
          {!n.online && <span className={styles.offline}>{t('connect.offline')}</span>}
        </label>
      ))}
    </div>
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
  const lastExitNode = useApp((s) => s.snapshot?.settings.lastExitNode ?? null)
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
      <UpdateBanner />
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
          <ExitNodeList state={state} last={lastExitNode} />
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
      <LanguageSelect />
    </ScreenFrame>
  )
}

const live = (phase: HelperState['phase']): boolean => phase !== 'off' && phase !== 'error'

/**
 * «Подключение» (spec 9.5, 9.6), opened from the connection pill: the device, the exit node and its change,
 * logout (only while routing is off) and the settings of the window, «Язык» among them.
 */
export function ConnectionScreen(): React.JSX.Element | null {
  const services = useServices()
  const { bridge, store } = services
  const state = useApp((s) => s.snapshot?.state ?? null)
  const settings = useApp((s) => s.snapshot?.settings ?? null)
  const link = useApp((s) => s.snapshot?.link ?? null)
  const update = useApp((s) => s.snapshot?.update ?? null)
  const [appVersion, setAppVersion] = useState<string | null>(null)
  const updates = useUpdateActions()
  const back = (): void => store.getState().showConnection(false)

  // Esc goes back to the lanes, unless a select or a list inside has the key (an open select closes on it)
  useEffect(() => {
    const onKey = (e: KeyboardEvent): void => {
      if (e.key !== 'Escape' || e.defaultPrevented) return
      if (e.target instanceof Element && e.target.closest('select, [role="listbox"]') !== null) return
      store.getState().showConnection(false)
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [store])

  useEffect(() => {
    let live = true
    bridge.appInfo().then(
      (i) => {
        if (live) setAppVersion(i.version)
      },
      () => {}
    )
    return () => {
      live = false
    }
  }, [bridge])
  if (state === null || settings === null) return null
  const versions =
    appVersion === null
      ? null
      : link?.kind === 'ready'
        ? t('connect.versions', { app: appVersion, helper: link.hello.helper, core: link.hello.core })
        : t('connect.versionApp', { app: appVersion })

  const save = (patch: Partial<WindowSettings>): void => {
    bridge.setSettings(patch).catch(() => {
      store.getState().showNotice(t('connection.saveFailed'))
    })
  }
  const canLogout = !live(state.phase)
  const updateLine = update === null ? null : updateStatusLine(update)
  const updateActions = update === null ? [] : updateStatusActions(update)

  return (
    <ScreenFrame title={t('connection.title')} wide onBack={back}>
      <Banner />
      <div className={styles.row}>
        <span className={styles.label}>{t('connection.device')}</span>
        <span className={styles.value}>{state.tailscale.deviceName}</span>
      </div>
      <div className={styles.section}>
        <ExitNodeList state={state} last={settings.lastExitNode} />
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
          on={settings.checkUpdates}
          label={t('connection.checkUpdates')}
          caption={t('connection.checkUpdates')}
          onToggle={() => save({ checkUpdates: !settings.checkUpdates })}
        />
        <Toggle
          on={settings.notifications}
          label={t('connection.notifications')}
          caption={t('connection.notifications')}
          onToggle={() => save({ notifications: !settings.notifications })}
        />
        <Toggle
          on={settings.enableOnStart}
          label={t('connection.enableOnStart')}
          caption={t('connection.enableOnStart')}
          onToggle={() => save({ enableOnStart: !settings.enableOnStart })}
        />
        <LanguageSelect />
      </div>
      <div className={styles.actions}>
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
      {versions !== null && <p className={styles.lead}>{versions}</p>}
      {updateLine !== null && (
        <div className={styles.updateLine} role="status">
          <span>{updateLine}</span>
          {updateActions.map((action, i) => (
            <button
              key={action}
              type="button"
              className={`${styles.button} ${i === 0 ? styles.primary : ''}`}
              disabled={updates.busy}
              onClick={() => void updates.run(action)}
            >
              {t(`update.action.${action}`)}
            </button>
          ))}
        </div>
      )}
      <div className={styles.actions}>
        <button
          type="button"
          className={styles.button}
          disabled={update !== null && updateBusy(update)}
          onClick={() => {
            bridge.checkUpdates().catch(() => {})
          }}
        >
          {t('update.check')}
        </button>
      </div>
    </ScreenFrame>
  )
}
