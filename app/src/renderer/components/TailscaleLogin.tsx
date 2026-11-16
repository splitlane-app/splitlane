import { useEffect, useState } from 'react'
import type { HelperState } from '../../shared/protocol'
import { t } from '../i18n/t'
import { routingLive, type LoginStep } from '../model/connect'
import styles from '../screens/screens.module.css'
import { sendCommand, useApp, useServices, type Services } from '../services'

/** Opens the login link in the browser; a failure is told by a notice. */
function openLogin({ bridge, store }: Services, url: string): void {
  const failed = (): void => store.getState().showNotice(t('connect.openFailed'))
  bridge
    .openLogin(url)
    .then((ok) => {
      if (!ok) failed()
    })
    .catch(failed)
}

/**
 * The login to Tailscale (spec 9.5, 7), the same on «Подключите VPN» and on «Подключение»: «Войти в Tailscale», then
 * the wait for the browser with «Открыть ссылку снова», then the wait for the approval of the node. The link of a login
 * this page sent goes to the browser by itself, once; any other link (the one the helper keeps after a logout) waits
 * for «Открыть ссылку снова». While routing is not off the helper does not start the login (the setup core never runs
 * beside the main core: tailscale.login is wrong_phase while live, busy while starting or stopping), so a note says when
 * it can instead of the button: for a user of the proxy list while routing runs on a server, and for a Tailscale user
 * whose routing is stopping with the login gone.
 */
export function TailscaleLogin({ state, step }: { state: HelperState; step: LoginStep }): React.JSX.Element {
  const services = useServices()
  const loginRequested = useApp((s) => s.loginRequested)
  const [busy, setBusy] = useState(false)
  const authUrl = state.tailscale.authUrl

  // the request is taken from the store, so a second run of the effect does not open the link twice
  useEffect(() => {
    if (authUrl !== null && loginRequested && services.store.getState().takeLoginRequest()) openLogin(services, authUrl)
  }, [authUrl, loginRequested, services])

  const login = async (): Promise<void> => {
    setBusy(true)
    try {
      await sendCommand(services, { method: 'tailscale.login' })
    } finally {
      setBusy(false)
    }
  }

  if (step === 'login') {
    if (routingLive(state.phase)) {
      return (
        <p className={styles.lead} role="note">
          {t('connect.loginBlocked')}
        </p>
      )
    }
    return (
      <div className={styles.section}>
        <p className={styles.lead}>{t('connect.loginHint')}</p>
        <button type="button" className={`${styles.button} ${styles.primary}`} disabled={busy} onClick={() => void login()}>
          {t('connect.loginButton')}
        </button>
      </div>
    )
  }
  if (step === 'waitLogin' && authUrl !== null) {
    return (
      <div className={styles.section}>
        <p className={styles.status} role="status">
          {t('connect.waitLogin')}
        </p>
        <button type="button" className={styles.button} onClick={() => openLogin(services, authUrl)}>
          {t('connect.openAgain')}
        </button>
      </div>
    )
  }
  return (
    <p className={styles.status} role="status">
      {step === 'approval' ? t('connect.approval') : t('connect.waitLogin')}
    </p>
  )
}
