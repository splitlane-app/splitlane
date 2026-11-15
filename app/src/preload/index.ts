import { contextBridge, ipcRenderer, webUtils, type IpcRendererEvent } from 'electron'
import { IPC, MAX_DROPPED, type SplitLaneBridge } from '../shared/ipc'

// Sandboxed preload: the bundle requires nothing but `electron`; the shared module is inlined.

function listen<T>(channel: string, cb: (x: T) => void): () => void {
  const listener = (_event: IpcRendererEvent, x: T): void => cb(x)
  ipcRenderer.on(channel, listener)
  return () => {
    ipcRenderer.removeListener(channel, listener)
  }
}

const bridge: SplitLaneBridge = {
  snapshot: () => ipcRenderer.invoke(IPC.snapshot),
  onSnapshot: (cb) => listen(IPC.snapshotChanged, cb),
  onActivity: (cb) => listen(IPC.activity, cb),
  // nothing of the push reaches the page: only that it came
  onOpenSettings: (cb) => listen<unknown>(IPC.openSettings, () => cb()),
  command: (c) => ipcRenderer.invoke(IPC.command, c),
  setActivityDetail: (key) => ipcRenderer.invoke(IPC.setActivityDetail, key),
  installHelper: () => ipcRenderer.invoke(IPC.installHelper),
  appInfo: () => ipcRenderer.invoke(IPC.appInfo),
  retryLink: () => ipcRenderer.invoke(IPC.retryLink),
  setSettings: (patch) => ipcRenderer.invoke(IPC.setSettings, patch),
  installedPrograms: () => ipcRenderer.invoke(IPC.installedPrograms),
  pickProgram: () => ipcRenderer.invoke(IPC.pickProgram),
  // only strings cross: the path of each file, or its bare name when it has no path on disk
  // (the main process then reports it as skipped)
  resolveDropped: (files) =>
    ipcRenderer.invoke(
      IPC.resolveDropped,
      files.slice(0, MAX_DROPPED).map((f) => webUtils.getPathForFile(f) || f.name)
    ),
  icon: (exe) => ipcRenderer.invoke(IPC.icon, exe),
  openLogin: (url) => ipcRenderer.invoke(IPC.openLogin, url),
  saveDiagnostics: () => ipcRenderer.invoke(IPC.saveDiagnostics),
  quit: () => ipcRenderer.invoke(IPC.quit)
}

contextBridge.exposeInMainWorld('splitlane', bridge)
