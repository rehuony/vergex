import React from 'react'
import ReactDOM from 'react-dom/client'
import { App } from './app'
import { QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from 'sonner'
// Served with the app, so the first paint waits on no other site.
import '@fontsource-variable/inter/wght.css'
import '@fontsource/ibm-plex-mono/400.css'
import '@fontsource/ibm-plex-mono/500.css'
import '@fontsource/ibm-plex-mono/600.css'
import '@/styles/index.css'
import { BrowserRouter } from 'react-router-dom'
import { initPageScrollbars } from '@/utils/page-scroll'
import { queryClient, restoreQueryCache } from '@/utils/query-client'

initPageScrollbars()
restoreQueryCache()

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      {/* Colours come from the theme tokens in styles/tokens.css. */}
      <Toaster
        closeButton
        // The corner holds the assistant's window and icon.
        position="bottom-center"
        duration={2200}
        toastOptions={{
          className: 'vergex-toast',
        }}
      />
      <QueryClientProvider client={queryClient}>
        <App />
      </QueryClientProvider>
    </BrowserRouter>
  </React.StrictMode>
)
