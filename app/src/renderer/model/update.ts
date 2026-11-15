import type { UpdateState } from '../../shared/update'
import type { Key } from '../i18n/ru'
import { t } from '../i18n/t'

export type UpdateAction = 'install' | 'notes' | 'later' | 'cancel' | 'retry' | 'dismiss'

/**
 * The English reasons of the updater by their exact text: main/updater.ts (its errors, limits and NOT_* texts),
 * main/netHop.ts and pickRelease of shared/update.ts. A test reads those files: a new literal reason needs its line.
 */
const REASONS: ReadonlyMap<string, Key> = new Map<string, Key>([
  ['updates are available only for an installed SplitLane', 'update.reason.notInstalled'],
  ['the installer did not start the update', 'update.reason.notStarted'],
  ['cancelled', 'update.reason.cancelled'],
  ['aborted', 'update.reason.aborted'],
  ['the release is not available', 'update.reason.releaseGone'],
  ['the checksum file is larger than 1 KB', 'update.reason.checksumTooLarge'],
  ['the checksum file is not valid', 'update.reason.checksumInvalid'],
  ['the file is larger than the release says', 'update.reason.fileLargerThanRelease'],
  ['the file is larger than 400 MB', 'update.reason.fileTooLarge'],
  ['the file does not match its checksum', 'update.reason.checksumMismatch'],
  ['the release answer is larger than 1 MB', 'update.reason.answerTooLarge'],
  ['the release answer is not valid JSON', 'update.reason.answerNotJson'],
  ['no data for 60 seconds', 'update.reason.stalled'],
  ['the address is not allowed', 'update.reason.addressNotAllowed'],
  ['redirect to a host that is not allowed', 'update.reason.redirectNotAllowed'],
  ['too many redirects', 'update.reason.tooManyRedirects'],
  ['redirect without a location', 'update.reason.redirectNoLocation'],
  ['redirect to a bad address', 'update.reason.redirectBadAddress'],
  ['HTTP 403 (GitHub request limit?)', 'update.reason.http403'],
  ['the answer has no body', 'update.reason.noBody'],
  ['release is not an object', 'update.reason.releaseNotObject'],
  ['release is a draft or a prerelease', 'update.reason.releaseDraft'],
  ['bad release tag', 'update.reason.releaseTag'],
  ['bad current version', 'update.reason.currentVersion'],
  ['release file address is not the expected one', 'update.reason.releaseAddress'],
  ['installer size is out of range', 'update.reason.installerSize']
])

/** The reasons with a value in them: the pattern, the text, the name of its one parameter. */
const PATTERNS: readonly (readonly [RegExp, Key, string])[] = [
  [/^HTTP (\d{3})$/, 'update.reason.http', 'status'],
  [/^powershell exit (-?\d+)$/, 'update.reason.powershellExit', 'code'],
  [/^release must have exactly one file (\S+)$/, 'update.reason.releaseFiles', 'name']
]

/**
 * A reason of the updater in the language of the window. The main process words its reasons in English; the known
 * ones have a text of the dictionary, any other (a network error, the reason of a refused install) is shown as is.
 */
export function translateUpdateReason(reason: string): string {
  const key = REASONS.get(reason)
  if (key !== undefined) return t(key)
  for (const [pattern, text, param] of PATTERNS) {
    const m = pattern.exec(reason)
    if (m !== null) return t(text, { [param]: m[1] })
  }
  return reason
}

export interface UpdateBannerModel {
  text: string
  actions: UpdateAction[]
  tone: 'info' | 'warn'
}

/** What the update banner over the board says; null when the state needs none. */
export function updateBannerModel(s: UpdateState): UpdateBannerModel | null {
  switch (s.kind) {
    case 'available':
      if (s.hidden) return null
      return { text: t('update.available', { v: s.version }), actions: ['install', 'notes', 'later'], tone: 'info' }
    case 'downloading':
      return { text: t('update.downloading', { v: s.version, p: s.percent }), actions: ['cancel'], tone: 'info' }
    case 'verifying':
      return { text: t('update.verifying', { v: s.version }), actions: [], tone: 'info' }
    case 'launching':
      return { text: t('update.launching', { v: s.version }), actions: [], tone: 'info' }
    case 'cancelled':
      return { text: t('update.cancelled'), actions: ['install', 'dismiss'], tone: 'info' }
    case 'downloadFailed':
      return {
        text: t('update.downloadFailed', { v: s.version, reason: translateUpdateReason(s.reason) }),
        actions: ['retry', 'dismiss'],
        tone: 'warn'
      }
    case 'installed':
      return { text: t('update.installed', { v: s.version }), actions: ['dismiss'], tone: 'info' }
    case 'installRefused':
      return {
        text:
          s.reason === ''
            ? t('update.installRefusedNoReason', { v: s.version })
            : t('update.installRefused', { v: s.version, reason: translateUpdateReason(s.reason) }),
        // a refusal of the window's own version has nothing to install again: it can only be closed
        actions: s.canRetry ? ['retry', 'notes', 'dismiss'] : ['dismiss'],
        tone: 'warn'
      }
    case 'idle':
    case 'checking':
    case 'upToDate':
    case 'checkFailed':
      return null
  }
}

/** The line under the versions on «Подключение»; null when there is nothing to say. */
export function updateStatusLine(s: UpdateState): string | null {
  switch (s.kind) {
    case 'idle':
      return null
    case 'checking':
      return t('update.checking')
    case 'upToDate':
      return t('update.upToDate')
    case 'checkFailed':
      return t('update.checkFailed', { reason: translateUpdateReason(s.reason) })
    case 'available':
      // a hidden offer still shows here: the user may have changed their mind
      return t('update.available', { v: s.version })
    default:
      return updateBannerModel(s)?.text ?? null
  }
}

/** True while a check or a download is running: the check button waits. */
export function updateBusy(s: UpdateState): boolean {
  return s.kind === 'checking' || s.kind === 'downloading' || s.kind === 'verifying' || s.kind === 'launching'
}
