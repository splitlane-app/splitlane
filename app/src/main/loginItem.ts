/** What app.setLoginItemSettings takes for the window. */
export interface LoginItemSettings {
  openAtLogin: boolean
  path: string
  args: string[]
}

/** The Windows login item of the window (spec 9.6): the built app's exe, started hidden in the tray. */
export function loginItemSettings(openAtLogin: boolean, execPath: string): LoginItemSettings {
  return { openAtLogin, path: execPath, args: ['--hidden'] }
}

/**
 * The login item to apply once at start: the saved setting again, for the exe of this start (the unpacked folder
 * may have moved since the toggle last changed, and the item would name the old place). Only the built app has one.
 */
export function startupLoginItem(
  app: { isPackaged: boolean; execPath: string },
  openAtLogin: boolean
): LoginItemSettings | null {
  return app.isPackaged ? loginItemSettings(openAtLogin, app.execPath) : null
}
