// A SplitLaneBridge that answers from scenes.ts and from nothing else: no main process, no pipe, no network.
import type { SplitLaneBridge } from '../../shared/ipc'
import { DEFAULT_SETTINGS } from '../../shared/settings'
import { monogram } from './monogram'
import { PHONE_ADDRESS, PROGRAMS, sceneOf, type SceneName } from './scenes'

const base = (exe: string): string => exe.slice(exe.lastIndexOf('\\') + 1).toLowerCase()

/** `icons`: data URLs by file name of the exe (lower case), from icons.local.json when it is there. */
export function demoBridge(scene: SceneName, icons: Record<string, string>): SplitLaneBridge {
  const snap = sceneOf(scene).snapshot
  const nameByFile = new Map(PROGRAMS.map((p) => [base(p.exe), p.name]))
  return {
    snapshot: async () => snap,
    onSnapshot: () => () => {},
    onActivity: () => () => {},
    onOpenSettings: () => () => {},
    command: async () => ({ ok: true }),
    setActivityDetail: async () => {},
    installHelper: async () => {},
    proxyAdd: async () => ({ kind: 'cancelled' }),
    proxyAddFile: async () => ({ kind: 'cancelled' }),
    proxyRefresh: async () => ({ kind: 'cancelled' }),
    proxyReveal: async () => ({ address: PHONE_ADDRESS }),
    vpsPickKey: async () => null,
    vpsForgetKey: async () => {},
    vpsSetup: async () => ({ kind: 'cancelled' }),
    vpsCancel: async () => {},
    onVpsEvent: () => () => {},
    appInfo: async () => ({ version: '0.5.0', installed: true }),
    retryLink: async () => {},
    setSettings: async (patch) => ({ ...DEFAULT_SETTINGS, ...patch }),
    installedPrograms: async () => [],
    pickProgram: async () => null,
    resolveDropped: async () => [],
    icon: async (exe) => icons[base(exe)] ?? monogram(nameByFile.get(base(exe)) ?? base(exe)),
    openLogin: async () => false,
    saveDiagnostics: async () => ({ saved: false }),
    quit: async () => {},
    checkUpdates: async () => {},
    installUpdate: async () => {},
    cancelUpdate: async () => {},
    updateLater: async () => {},
    dismissUpdate: async () => {},
    openUpdateNotes: async () => false
  }
}
