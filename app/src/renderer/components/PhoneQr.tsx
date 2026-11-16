import { useId, useMemo, useRef, useState } from 'react'
import qrcode from 'qrcode-generator'
import { t } from '../i18n/t'
import { useServices } from '../services'
import styles from './PhoneQr.module.css'
import { useDialog } from './useDialog'
import { useMounted } from './useMounted'

/** The white border round the code, in modules (the standard quiet zone), so it scans on a dark theme too. */
const QUIET = 4

/** The code as one SVG path: type 0 (the smallest that fits), correction level M, no network. */
function QrCode({ address }: { address: string }): React.JSX.Element {
  const { size, path } = useMemo(() => {
    const qr = qrcode(0, 'M')
    qr.addData(address)
    qr.make()
    const n = qr.getModuleCount()
    let d = ''
    for (let r = 0; r < n; r++) {
      for (let c = 0; c < n; c++) {
        if (qr.isDark(r, c)) d += `M${c + QUIET} ${r + QUIET}h1v1h-1z`
      }
    }
    return { size: n + 2 * QUIET, path: d }
  }, [address])
  return (
    <svg
      className={styles.qr}
      viewBox={`0 0 ${size} ${size}`}
      shapeRendering="crispEdges"
      role="img"
      aria-label={t('phone.qr')}
    >
      <rect width={size} height={size} fill="#fff" />
      <path d={path} fill="#000" />
    </svg>
  )
}

/** The modal with the code; the address is a prop of this component only and is never drawn as text. */
function PhoneDialog({ address, close }: { address: string; close: () => void }): React.JSX.Element {
  const ref = useRef<HTMLDivElement | null>(null)
  const titleId = useId()
  const dialog = useDialog(ref, close)
  return (
    <div className={styles.backdrop} onMouseDown={(e) => e.target === e.currentTarget && close()}>
      <div ref={ref} role="dialog" aria-modal="true" aria-labelledby={titleId} className={styles.dialog} {...dialog}>
        <h2 id={titleId} className={styles.title}>
          {t('phone.connect')}
        </h2>
        <QrCode address={address} />
        <p className={styles.caption}>{t('phone.caption')}</p>
        <button type="button" className={styles.close} onClick={close}>
          {t('program.close')}
        </button>
      </div>
    </div>
  )
}

/**
 * «Подключить телефон» of a subscription: asks the helper for its address and shows it as a QR code in a dialog.
 * The address lives in this state only while the dialog is open; an error is a short line by the button.
 */
export function ConnectPhone({ subscriptionId }: { subscriptionId: string }): React.JSX.Element {
  const { bridge } = useServices()
  const mounted = useMounted()
  const buttonRef = useRef<HTMLButtonElement | null>(null)
  const [address, setAddress] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const open = async (): Promise<void> => {
    setBusy(true)
    setError(null)
    let answer: Awaited<ReturnType<typeof bridge.proxyReveal>> | null
    try {
      answer = await bridge.proxyReveal(subscriptionId)
    } catch {
      answer = null
    }
    if (!mounted.current) return
    setBusy(false)
    if (answer !== null && 'address' in answer) setAddress(answer.address)
    else setError(answer?.error.code === 'unknown_server' ? t('phone.unknown') : t('phone.failed'))
  }
  const close = (): void => {
    setAddress(null)
    buttonRef.current?.focus()
  }

  return (
    <>
      <button ref={buttonRef} type="button" className={styles.button} disabled={busy} onClick={() => void open()}>
        {t('phone.connect')}
      </button>
      {error !== null && (
        <span className={styles.error} role="alert">
          {error}
        </span>
      )}
      {address !== null && <PhoneDialog address={address} close={close} />}
    </>
  )
}
