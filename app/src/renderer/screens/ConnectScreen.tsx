import { useEffect, useRef, useState } from 'react'
import type { Command } from '../../shared/ipc'
import type { HelperState } from '../../shared/protocol'
import type { WindowSettings } from '../../shared/settings'
import { Banner } from '../components/Banner'
import { ExitNodeList } from '../components/ExitNodeList'
import { LanguageSelect } from '../components/LanguageSelect'
import { MethodTiles } from '../components/MethodTiles'
import { ProxyPanel } from '../components/ProxyPanel'
import { ScreenFrame } from '../components/ScreenFrame'
import { TailscaleLogin } from '../components/TailscaleLogin'
import { Toggle } from '../components/Toggle'
import { UpdateBanner } from '../components/UpdateBanner'
import { useUpdateActions } from '../components/useUpdateActions'
import { VpsPanel } from '../components/VpsPanel'
import type { Key } from '../i18n/ru'
import { t } from '../i18n/t'
import {
  isLoginStep,
  methodOf,
  proxyStep,
  routingLive,
  tailscaleStep,
  type ConnectMethod,
  type ConnectStep,
  type ProxyStep
} from '../model/connect'
import { updateBusy, updateStatusActions, updateStatusLine } from '../model/update'
import { sendCommand, useApp, useServices, type Services } from '../services'
import styles from './screens.module.css'

const TAILSCALE_STEP: Record<ConnectStep, number> = {
  login: 0,
  waitLogin: 0,
  approval: 0,
  exitNode: 1,
  server: 1,
  enable: 2
}
const PROXY_STEP: Record<ProxyStep, number> = { add: 0, server: 1, enable: 2 }
const STEP_KEYS: Record<ConnectMethod, readonly Key[]> = {
  tailscale: ['connect.step.login', 'connect.step.exitNode', 'connect.step.enable'],
  key: ['connect.step.add', 'connect.step.server', 'connect.step.enable'],
  file: ['connect.step.file', 'connect.step.server', 'connect.step.enable'],
  vps: ['connect.step.vps', 'connect.step.vpsWait', 'connect.step.enable']
}

/**
 * The tile a screen opens on: the VPS one while a server setup runs (its progress is there), else the one of the
 * connector in force.
 */
function openingMethod({ store }: Services, state: HelperState | null): ConnectMethod {
  if (store.getState().vps?.outcome === null) return 'vps'
  return state === null ? 'tailscale' : methodOf(state)
}

/** The three steps of a method, the current one marked. */
function Steps({ method, current }: { method: ConnectMethod; current: number }): React.JSX.Element {
  return (
    <ol className={styles.steps} aria-label={t('connect.steps')}>
      {STEP_KEYS[method].map((key, i) => (
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
  )
}

/** The last step of either method: the warning about the exit node of the Tailscale app and the enable button. */
function EnableStep({ state, busy, onEnable }: { state: HelperState; busy: boolean; onEnable(): void }): React.JSX.Element {
  const starting = state.phase === 'starting'
  return (
    <div className={styles.section}>
      {state.systemTailscale.exitNodeOn && (
        <p className={styles.warning} role="note">
          {t('connect.systemWarn')}
        </p>
      )}
      <button type="button" className={`${styles.button} ${styles.primary}`} disabled={busy || starting} onClick={onEnable}>
        {starting ? t('connect.enabling') : t('connect.enable')}
      </button>
    </div>
  )
}

/**
 * «Подключите VPN» (spec 9.5, stage 3 spec 6): the tiles of the connection methods and the three steps of the chosen
 * one. Tailscale: log in, choose the exit node, turn splitting on. A key or a file: add one, choose a server, turn
 * splitting on. Own VPS (part 2 spec 6): enter the server, wait for the setup (it adds and selects the first server),
 * turn splitting on. The tile opens on the connector in force. Once routing is on the first time it records
 * `connectedOnce`.
 */
export function ConnectScreen(): React.JSX.Element | null {
  const services = useServices()
  const { bridge } = services
  const state = useApp((s) => s.snapshot?.state ?? null)
  const connectedOnce = useApp((s) => s.snapshot?.settings.connectedOnce ?? false)
  const lastExitNode = useApp((s) => s.snapshot?.settings.lastExitNode ?? null)
  const vps = useApp((s) => s.vps)
  const [busy, setBusy] = useState(false)
  const [method, setMethod] = useState<ConnectMethod>(() => openingMethod(services, state))
  const recorded = useRef(false)
  const phase = state?.phase ?? null

  // the first time routing is on, the screen has done its job
  useEffect(() => {
    if (phase !== 'on' || connectedOnce || recorded.current) return
    recorded.current = true
    bridge.setSettings({ connectedOnce: true }).catch(() => {
      recorded.current = false
    })
  }, [phase, connectedOnce, bridge])

  if (state === null) return null
  const step = tailscaleStep(state)
  const proxy = proxyStep(state)
  const vpsDone = vps?.outcome?.kind === 'done'
  const current =
    method === 'tailscale' ? TAILSCALE_STEP[step] : method === 'vps' ? (vps === null ? 0 : vpsDone ? 2 : 1) : PROXY_STEP[proxy]

  const run = async (c: Command): Promise<void> => {
    setBusy(true)
    try {
      await sendCommand(services, c)
    } finally {
      setBusy(false)
    }
  }
  const enable = <EnableStep state={state} busy={busy} onEnable={() => void run({ method: 'routing.enable' })} />

  return (
    <ScreenFrame title={t('connect.title')} wide>
      <Banner withoutOff />
      <UpdateBanner />
      <p className={styles.lead}>{t('connect.lead')}</p>
      <MethodTiles value={method} onChange={setMethod} />
      <Steps method={method} current={current} />

      {(method === 'key' || method === 'file') && (
        <>
          {/* a tile of its own: the pasted text does not follow to the file tile */}
          <ProxyPanel key={method} mode={method} />
          {proxy === 'enable' && enable}
        </>
      )}
      {method === 'vps' && (
        <>
          <VpsPanel />
          {/* the setup selected its first server: what is left is the enable */}
          {vpsDone && proxy === 'enable' && enable}
        </>
      )}
      {method === 'tailscale' && isLoginStep(step) && <TailscaleLogin state={state} step={step} />}
      {method === 'tailscale' && (step === 'exitNode' || step === 'enable') && (
        <div className={styles.section}>
          <p className={styles.lead}>{t('connect.exitNodeHint')}</p>
          <ExitNodeList state={state} last={lastExitNode} />
        </div>
      )}
      {method === 'tailscale' && step === 'enable' && enable}
      <LanguageSelect />
    </ScreenFrame>
  )
}

/**
 * «Подключение» (spec 9.5, 9.6, stage 3 spec 6), opened from the connection pill: the tiles of the methods, opened on
 * the connector in force (on own VPS while its setup runs), and the part of the chosen one. Tailscale: the device, the
 * exit node and its change, logout (only while routing is off); before the login (a user of the proxy list), the login
 * as on «Подключите VPN». A key or a file: the add, the servers with their choice and removal, the subscriptions with
 * their refresh and removal. Own VPS: the setup, then the list of servers. Switching the connector is choosing a
 * server or an exit node. Then the settings of the window, «Язык» among them.
 */
export function ConnectionScreen(): React.JSX.Element | null {
  const services = useServices()
  const { bridge, store } = services
  const state = useApp((s) => s.snapshot?.state ?? null)
  const settings = useApp((s) => s.snapshot?.settings ?? null)
  const link = useApp((s) => s.snapshot?.link ?? null)
  const update = useApp((s) => s.snapshot?.update ?? null)
  const [appVersion, setAppVersion] = useState<string | null>(null)
  const [method, setMethod] = useState<ConnectMethod>(() => openingMethod(services, state))
  const updates = useUpdateActions()
  const back = (): void => store.getState().showConnection(false)

  // Esc goes back to the lanes, unless a select or a list inside has the key (an open select closes on it) or a text
  // is being typed, the text of a key or a field of the VPS form (going back would drop it)
  useEffect(() => {
    const onKey = (e: KeyboardEvent): void => {
      if (e.key !== 'Escape' || e.defaultPrevented) return
      const typing = 'select, textarea, input[type="text"], input[type="password"], [role="listbox"]'
      if (e.target instanceof Element && e.target.closest(typing) !== null) return
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
  const live = routingLive(state.phase)
  const step = tailscaleStep(state)
  const updateLine = update === null ? null : updateStatusLine(update)
  const updateActions = update === null ? [] : updateStatusActions(update)

  return (
    <ScreenFrame title={t('connection.title')} wide onBack={back}>
      <Banner />
      <MethodTiles value={method} onChange={setMethod} compact />
      {method === 'tailscale' && isLoginStep(step) ? (
        // a user of the proxy list who has not logged in to Tailscale yet logs in here
        <TailscaleLogin state={state} step={step} />
      ) : method === 'tailscale' ? (
        <>
          <div className={styles.row}>
            <span className={styles.label}>{t('connection.device')}</span>
            <span className={styles.value}>{state.tailscale.deviceName}</span>
          </div>
          <div className={styles.section}>
            <ExitNodeList state={state} last={settings.lastExitNode} />
            {live && <p className={styles.lead}>{t('connection.changeNote')}</p>}
          </div>
          <div className={styles.actions}>
            <button
              type="button"
              className={styles.button}
              disabled={live}
              title={live ? t('connection.logoutBlocked') : undefined}
              onClick={() => void sendCommand(services, { method: 'tailscale.logout' })}
            >
              {t('connection.logout')}
            </button>
          </div>
          {live && <p className={styles.lead}>{t('connection.logoutBlocked')}</p>}
        </>
      ) : method === 'vps' ? (
        <VpsPanel showList />
      ) : (
        <ProxyPanel key={method} mode={method} manage />
      )}
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
