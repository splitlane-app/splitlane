/**
 * Command line switches that would let a program of the user drive the installed window: a DevTools endpoint, the
 * Node.js inspector or V8 flags (final review of stage 2 part 1, item 2). The helper serves the installed
 * SplitLane.exe without elevation, so the built window exits at once when it gets one of them. The fuses already
 * turn off the Node.js inspector arguments (electron-builder.yml); this check covers the Chromium side too.
 */
export const FORBIDDEN_SWITCHES = [
  'remote-debugging-port',
  'remote-debugging-pipe',
  'remote-allow-origins',
  'inspect',
  'inspect-brk',
  'inspect-port',
  'js-flags'
] as const

/** The first forbidden switch the command line has (`has` is app.commandLine.hasSwitch), or null. */
export function forbiddenSwitch(has: (name: string) => boolean): string | null {
  return FORBIDDEN_SWITCHES.find((name) => has(name)) ?? null
}
