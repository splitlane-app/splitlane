import { useState } from 'react'
import type { Command } from '../../shared/ipc'
import type { Phase } from '../../shared/protocol'
import { t } from '../i18n/t'
import { bannerFor, type BannerAction } from '../model/banners'
import { saveDiagnostics, sendCommand, useApp, useServices, type Services } from '../services'
import styles from './Banner.module.css'

/**
 * The command of a banner action (5.1 table). «Повторить» depends on the phase: in degraded it is the extra
 * check of the link and in error a new start (both routing.enable); in stopping with restore_pending it retries
 * the restore (routing.disable). null: the action is not a helper command.
 */
export function actionCommand(action: BannerAction, phase: Phase | null): Command | null {
  switch (action) {
    case 'enable':
      return { method: 'routing.enable' }
    case 'cancel':
    case 'disable':
      return { method: 'routing.disable' }
    case 'skipRestore':
      return { method: 'routing.disable', skipRestore: true }
    case 'retry':
      return phase === 'stopping' ? { method: 'routing.disable' } : { method: 'routing.enable' }
    case 'allDirectOn':
      return { method: 'routing.allDirect', on: true }
    case 'allDirectOff':
      return { method: 'routing.allDirect', on: false }
    case 'loginAgain':
      return { method: 'tailscale.login' }
    case 'clearSystemExitNode':
      return { method: 'systemTailscale.clearExitNode' }
    case 'saveLog':
    case 'retryLink':
      return null
  }
}

async function perform(services: Services, action: BannerAction, phase: Phase | null): Promise<void> {
  if (action === 'saveLog') return saveDiagnostics(services)
  if (action === 'retryLink') {
    try {
      await services.bridge.retryLink()
    } catch {
      // the link reports its own status in the next snapshot
    }
    return
  }
  const c = actionCommand(action, phase)
  if (c !== null) await sendCommand(services, c)
}

/**
 * The state banner over the lanes and on the connect screens (spec 9.5); nothing when the state needs none.
 * withoutOff: no banner for the phase off (the connect screen has its own enable button).
 */
export function Banner({ withoutOff = false }: { withoutOff?: boolean }): React.JSX.Element | null {
  const services = useServices()
  const link = useApp((s) => s.snapshot?.link ?? null)
  const state = useApp((s) => s.snapshot?.state ?? null)
  const [busy, setBusy] = useState<BannerAction | null>(null)
  if (link === null) return null
  if (withoutOff && link.kind === 'ready' && state?.phase === 'off') return null
  const banner = bannerFor(link, state)
  if (banner === null) return null

  const run = async (action: BannerAction): Promise<void> => {
    setBusy(action)
    try {
      await perform(services, action, state?.phase ?? null)
    } finally {
      setBusy(null)
    }
  }

  return (
    <div className={styles.banner} data-tone={banner.tone} role={banner.tone === 'info' ? 'status' : 'alert'}>
      <div className={styles.body}>
        <p className={styles.title}>
          <span className={styles.dot} aria-hidden="true" />
          {banner.title}
        </p>
        {banner.detail !== undefined && <p className={styles.detail}>{banner.detail}</p>}
        {banner.command !== undefined && (
          <>
            <p className={styles.detail}>{t('banner.restoreHint')}</p>
            <code className={styles.command}>{banner.command}</code>
          </>
        )}
      </div>
      {banner.actions.length > 0 && (
        <div className={styles.actions}>
          {banner.actions.map((action, i) => (
            <button
              key={action}
              type="button"
              className={i === 0 ? styles.primary : styles.ghost}
              disabled={busy !== null}
              onClick={() => void run(action)}
            >
              {t(`action.${action}`)}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
