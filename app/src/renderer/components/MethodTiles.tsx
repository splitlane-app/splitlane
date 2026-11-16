import { useId, useRef, type KeyboardEvent } from 'react'
import type { Key } from '../i18n/ru'
import { t } from '../i18n/t'
import type { ConnectMethod } from '../model/connect'
import styles from './MethodTiles.module.css'

const METHODS: readonly ConnectMethod[] = ['tailscale', 'key', 'file', 'vps']
const NAME: Record<ConnectMethod, Key> = {
  tailscale: 'connect.tailscale',
  key: 'connect.key',
  file: 'connect.file',
  vps: 'connect.vps'
}
const NOTE: Record<ConnectMethod, Key> = {
  tailscale: 'connect.tailscaleNote',
  key: 'connect.keyNote',
  file: 'connect.fileNote',
  vps: 'connect.vpsNote'
}

const STEP: Partial<Record<string, number>> = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }

/**
 * The connection methods (stage 3 spec 6, part 2 spec 6) as one radio group of tiles: the arrows move the choice and the focus along them,
 * Home and End go to the ends, Tab enters on the chosen one. Choosing a tile shows its steps; the connector changes
 * only when a server or an exit node is chosen. `compact`: the names alone («Подключение»).
 */
export function MethodTiles({
  value,
  onChange,
  compact = false
}: {
  value: ConnectMethod
  onChange(m: ConnectMethod): void
  compact?: boolean
}): React.JSX.Element {
  const id = useId()
  const refs = useRef<(HTMLButtonElement | null)[]>([])

  const onKey = (e: KeyboardEvent<HTMLButtonElement>): void => {
    const at = METHODS.indexOf(value)
    const step = STEP[e.key]
    const next =
      step !== undefined
        ? (at + step + METHODS.length) % METHODS.length
        : e.key === 'Home'
          ? 0
          : e.key === 'End'
            ? METHODS.length - 1
            : null
    if (next === null) return
    e.preventDefault()
    onChange(METHODS[next])
    refs.current[next]?.focus()
  }

  return (
    <div className={styles.tiles} role="radiogroup" aria-label={t('connect.methods')} data-compact={compact}>
      {METHODS.map((m, i) => (
        <button
          key={m}
          ref={(el) => {
            refs.current[i] = el
          }}
          type="button"
          role="radio"
          aria-checked={m === value}
          aria-labelledby={`${id}-${m}`}
          aria-describedby={compact ? undefined : `${id}-${m}-note`}
          tabIndex={m === value ? 0 : -1}
          className={styles.tile}
          onClick={() => onChange(m)}
          onKeyDown={onKey}
        >
          <strong id={`${id}-${m}`}>{t(NAME[m])}</strong>
          {!compact && (
            <span id={`${id}-${m}-note`} className={styles.note}>
              {t(NOTE[m])}
            </span>
          )}
        </button>
      ))}
    </div>
  )
}
