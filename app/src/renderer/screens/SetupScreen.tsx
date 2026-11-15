import { useEffect, useState } from 'react'
import { LanguageSelect } from '../components/LanguageSelect'
import { ScreenFrame } from '../components/ScreenFrame'
import { UpdateBanner } from '../components/UpdateBanner'
import { t } from '../i18n/t'
import { useApp, useServices } from '../services'
import styles from './screens.module.css'

/**
 * The first-run «Первая настройка» (spec 4.3) and, for a helper older than the window, the update (5.1): one button
 * that runs the install of the background service with the Windows prompt, the progress and the outcome.
 */
export function SetupScreen({ update }: { update: boolean }): React.JSX.Element {
  const { bridge } = useServices()
  const install = useApp((s) => s.snapshot?.install ?? { kind: 'idle' as const })
  const running = install.kind === 'running'
  const done = install.kind === 'done' ? install.outcome : null
  const finished = done?.kind === 'ok'
  // a packaged window outside the install folder cannot repair the helper; neither button nor hint until the main
  // process answers, and a failed answer counts as not installed
  const [installed, setInstalled] = useState<boolean | null>(null)
  useEffect(() => {
    let live = true
    bridge.appInfo().then(
      (i) => {
        if (live) setInstalled(i.installed)
      },
      () => {
        if (live) setInstalled(false)
      }
    )
    return () => {
      live = false
    }
  }, [bridge])

  const start = (): void => {
    bridge.installHelper().catch(() => {
      // the outcome arrives in the snapshot
    })
  }

  return (
    <ScreenFrame title={update ? t('setup.updateTitle') : t('setup.title')}>
      {/* after an install refusal the window may open here: the result of the update shows on this screen too */}
      <UpdateBanner />
      <p className={styles.lead}>{update ? t('setup.updateLead') : t('setup.lead')}</p>
      {running && (
        <p className={styles.status} role="status">
          {t('setup.running')}
        </p>
      )}
      {done?.kind === 'ok' && (
        <p className={styles.status} data-tone="ok" role="status">
          {t('setup.done')}
        </p>
      )}
      {done?.kind === 'cancelled' && (
        <p className={styles.status} data-tone="warn" role="status">
          {t('setup.cancelled')}
        </p>
      )}
      {done?.kind === 'refused' && <pre className={styles.mono}>{done.reason}</pre>}
      {done?.kind === 'failed' && (
        <>
          <p className={styles.status} data-tone="warn" role="alert">
            {t('setup.failed')}
          </p>
          <pre className={styles.mono}>{done.detail}</pre>
        </>
      )}
      {installed === false && <p className={styles.lead}>{t('setup.notInstalled')}</p>}
      {installed === true && !running && !finished && (
        <div className={styles.actions}>
          <button type="button" className={`${styles.button} ${styles.primary}`} onClick={start}>
            {update ? t('setup.update') : t('setup.install')}
          </button>
        </div>
      )}
      <LanguageSelect />
    </ScreenFrame>
  )
}
