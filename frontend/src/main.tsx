import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClientProvider } from '@tanstack/react-query'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import { BrowserRouter } from 'react-router-dom'
import './index.css'
import 'maplibre-gl-draw/dist/mapbox-gl-draw.css'
import App from './App'
import { initVisibilityRefresh, initAuthSync } from './lib/auth'
import { queryClient } from './lib/queryClient'

const root = document.getElementById('root')
if (!root) throw new Error('Root element not found')

initAuthSync()
initVisibilityRefresh()

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
      <ReactQueryDevtools initialIsOpen={false} />
    </QueryClientProvider>
  </StrictMode>,
)
