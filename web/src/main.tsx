import React from 'react'
import ReactDOM from 'react-dom/client'
import { App } from './app'
import { Toaster } from 'sonner'
import '@/styles/index.css'
import '@/styles/brand.css'
import { BrowserRouter } from 'react-router-dom'
import { initPageScrollbars } from '@/utils/page-scroll'

initPageScrollbars()

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      {/* Colours come from the theme tokens in styles/brand.css. */}
      <Toaster
        closeButton
        position="bottom-right"
        duration={2200}
        toastOptions={{
          className: 'vergex-toast',
        }}
      />
      <App />
    </BrowserRouter>
  </React.StrictMode>
)
