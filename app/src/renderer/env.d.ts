/// <reference types="vite/client" />

interface Window {
  /** The bridge of the preload script (src/preload/index.ts): the page's only way to the main process. */
  readonly splitlane: import('../shared/ipc').SplitLaneBridge
}
