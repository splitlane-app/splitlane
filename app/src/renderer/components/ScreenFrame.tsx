import type { ReactNode } from 'react'
import { t } from '../i18n/t'
import { Notice } from './Notice'
import styles from './ScreenFrame.module.css'

/** A full-window screen with the product name and a heading: the splash and the setup and connect screens. */
export function ScreenFrame({
  title,
  wide = false,
  onBack,
  children
}: {
  title: string
  wide?: boolean
  /** A back control before the title («← Назад к дорожкам»). */
  onBack?: () => void
  children?: ReactNode
}): React.JSX.Element {
  return (
    <div className={styles.screen}>
      <main className={styles.panel} data-wide={wide}>
        {onBack !== undefined && (
          <button type="button" className={styles.back} onClick={onBack}>
            <span aria-hidden="true">← </span>
            {t('screen.back')}
          </button>
        )}
        <span className={styles.name}>{t('app.name')}</span>
        <h1 className={styles.title}>{title}</h1>
        {children}
      </main>
      <Notice />
    </div>
  )
}
