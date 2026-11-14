import { Component, type ReactNode } from 'react'
import { t } from '../i18n/t'
import screens from '../screens/screens.module.css'
import styles from './ScreenFrame.module.css'

interface Props {
  children: ReactNode
  /** Reloads the page; location.reload by default. */
  onReload?: () => void
}

/**
 * The last resort of the page: an error while rendering shows a short text and «Перезапустить окно» instead of an
 * empty window. Only the page restarts; the helper and splitting go on (spec 10). The screen of the fallback
 * needs nothing of the store: it is drawn without ScreenFrame (whose notice reads the store).
 */
export class ErrorBoundary extends Component<Props, { failed: boolean }> {
  state = { failed: false }

  static getDerivedStateFromError(): { failed: boolean } {
    return { failed: true }
  }

  render(): ReactNode {
    if (!this.state.failed) return this.props.children
    const reload = this.props.onReload ?? ((): void => window.location.reload())
    return (
      <div className={styles.screen}>
        <main className={styles.panel}>
          <span className={styles.name}>{t('app.name')}</span>
          <h1 className={styles.title}>{t('crash.title')}</h1>
          <p className={screens.lead}>{t('crash.lead')}</p>
          <div className={screens.actions}>
            <button type="button" className={`${screens.button} ${screens.primary}`} onClick={reload}>
              {t('crash.reload')}
            </button>
          </div>
        </main>
      </div>
    )
  }
}
