import { readFileSync } from 'node:fs'
import { expect, test } from 'vitest'
import { APP_USER_MODEL_ID, LOGIN_ITEM_NAME, loginItemSettings, startupLoginItem } from './loginItem'

const EXE = 'D:\\Portable\\SplitLane\\SplitLane.exe'

test('the login item starts the built app hidden in the tray', () => {
  expect(loginItemSettings(true, EXE)).toEqual({ openAtLogin: true, path: EXE, args: ['--hidden'], name: 'SplitLane' })
  expect(loginItemSettings(false, EXE)).toEqual({ openAtLogin: false, path: EXE, args: ['--hidden'], name: 'SplitLane' })
})

test('the saved autostart is applied again at every start of the built app', () => {
  // the unpacked folder may have moved since the toggle was last changed: the item names the exe of this start
  expect(startupLoginItem({ isPackaged: true, execPath: EXE }, true)).toEqual(loginItemSettings(true, EXE))
  expect(startupLoginItem({ isPackaged: true, execPath: EXE }, false)).toEqual(loginItemSettings(false, EXE))
  // a development run never registers electron.exe
  expect(startupLoginItem({ isPackaged: false, execPath: 'C:\\code\\electron.exe' }, true)).toBeNull()
})

test('the window has the AppUserModelID of its shortcuts, the appId of electron-builder.yml', () => {
  // the installer gives the shortcuts the appId: another ID would make the running window a taskbar button of its own
  const yml = readFileSync(new URL('../../electron-builder.yml', import.meta.url), 'utf8')
  const appId = /^appId:\s*(\S+)\s*$/m.exec(yml)?.[1]
  expect(appId).toBe('app.splitlane.window')
  expect(APP_USER_MODEL_ID).toBe(appId)
})

test('the Run value keeps the name SplitLane whatever the AppUserModelID', () => {
  // the uninstaller removes HKCU\...\Run "SplitLane"; earlier builds wrote it under that name too (their AUMID)
  expect(LOGIN_ITEM_NAME).toBe('SplitLane')
  expect(loginItemSettings(true, EXE).name).toBe(LOGIN_ITEM_NAME)
})
