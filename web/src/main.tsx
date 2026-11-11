import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router'
import './tokens.css'
import './index.css'
import App from './App.tsx'
import { AuthProvider } from './AuthProvider'
import { LangProvider } from './LangProvider'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <LangProvider>
        <AuthProvider>
          <App />
        </AuthProvider>
      </LangProvider>
    </BrowserRouter>
  </StrictMode>,
)
