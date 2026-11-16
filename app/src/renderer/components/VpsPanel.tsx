import { useEffect, useId, useRef, useState, type Ref } from 'react'
import type { VpsOutcome, VpsRequest } from '../../shared/ipc'
import type { Key } from '../i18n/ru'
import { t } from '../i18n/t'
import { routingLive } from '../model/connect'
import {
  EMPTY_FORM,
  formErrors,
  newRun,
  retryFocus,
  runEnd,
  runEvent,
  runMoved,
  vpsErrorText,
  vpsNotice,
  vpsRequest,
  warningTexts,
  type VpsField,
  type VpsForm
} from '../model/vps'
import { useApp, useServices, type Services } from '../services'
import { ServerList } from './ServerList'
import { focusLost, useMounted } from './useMounted'
import { VpsProgress } from './VpsProgress'
import styles from './VpsPanel.module.css'

/** The fields in the order of the form: the first one with an error takes the focus. */
const FIELD_ORDER: readonly VpsField[] = ['address', 'user', 'password', 'key', 'passphrase']

type Target = VpsField | 'submit' | 'cancel' | 'retry' | 'result'

/**
 * One setup: its run goes into the store (it outlives the panel, which may go and come back meanwhile), its progress
 * comes from onVpsEvent while it runs, and an end that no panel shows is told by a notice. The request carries the
 * secrets: it goes to the main process at once, and only the way in (`useKey`) is kept for the wait (this function is
 * not async, so nothing holds the request while the setup runs).
 */
export function runVpsSetup(services: Services, req: VpsRequest): Promise<VpsOutcome> {
  const { bridge, store } = services
  const { useKey } = req
  store.getState().setVps(() => newRun(Date.now(), useKey))
  const off = bridge.onVpsEvent((e) => store.getState().setVps((r) => (r === null ? r : runEvent(r, e))))
  let call: Promise<VpsOutcome>
  try {
    call = bridge.vpsSetup(req)
  } catch (e) {
    call = Promise.reject(e)
  }
  return followVpsSetup(services, call, useKey, off)
}

async function followVpsSetup({ store }: Services, call: Promise<VpsOutcome>, useKey: boolean, off: () => void): Promise<VpsOutcome> {
  let outcome: VpsOutcome
  try {
    outcome = await call
  } catch {
    outcome = { kind: 'failed', code: 'kit_failed' }
  } finally {
    off()
  }
  store.getState().setVps((r) => (r === null ? r : runEnd(r, outcome, Date.now())))
  if (store.getState().vpsViews === 0) {
    store.getState().showNotice(vpsNotice(outcome, useKey))
    store.getState().setVps(() => null)
  }
  return outcome
}

/** A labelled text or password field with its hint and its error under it. */
function TextField({
  label,
  value,
  onChange,
  hint,
  error,
  secret = false,
  inputRef
}: {
  label: string
  value: string
  onChange(value: string): void
  hint?: string
  error?: Key
  secret?: boolean
  inputRef: Ref<HTMLInputElement>
}): React.JSX.Element {
  const id = useId()
  const described = [hint === undefined ? null : `${id}-hint`, error === undefined ? null : `${id}-error`].filter((x) => x !== null)
  return (
    <div className={styles.field}>
      <label htmlFor={id} className={styles.label}>
        {label}
      </label>
      <input
        ref={inputRef}
        id={id}
        className={styles.input}
        type={secret ? 'password' : 'text'}
        value={value}
        spellCheck={false}
        autoComplete="off"
        autoCapitalize="off"
        aria-invalid={error !== undefined}
        aria-describedby={described.length > 0 ? described.join(' ') : undefined}
        onChange={(e) => onChange(e.target.value)}
      />
      {hint !== undefined && (
        <p id={`${id}-hint`} className={styles.hint}>
          {hint}
        </p>
      )}
      {error !== undefined && (
        <p id={`${id}-error`} className={styles.error}>
          {t(error)}
        </p>
      )}
    </div>
  )
}

/**
 * «Свой VPS» (stage 3 part 2 spec 6): the form (the address, the login, a password or a key file), then the progress
 * of the setup with «Отменить», then its result or its error with «Попробовать снова», which brings the form back
 * as it was. «Настроить» waits for split routing to be off. The password and the passphrase live in this component's
 * state only, never in the store or the settings; leaving the screen drops them with the component and makes the main
 * process forget the key (spec 3, ruling 18), and so does a hidden window (closed to the tray or minimized, ruling 21),
 * while a setup that runs goes on. `showList` («Подключение»): the list of servers under the result.
 */
export function VpsPanel({ showList = false }: { showList?: boolean }): React.JSX.Element | null {
  const services = useServices()
  const { bridge, store } = services
  const state = useApp((s) => s.snapshot?.state ?? null)
  const run = useApp((s) => s.vps)
  const mounted = useMounted()
  const [form, setForm] = useState<VpsForm>(EMPTY_FORM)
  // the errors show after the first «Настроить», then follow the typing
  const [checked, setChecked] = useState(false)
  const [keyError, setKeyError] = useState<Key | null>(null)
  const [picking, setPicking] = useState(false)
  const [cancelling, setCancelling] = useState(false)
  const targets = useRef<Partial<Record<Target, HTMLElement | null>>>({})
  const focusDue = useRef<Target | null>(null)
  const wayId = useId()
  const keyId = useId()
  const outcome = run?.outcome ?? null

  // on screen it is counted, so that an end no panel shows is a notice; leaving drops an end already shown and
  // makes the main process forget the key
  useEffect(() => {
    store.getState().viewVps(1)
    return () => {
      store.getState().viewVps(-1)
      if (store.getState().vpsViews === 0 && store.getState().vps?.outcome != null) store.getState().setVps(() => null)
      bridge.vpsForgetKey().catch(() => {})
    }
  }, [bridge, store])

  // a hidden window counts as leaving for the secrets (ruling 21): the form drops them, the main process the key; the
  // address and the login stay, and so does a setup that runs (its request went to the main process already)
  useEffect(() => {
    const onVisibility = (): void => {
      if (!document.hidden) return
      setForm((f) => (f.password === '' && f.passphrase === '' && f.keyName === null ? f : { ...f, password: '', passphrase: '', keyName: null }))
      setKeyError(null)
      bridge.vpsForgetKey().catch(() => {})
    }
    document.addEventListener('visibilitychange', onVisibility)
    return () => document.removeEventListener('visibilitychange', onVisibility)
  }, [bridge])

  // a control asked for the focus once it is on screen (the one asked for, or «Настроить» when it is not there)
  useEffect(() => {
    const target = focusDue.current
    if (target === null) return
    const el = targets.current[target] ?? targets.current.submit
    if (el == null) return
    focusDue.current = null
    el.focus()
  })

  // the end took «Отменить» away: the result, or «Попробовать снова»
  useEffect(() => {
    if (outcome === null || !focusLost()) return
    targets.current[outcome.kind === 'done' ? 'result' : 'retry']?.focus()
  }, [outcome])

  if (state === null) return null
  const live = routingLive(state.phase)
  const at =
    (name: Target) =>
    (el: HTMLElement | null): void => {
      targets.current[name] = el
    }
  const change = (patch: Partial<VpsForm>): void => setForm((f) => ({ ...f, ...patch }))

  const submit = (): void => {
    if (live || store.getState().vps !== null) return
    setChecked(true)
    const errors = formErrors(form)
    const first = FIELD_ORDER.find((f) => errors[f] !== undefined)
    if (first !== undefined) {
      targets.current[first]?.focus()
      return
    }
    setCancelling(false)
    focusDue.current = 'cancel'
    void runVpsSetup(services, vpsRequest(form)).then((o) => {
      // a done setup made the main process forget the key; the secrets are of no use any more
      if (o.kind === 'done' && mounted.current) setForm((f) => ({ ...f, password: '', passphrase: '', keyName: null }))
    })
  }

  const pick = async (): Promise<void> => {
    setPicking(true)
    let r: Awaited<ReturnType<typeof bridge.vpsPickKey>>
    try {
      r = await bridge.vpsPickKey()
    } catch {
      r = { error: 'not_text' }
    }
    if (!mounted.current) {
      // the screen went while the dialog was open: the key it took must not stay in the main process (ruling 22)
      if (r !== null && !('error' in r)) bridge.vpsForgetKey().catch(() => {})
      return
    }
    setPicking(false)
    if (r === null) return // the dialog was closed: the key in force, if any, stays
    if ('error' in r) {
      setKeyError(r.error === 'too_large' ? 'vps.check.keyTooLarge' : 'vps.check.keyNotText')
      return
    }
    setKeyError(null)
    change({ keyName: r.name })
  }

  const retry = (): void => {
    focusDue.current = (outcome !== null && run !== null ? retryFocus(outcome, run.useKey) : null) ?? 'submit'
    store.getState().setVps(() => null)
  }

  if (run !== null) {
    return (
      <div className={styles.panel}>
        {(outcome === null || runMoved(run)) && <VpsProgress run={run} />}
        {outcome === null && (
          <div className={styles.actions}>
            <button
              ref={at('cancel')}
              type="button"
              className={styles.button}
              disabled={cancelling}
              onClick={() => {
                setCancelling(true)
                bridge.vpsCancel().catch(() => {})
              }}
            >
              {cancelling ? t('vps.cancelling') : t('vps.cancel')}
            </button>
          </div>
        )}
        {outcome?.kind === 'done' && (
          <>
            <div ref={at('result')} className={styles.result} role="status" tabIndex={-1}>
              <p className={styles.done}>{t('vps.done', { n: outcome.servers })}</p>
              {warningTexts(outcome.warnings).map((w) => (
                <p key={w} className={styles.warning}>
                  {w}
                </p>
              ))}
            </div>
            {/* «Подключить телефон» for outcome.subscriptionId (task 8) goes here, under the result */}
            {showList && <ServerList state={state} />}
          </>
        )}
        {outcome !== null && outcome.kind !== 'done' && (
          <>
            {outcome.kind === 'cancelled' ? (
              <p className={styles.status} role="status">
                {t('vps.error.cancelled')}
              </p>
            ) : (
              <p className={styles.failed} role="alert">
                {vpsErrorText(outcome.code, outcome.detail, run.useKey)}
              </p>
            )}
            <div className={styles.actions}>
              <button ref={at('retry')} type="button" className={`${styles.button} ${styles.primary}`} onClick={retry}>
                {t('vps.retry')}
              </button>
            </div>
          </>
        )}
      </div>
    )
  }

  const errors = checked ? formErrors(form) : {}
  const keyMessage = keyError ?? errors.key ?? null
  return (
    <form
      className={styles.panel}
      noValidate
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <p className={styles.lead}>{t('vps.hint')}</p>
      <TextField
        label={t('vps.address')}
        value={form.address}
        onChange={(address) => change({ address })}
        hint={t('vps.addressHint')}
        error={errors.address}
        inputRef={at('address')}
      />
      <TextField label={t('vps.user')} value={form.user} onChange={(user) => change({ user })} error={errors.user} inputRef={at('user')} />
      <div className={styles.way} role="radiogroup" aria-labelledby={wayId}>
        <span id={wayId} className={styles.label}>
          {t('vps.auth')}
        </span>
        <label className={styles.option}>
          <input type="radio" name={wayId} checked={!form.useKey} onChange={() => change({ useKey: false })} />
          {t('vps.authPassword')}
        </label>
        <label className={styles.option}>
          <input type="radio" name={wayId} checked={form.useKey} onChange={() => change({ useKey: true })} />
          {t('vps.authKey')}
        </label>
      </div>
      {form.useKey ? (
        <>
          <div className={styles.field}>
            <div className={styles.keyRow}>
              <button
                ref={at('key')}
                type="button"
                className={styles.button}
                disabled={picking}
                aria-invalid={keyMessage !== null}
                aria-describedby={[form.keyName === null ? null : `${keyId}-name`, keyMessage === null ? null : `${keyId}-error`]
                  .filter((x) => x !== null)
                  .join(' ') || undefined}
                onClick={() => void pick()}
              >
                {form.keyName === null ? t('vps.pickKey') : t('vps.pickOtherKey')}
              </button>
              {form.keyName !== null && (
                <span id={`${keyId}-name`} className={styles.keyName}>
                  {t('vps.keyPicked', { name: form.keyName })}
                </span>
              )}
            </div>
            {keyMessage !== null && (
              <p id={`${keyId}-error`} className={styles.error}>
                {t(keyMessage)}
              </p>
            )}
          </div>
          <TextField
            label={t('vps.passphrase')}
            value={form.passphrase}
            onChange={(passphrase) => change({ passphrase })}
            error={errors.passphrase}
            secret
            inputRef={at('passphrase')}
          />
        </>
      ) : (
        <TextField
          label={t('vps.password')}
          value={form.password}
          onChange={(password) => change({ password })}
          error={errors.password}
          secret
          inputRef={at('password')}
        />
      )}
      <p className={styles.hint}>{t('vps.secretNote')}</p>
      <div className={styles.actions}>
        <button
          ref={at('submit')}
          type="submit"
          className={`${styles.button} ${styles.primary}`}
          disabled={live}
          title={live ? t('vps.routingOn') : undefined}
        >
          {t('vps.setUp')}
        </button>
      </div>
      {live && <p className={styles.hint}>{t('vps.routingOn')}</p>}
    </form>
  )
}
