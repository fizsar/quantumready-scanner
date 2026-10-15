import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { describe, expect, it, vi } from 'vitest'
import App from './App'
import { AuthProvider } from './AuthProvider'
import { detectLang, translate } from './i18n'
import { LangProvider } from './LangProvider'
import { json, setNavigatorLanguages, stubFetch } from './test/fixtures'

describe('detectLang', () => {
  it('follows the browser: Spanish for es-*, English otherwise', () => {
    setNavigatorLanguages(['es-MX', 'en'])
    expect(detectLang()).toBe('es')
    setNavigatorLanguages(['en-GB'])
    expect(detectLang()).toBe('en')
    setNavigatorLanguages(['fr-FR', 'es'])
    expect(detectLang()).toBe('es')
    setNavigatorLanguages(['de-DE'])
    expect(detectLang()).toBe('en')
  })

  it('prefers the saved choice, and ?lang= over both', () => {
    setNavigatorLanguages(['es-ES'])
    window.localStorage.setItem('qr.lang', 'en')
    expect(detectLang()).toBe('en')
    window.history.replaceState(null, '', '/?lang=es')
    expect(detectLang()).toBe('es')
  })

  it('survives a storage that throws (private mode, blocked cookies)', () => {
    setNavigatorLanguages(['en-US'])
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('denied', 'SecurityError')
    })
    expect(detectLang()).toBe('en')
  })
})

describe('translate', () => {
  it('fills placeholders in both languages', () => {
    expect(translate('es', 'err.rate_limited', { seconds: 42 })).toContain('42 segundos')
    expect(translate('en', 'err.rate_limited', { seconds: 42 })).toContain('42 seconds')
  })
})

describe('language switch', () => {
  const renderApp = () => {
    stubFetch((url) => (url.startsWith('/api/auth/') ? json({ error: { code: 'unauthenticated' } }, 401) : json({ hosts: ['fizsar.com'], port: 443 })))
    return render(
      <MemoryRouter>
        <LangProvider>
          <AuthProvider>
            <App />
          </AuthProvider>
        </LangProvider>
      </MemoryRouter>,
    )
  }

  it('switches the UI, sets <html lang> and remembers the choice', async () => {
    setNavigatorLanguages(['es-ES'])
    renderApp()
    expect(screen.getByRole('heading', { name: /preparada para la era cuántica/ })).toBeTruthy()
    expect(document.documentElement.lang).toBe('es')

    await userEvent.click(screen.getByRole('button', { name: 'EN' }))
    expect(screen.getByRole('heading', { name: /ready for the quantum era/ })).toBeTruthy()
    expect(document.documentElement.lang).toBe('en')
    expect(window.localStorage.getItem('qr.lang')).toBe('en')
    expect(screen.getByRole('button', { name: 'EN' }).getAttribute('aria-pressed')).toBe('true')
  })

  it('still switches when storage throws', async () => {
    setNavigatorLanguages(['es-ES'])
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('quota', 'QuotaExceededError')
    })
    renderApp()
    await userEvent.click(screen.getByRole('button', { name: 'EN' }))
    expect(screen.getByRole('heading', { name: /ready for the quantum era/ })).toBeTruthy()
  })
})
