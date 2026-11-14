import { useEffect } from 'react'
import { CloseIcon } from '../icons/builtin'
import { t } from '../i18n/t'
import { useApp, useServices } from '../services'
import styles from './Notice.module.css'

/** How long a notice stays before it goes by itself. */
const NOTICE_MS = 10_000

/** The notice of the store (a failed command, a rejected rule) as a small toast at the bottom of the window. */
export function Notice(): React.JSX.Element | null {
  const { store } = useServices()
  const notice = useApp((s) => s.notice)

  useEffect(() => {
    if (notice === null) return
    const timer = setTimeout(() => {
      if (store.getState().notice === notice) store.getState().dismissNotice()
    }, NOTICE_MS)
    return () => clearTimeout(timer)
  }, [notice, store])

  if (notice === null) return null
  return (
    <div className={styles.notice} role="status">
      <p className={styles.text}>{notice}</p>
      <button
        type="button"
        className={styles.close}
        aria-label={t('notice.close')}
        onClick={() => store.getState().dismissNotice()}
      >
        <CloseIcon />
      </button>
    </div>
  )
}
