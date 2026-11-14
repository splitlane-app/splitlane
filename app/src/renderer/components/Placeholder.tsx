import type { ReactNode } from 'react'
import { t } from '../i18n/t'
import styles from './Placeholder.module.css'

/** A screen with the product name and a heading: the splash, and the screens task 13 fills in. */
export function Placeholder({ title, children }: { title: string; children?: ReactNode }): React.JSX.Element {
  return (
    <div className={styles.screen}>
      <div className={styles.panel}>
        <span className={styles.name}>{t('app.name')}</span>
        <h1 className={styles.title}>{title}</h1>
        {children}
      </div>
    </div>
  )
}
