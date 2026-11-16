import { t } from '../i18n/t'
import { proxyErrorText, rejectionLine } from '../model/proxy'
import styles from './ProxyResult.module.css'
import type { ShownOutcome } from './useProxyOutcome'

/**
 * The answer of an add or a refresh (spec 6): how many servers came, how many were there already, each line refused
 * with its number and reason (never the line itself: it may hold a secret), or the error of a failed call.
 */
export function ProxyResult({ outcome }: { outcome: ShownOutcome }): React.JSX.Element {
  if (outcome.kind === 'failed') {
    return (
      <p className={styles.failed} role="alert">
        {proxyErrorText(outcome.error, outcome.request)}
      </p>
    )
  }
  const { added, existing, rejected, rejectedMore } = outcome.result
  return (
    <div className={styles.result} role="status">
      <p className={styles.line}>{t('proxy.added', { n: added.length })}</p>
      {existing > 0 && <p className={styles.line}>{t('proxy.existing', { n: existing })}</p>}
      {rejected.length > 0 && (
        <ul className={styles.rejected}>
          {rejected.map((r, i) => (
            <li key={i}>{rejectionLine(r)}</li>
          ))}
        </ul>
      )}
      {rejectedMore > 0 && <p className={styles.line}>{t('proxy.rejectedMore', { n: rejectedMore })}</p>}
    </div>
  )
}
