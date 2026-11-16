// Captures the scenes of the demo window to docs/screens/<scene>-<theme>.png (dark only).
//   pnpm demo:build && pnpm demo:capture      (from app/)
// Runs under Electron (`electron scripts/capture-screens.mjs`) and loads only out-demo/demo/demo.html, a page with a
// fake bridge: no main process of SplitLane, no helper, no pipe, no network.
import { app, BrowserWindow } from 'electron'
import { mkdirSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

// the same time zone on any PC: the subscription's «Updated» time is shown in local time; set before the first
// window, so that the renderer processes inherit it
process.env.TZ = 'UTC'

const here = dirname(fileURLToPath(import.meta.url))
const page = resolve(here, '../out-demo/demo/demo.html')
const outDir = resolve(here, '../../docs/screens')
const SCENES = ['board', 'connection', 'vps-progress', 'vps-done', 'phone']
// the app has one theme, the dark one
const THEMES = ['dark']
const WIDTH = 1200
const HEIGHT = 760
// the board is cropped to its content (the app fills the window, so its layout is the same, only shorter)
const HEIGHTS = { board: 480 }
// what a scene shows when it is done: the program icons (img) and, for the phone dialog, the QR code
const EXPECT = { board: { imgs: 7 }, phone: { selector: 'svg[role="img"]' } }
const ZOOM = 2

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

async function capture(scene, theme) {
  const height = HEIGHTS[scene] ?? HEIGHT
  const win = new BrowserWindow({
    width: WIDTH * ZOOM,
    height: height * ZOOM,
    useContentSize: true,
    enableLargerThanScreen: true,
    show: false,
    frame: false,
    webPreferences: { offscreen: true, contextIsolation: true, nodeIntegration: false, sandbox: true, zoomFactor: ZOOM }
  })
  win.setContentSize(WIDTH * ZOOM, height * ZOOM)
  const url = pathToFileURL(page)
  url.search = `screen=${scene}`
  await win.loadURL(url.href)
  win.webContents.setZoomFactor(ZOOM)
  // done when the page says so, every <img> has loaded, the scene has all its icons and the QR code is there, on two
  // polls in a row (a scene that never gets there throws)
  const want = EXPECT[scene] ?? {}
  const probe = `(() => {
    const imgs = Array.from(document.images)
    const sel = ${JSON.stringify(want.selector ?? null)}
    return document.documentElement.hasAttribute('data-demo-ready') &&
      imgs.every((i) => i.complete && i.naturalWidth > 0) &&
      imgs.length >= ${want.imgs ?? 0} &&
      (sel === null || document.querySelector(sel) !== null)
  })()`
  const deadline = Date.now() + 15_000
  for (let steady = 0; steady < 2; ) {
    steady = (await win.webContents.executeJavaScript(probe)) ? steady + 1 : 0
    if (Date.now() > deadline) throw new Error(`${scene}-${theme}: the scene did not settle`)
    await sleep(100)
  }
  const css = await win.webContents.executeJavaScript("innerWidth + 'x' + innerHeight")
  if (css !== `${WIDTH}x${height}`) throw new Error(`${scene}-${theme}: the page is ${css} CSS px, not ${WIDTH}x${height}`)
  const image = await win.webContents.capturePage()
  const file = resolve(outDir, `${scene}-${theme}.png`)
  writeFileSync(file, image.toPNG())
  const size = image.getSize()
  console.log(`${scene}-${theme}.png ${size.width}x${size.height}`)
  win.destroy()
  await sleep(300)
}

// the app must outlive the window of each scene
app.on('window-all-closed', () => {})
app.disableHardwareAcceleration()
// no top-level await on whenReady: Electron finishes loading an ES module main before it emits ready
app.whenReady().then(async () => {
  mkdirSync(outDir, { recursive: true })
  try {
    for (const scene of SCENES) for (const theme of THEMES) await capture(scene, theme)
    app.exit(0)
  } catch (e) {
    console.error(e)
    app.exit(1)
  }
})
