import type { VpsEvent, VpsOutcome, VpsRequest, VpsStep } from '../../shared/ipc'
import { ERROR_CODES, PROXY_REJECT_CODES } from '../../shared/protocol'
import { parseVpsAddress, validVpsUser } from '../../shared/vps'
import type { Key } from '../i18n/ru'
import { isKey, LIMIT_PARAMS, t } from '../i18n/t'

// «Свой VPS» (stage 3 part 2, spec 6, 8): the check of the form, the steps of the progress and the texts of the end.
// The secrets (the password, the passphrase) live in the form only: nothing here keeps them.

/** The longest password or passphrase the main process takes (vpsSetup.ts); the form says so before it is sent. */
export const MAX_SECRET_CHARS = 1024

export interface VpsForm {
  address: string
  user: string
  useKey: boolean
  password: string
  /** The base name of the key file the main process holds; null until one is picked. */
  keyName: string | null
  passphrase: string
}

export const EMPTY_FORM: VpsForm = { address: '', user: 'root', useKey: false, password: '', keyName: null, passphrase: '' }

export type VpsField = 'address' | 'user' | 'password' | 'key' | 'passphrase'

/**
 * The errors of the form by field, with the rules of the main process and the helper (shared/vps.ts): the address as
 * typed (spaces at the ends cut), the login trimmed as the helper trims it, a password or passphrase of at most 1024
 * characters (a longer one is a password error, never the one of the address and the login).
 */
export function formErrors(f: VpsForm): Partial<Record<VpsField, Key>> {
  const e: Partial<Record<VpsField, Key>> = {}
  if (f.address.trim() === '') e.address = 'vps.check.addressEmpty'
  else if (parseVpsAddress(f.address) === null) e.address = 'vps.check.address'
  const user = f.user.trim()
  if (user === '') e.user = 'vps.check.userEmpty'
  else if (!validVpsUser(user)) e.user = 'vps.check.user'
  if (f.useKey) {
    if (f.keyName === null) e.key = 'vps.check.key'
    if (f.passphrase.length > MAX_SECRET_CHARS) e.passphrase = 'vps.check.passphraseLong'
  } else if (f.password === '') e.password = 'vps.check.passwordEmpty'
  else if (f.password.length > MAX_SECRET_CHARS) e.password = 'vps.check.passwordLong'
  return e
}

/** The request of a form that passed: only the secret of the way in it uses (the key itself is in the main process). */
export function vpsRequest(f: VpsForm): VpsRequest {
  const where = { address: f.address.trim(), user: f.user.trim() }
  if (!f.useKey) return { ...where, password: f.password, useKey: false }
  return f.passphrase === '' ? { ...where, useKey: true } : { ...where, useKey: true, keyPassphrase: f.passphrase }
}

// ---- the steps ----

/** The steps of the progress, in order: the first five are the subcommand's, `add` is the window's. */
export const VPS_STEPS: readonly VpsStep[] = ['connect', 'system', 'install', 'harden', 'subscription', 'add']

/**
 * What a step has seen: whether it runs now, and how its parts ended. The install has three parts (sing-box, the
 * configuration, the service), each with its start and its end, so a step may start again after it ended.
 */
export interface StepTrack {
  running: boolean
  done: boolean
  skip: boolean
  warn: boolean
  failed: boolean
  cancelled: boolean
}

export type VpsSteps = Readonly<Record<VpsStep, StepTrack>>

/** What a step shows: «ждёт», «идёт», «готово», «пропущено», «не удалось» (a warning), «ошибка», «отменено». */
export type StepView = 'waiting' | 'running' | 'done' | 'skip' | 'warn' | 'failed' | 'cancelled'

const IDLE: StepTrack = { running: false, done: false, skip: false, warn: false, failed: false, cancelled: false }

/** A failure or a cancel wins; then a part still running; then the worst ending of the parts (a skipped part does not hide a done one). */
export function stepView(s: StepTrack): StepView {
  if (s.failed) return 'failed'
  if (s.cancelled) return 'cancelled'
  if (s.running) return 'running'
  if (s.warn) return 'warn'
  if (s.done) return 'done'
  if (s.skip) return 'skip'
  return 'waiting'
}

/** One setup as the page shows it: no secret, only the steps, the times and the end. */
export interface VpsRun {
  steps: VpsSteps
  startedAt: number
  endedAt: number | null
  outcome: VpsOutcome | null
  /** The login was by a key (ssh_auth then speaks of the key). */
  useKey: boolean
}

export function newRun(at: number, useKey: boolean): VpsRun {
  const steps = Object.fromEntries(VPS_STEPS.map((s) => [s, IDLE])) as Record<VpsStep, StepTrack>
  return { steps, startedAt: at, endedAt: null, outcome: null, useKey }
}

/** A step event of the main process; one of an unknown step or state (a newer window) changes nothing. */
export function runEvent(run: VpsRun, e: VpsEvent): VpsRun {
  if (!(VPS_STEPS as readonly string[]).includes(e.step)) return run
  const s = run.steps[e.step]
  let next: StepTrack
  switch (e.state) {
    case 'start':
      next = { ...s, running: true }
      break
    case 'done':
      next = { ...s, running: false, done: true }
      break
    case 'skip':
      next = { ...s, running: false, skip: true }
      break
    case 'warn':
      next = { ...s, running: false, warn: true }
      break
    case 'failed':
      next = { ...s, running: false, failed: true }
      break
    default:
      return run
  }
  return { ...run, steps: { ...run.steps, [e.step]: next } }
}

/** The end: a step still running ends as the setup did (done, cancelled or failed); the others stay as they were. */
export function runEnd(run: VpsRun, outcome: VpsOutcome, at: number): VpsRun {
  const close = (s: StepTrack): StepTrack => {
    if (!s.running) return s
    if (outcome.kind === 'done') return { ...s, running: false, done: true }
    if (outcome.kind === 'cancelled') return { ...s, running: false, cancelled: true }
    return { ...s, running: false, failed: true }
  }
  const steps = Object.fromEntries(VPS_STEPS.map((s) => [s, close(run.steps[s])])) as Record<VpsStep, StepTrack>
  return { ...run, steps, outcome, endedAt: at }
}

/** Whether any step moved: a setup refused before it began (routing on, busy) shows no progress. */
export function runMoved(run: VpsRun): boolean {
  return VPS_STEPS.some((s) => stepView(run.steps[s]) !== 'waiting')
}

/** «1:05»: minutes and seconds (the setup is cut at 10 minutes). */
export function elapsedText(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000))
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

// ---- the texts of the end ----

/** The codes with a text: the subcommand's and the kit's (spec 8), and the window's own. */
export const VPS_ERROR_CODES = [
  'bad_input',
  'ssh_unreachable',
  'ssh_auth',
  'ssh_key',
  'password_expired',
  'no_admin',
  'os_unsupported',
  'download_failed',
  'ports_busy',
  'kit_failed',
  'timeout',
  'cancelled',
  'connection_lost',
  'routing_on',
  'busy',
  'add_failed'
] as const

const DETAIL_KEYS: Partial<Record<string, readonly string[]>> = {
  bad_input: ['key', 'password', 'passphrase'],
  no_admin: ['sudo_needs_password'],
  ssh_key: ['ppk', 'passphrase', 'format']
}

/**
 * Why the helper did not take the result (the detail of add_failed): the text of its code, or null for one without a
 * text of its own (no_servers, a code of a newer helper).
 */
function addReason(code: string | undefined): string | null {
  if (code === undefined) return null
  if (code === 'apply_failed') return t('proxy.applyFailed') // servers.json, not a layout
  const known = (ERROR_CODES as readonly string[]).includes(code) || (PROXY_REJECT_CODES as readonly string[]).includes(code)
  const key = `error.${code}`
  return known && isKey(key) ? t(key, LIMIT_PARAMS) : null
}

/** The text of a failed setup by its code and detail (spec 8); a code of a newer window gets the general one. */
export function vpsErrorText(code: string, detail: string | undefined, useKey: boolean): string {
  if (!(VPS_ERROR_CODES as readonly string[]).includes(code)) return t('vps.error.unknown', { code })
  if (detail !== undefined && DETAIL_KEYS[code]?.includes(detail)) {
    const key = `vps.error.${code}.${detail}`
    if (isKey(key)) return t(key)
  }
  switch (code) {
    case 'ssh_auth':
      return t(useKey ? 'vps.error.ssh_auth.key' : 'vps.error.ssh_auth')
    case 'kit_failed':
      if (detail === 'spawn') return t('vps.error.kit_failed.spawn')
      return detail === undefined || detail.trim() === '' ? t('vps.error.kit_failed.plain') : t('vps.error.kit_failed', { detail })
    case 'add_failed': {
      const reason = addReason(detail)
      return reason === null ? t('vps.error.add_failed.plain') : t('vps.error.add_failed', { reason })
    }
  }
  return t(`vps.error.${code}` as Key)
}

/** The notice of an end that no screen showed (the screen went meanwhile): the number of servers, or what went wrong. */
export function vpsNotice(o: VpsOutcome, useKey: boolean): string {
  if (o.kind === 'done') return t('vps.done', { n: o.servers })
  if (o.kind === 'cancelled') return t('vps.error.cancelled')
  return vpsErrorText(o.code, o.detail, useKey)
}

/** The warnings of the phone, the first one only: each says the phone cannot connect, with its reason. */
const PHONE_WARNINGS = ['sub_port80_busy', 'sub_skipped', 'sub_unreachable'] as const

/** The warnings of a done setup, in the order of the steps: the protection, then the phone; unknown ones are not shown. */
export function warningTexts(warnings: readonly string[]): string[] {
  const out: string[] = []
  if (warnings.includes('harden_failed')) out.push(t('vps.warn.harden_failed'))
  const phone = PHONE_WARNINGS.find((w) => warnings.includes(w))
  if (phone !== undefined) out.push(t(`vps.warn.${phone}`))
  return out
}

/** The field to fix after a failure, where «Попробовать снова» puts the focus; null for none in particular. */
export function retryFocus(o: VpsOutcome, useKey: boolean): VpsField | null {
  if (o.kind !== 'failed') return null
  const secret: VpsField = useKey ? 'key' : 'password'
  switch (o.code) {
    case 'bad_input':
      return o.detail === 'address' || o.detail === 'user' || o.detail === 'password' || o.detail === 'key' || o.detail === 'passphrase'
        ? o.detail
        : 'address'
    case 'ssh_unreachable':
    case 'os_unsupported':
      return 'address'
    case 'ssh_auth':
    case 'password_expired':
      return secret
    case 'ssh_key':
      return o.detail === 'passphrase' ? 'passphrase' : 'key'
    case 'no_admin':
      return 'user'
  }
  return null
}
