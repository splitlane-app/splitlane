import styles from './Toggle.module.css'

export interface ToggleProps {
  on: boolean
  disabled?: boolean
  /** The accessible name of the switch. */
  label: string
  /** The visible caption beside the switch. */
  caption: string
  /** Why it is disabled, shown on hover. */
  title?: string
  onToggle(): void
}

/** A switch with a caption: a button of role switch, so it is reachable and pressed like any button. */
export function Toggle({ on, disabled = false, label, caption, title, onToggle }: ToggleProps): React.JSX.Element {
  return (
    <span className={styles.toggle} data-on={on}>
      <button
        type="button"
        role="switch"
        aria-checked={on}
        aria-label={label}
        disabled={disabled}
        title={title}
        className={styles.knob}
        onClick={onToggle}
      />
      <span className={styles.caption} aria-hidden="true">
        {caption}
      </span>
    </span>
  )
}
