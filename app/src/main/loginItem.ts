/**
 * The AppUserModelID of the window: the appId of electron-builder.yml, which the installer gives the Start menu and
 * desktop shortcuts. The same ID keeps a pinned shortcut and the running window one taskbar button.
 */
export const APP_USER_MODEL_ID = 'app.splitlane.window'

/**
 * The name of the window's value under HKCU\Software\Microsoft\Windows\CurrentVersion\Run. Electron names it after
 * the AppUserModelID unless told otherwise; it stays "SplitLane", the name the uninstaller removes.
 */
export const LOGIN_ITEM_NAME = 'SplitLane'

/** What app.setLoginItemSettings takes for the window. */
export interface LoginItemSettings {
  openAtLogin: boolean
  path: string
  args: string[]
  name: string
}

/** The Windows login item of the window (spec 9.6): the built app's exe, started hidden in the tray. */
export function loginItemSettings(openAtLogin: boolean, execPath: string): LoginItemSettings {
  return { openAtLogin, path: execPath, args: ['--hidden'], name: LOGIN_ITEM_NAME }
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
