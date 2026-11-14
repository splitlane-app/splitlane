import { contextBridge } from 'electron'

contextBridge.exposeInMainWorld('splitlane', {})
