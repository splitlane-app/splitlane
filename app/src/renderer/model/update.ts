import type { UpdateState } from '../../shared/update'
import { t } from '../i18n/t'

export type UpdateAction = 'install' | 'notes' | 'later' | 'cancel' | 'retry' | 'dismiss'

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
        text: t('update.downloadFailed', { v: s.version, reason: s.reason }),
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
            : t('update.installRefused', { v: s.version, reason: s.reason }),
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
      return t('update.checkFailed', { reason: s.reason })
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
