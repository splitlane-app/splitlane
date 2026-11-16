// The demo entry: the real App and the real store, fed from scenes.ts through a fake bridge. Used for the
// screenshots only (app/scripts/capture-screens.mjs); it is not in the product build.
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from '../App'
import { IconCache } from '../bridge'
import { ErrorBoundary } from '../components/ErrorBoundary'
import { setLang, t } from '../i18n/t'
import { ServicesContext, type Services } from '../services'
import { useAppStore } from '../store/appStore'
import '../styles/theme.css'
import { demoBridge } from './fakeBridge'
import { activityAt, HISTORY_SECONDS, SCENES, sceneOf, type SceneName } from './scenes'

const params = new URLSearchParams(window.location.search)
const screen = (SCENES as readonly string[]).includes(params.get('screen') ?? '') ? (params.get('screen') as SceneName) : 'board'
setLang('en')
document.documentElement.lang = 'en'

// the icons of the programs installed on this PC, written by scripts/demo-icons.ps1 (git-ignored); none is fine
const found = import.meta.glob<Record<string, string>>('./icons.local.json', { eager: true, import: 'default' })
const icons = Object.values(found)[0] ?? {}

const bridge = demoBridge(screen, icons)
// the store sends its commands through window.splitlane: give it the demo bridge too
Object.defineProperty(window, 'splitlane', { value: bridge })
const services: Services = { store: useAppStore, bridge, icons: new IconCache((exe) => bridge.icon(exe)) }
const scene = sceneOf(screen)
const store = useAppStore.getState()

store.setSnapshot(scene.snapshot)
if (scene.live) for (let i = 0; i < HISTORY_SECONDS; i++) useAppStore.getState().setActivity(activityAt(i))
if (scene.connection) store.showConnection(true)
if (scene.vps !== null) {
  const run = scene.vps(Date.now())
  store.setVps(() => run)
}

createRoot(document.getElementById('root') as HTMLElement).render(
  <StrictMode>
    <ErrorBoundary>
      <ServicesContext value={services}>
        <App />
      </ServicesContext>
    </ErrorBoundary>
  </StrictMode>
)

const sleep = (ms: number): Promise<void> => new Promise((r) => setTimeout(r, ms))

async function click(find: () => Element | null | undefined): Promise<void> {
  for (let i = 0; i < 50; i++) {
    const el = find()
    if (el instanceof HTMLElement) {
      el.click()
      return
    }
    await sleep(50)
  }
  throw new Error('demo: the control to click did not appear')
}

const byText = (selector: string, text: string) => (): Element | undefined =>
  Array.from(document.querySelectorAll(selector)).find((e) => e.textContent?.includes(text))

async function ready(): Promise<void> {
  if (scene.vpsTile) await click(byText('[role="radio"]', t('connect.vps')))
  if (scene.phone) {
    await click(byText('button', t('phone.connect')))
    await click(() => document.querySelector('[role="dialog"]'))
  }
  // the capture script waits for the icons and the QR code itself
  document.documentElement.setAttribute('data-demo-ready', '1')
}
void ready()
