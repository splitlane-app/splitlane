import type { ReactNode } from 'react'
import { t } from '../i18n/t'
import { Notice } from './Notice'
import styles from './ScreenFrame.module.css'

/** A full-window screen with the product name and a heading: the splash and the setup and connect screens. */
export function ScreenFrame({
  title,
  wide = false,
  children
}: {
  title: string
  wide?: boolean
  children?: ReactNode
}): React.JSX.Element {
  return (
    <div className={styles.screen}>
      <main className={styles.panel} data-wide={wide}>
        <span className={styles.name}>{t('app.name')}</span>
        <h1 className={styles.title}>{title}</h1>
        {children}
      </main>
      <Notice />
    </div>
  )
}
