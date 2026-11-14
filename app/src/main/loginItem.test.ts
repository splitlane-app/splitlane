import { expect, test } from 'vitest'
import { loginItemSettings, startupLoginItem } from './loginItem'

const EXE = 'D:\\Portable\\SplitLane\\SplitLane.exe'

test('the login item starts the built app hidden in the tray', () => {
  expect(loginItemSettings(true, EXE)).toEqual({ openAtLogin: true, path: EXE, args: ['--hidden'] })
  expect(loginItemSettings(false, EXE)).toEqual({ openAtLogin: false, path: EXE, args: ['--hidden'] })
})

test('the saved autostart is applied again at every start of the built app', () => {
  // the unpacked folder may have moved since the toggle was last changed: the item names the exe of this start
  expect(startupLoginItem({ isPackaged: true, execPath: EXE }, true)).toEqual(loginItemSettings(true, EXE))
  expect(startupLoginItem({ isPackaged: true, execPath: EXE }, false)).toEqual(loginItemSettings(false, EXE))
  // a development run never registers electron.exe
  expect(startupLoginItem({ isPackaged: false, execPath: 'C:\\code\\electron.exe' }, true)).toBeNull()
})
