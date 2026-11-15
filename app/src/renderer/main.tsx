import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { resolveLanguage } from '../shared/i18n'
import { App } from './App'
import { connectBridge, IconCache } from './bridge'
import { ErrorBoundary } from './components/ErrorBoundary'
import { setLang } from './i18n/t'
import { ServicesContext, type Services } from './services'
import { useAppStore } from './store/appStore'
import './styles/theme.css'

// until the first snapshot names the language in force (the splash): the one of Windows, as by the default setting;
// App makes it the `lang` of the document
setLang(resolveLanguage('system', navigator.languages))

const bridge = window.splitlane
const services: Services = {
  store: useAppStore,
  bridge,
  icons: new IconCache((exe) => bridge.icon(exe))
}
// once for the life of the page, before the first render: no push is missed
connectBridge(bridge, useAppStore)

createRoot(document.getElementById('root') as HTMLElement).render(
  <StrictMode>
    <ErrorBoundary>
      <ServicesContext value={services}>
        <App />
      </ServicesContext>
    </ErrorBoundary>
  </StrictMode>
)
