import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import { connectBridge, IconCache } from './bridge'
import { ErrorBoundary } from './components/ErrorBoundary'
import { ServicesContext, type Services } from './services'
import { useAppStore } from './store/appStore'
import './styles/theme.css'

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
