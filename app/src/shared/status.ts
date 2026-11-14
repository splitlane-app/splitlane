import type { HelloResult } from './protocol'

/** The window's link to the helper (spec 4.3, 5.1, section 10). */
export type LinkStatus =
  | { kind: 'connecting' }
  | { kind: 'ready'; hello: HelloResult }
  | { kind: 'setup-needed' }
  | { kind: 'other-user' }
  | { kind: 'helper-older'; helperProtocol: number }
  | { kind: 'helper-newer'; helperProtocol: number }
  | { kind: 'restarting'; attempt: number }
  | { kind: 'unresponsive' }

/** The result of `splitlane-helper.exe install` run from the window (task 4). */
export type InstallOutcome =
  | { kind: 'ok' }
  | { kind: 'cancelled' }
  | { kind: 'refused'; reason: string }
  | { kind: 'failed'; detail: string }
