import { cleanup, render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it } from 'vitest'
import type { Lang } from './api'
import App from './App'
import { AuthProvider } from './AuthProvider'
import { LangProvider } from './LangProvider'
import { json } from './test/fixtures'
import { stubFetch } from './test/visitor'

function renderAt(path: string, lang: Lang) {
  stubFetch(() => json({ hosts: ['fizsar.com'], port: 443 }))
  return render(
    <MemoryRouter initialEntries={[path]}>
      <LangProvider initial={lang}>
        <AuthProvider>
          <App />
        </AuthProvider>
      </LangProvider>
    </MemoryRouter>,
  )
}

afterEach(cleanup)

describe('footer signature', () => {
  const cases: [string, Lang, string][] = [
    ['/', 'es', 'por Fiz Sar López'],
    ['/', 'en', 'by Fiz Sar López'],
    ['/architecture', 'en', 'by Fiz Sar López'],
    ['/historia', 'es', 'por Fiz Sar López'],
    ['/historia', 'en', 'by Fiz Sar López'],
  ]
  for (const [path, lang, text] of cases) {
    it(`${path} (${lang}): the FS mark and "${text}" link to fizsar.com, self-hosted, first in the footer`, async () => {
      renderAt(path, lang)
      const footer = await screen.findByRole('contentinfo')
      const link = within(footer).getByRole('link', { name: text })
      expect(link.getAttribute('href')).toBe('https://fizsar.com')
      expect(footer.querySelector('a')).toBe(link)
      const img = link.querySelector('img')!
      expect(img.getAttribute('src')).toBe('/fs-symbol.svg') // own asset: the CSP only allows 'self'
      expect(img.getAttribute('alt')).toBe('') // the text names the link
    })
  }

  it('keeps the rest of the footer', async () => {
    renderAt('/', 'en')
    const footer = await screen.findByRole('contentinfo')
    expect(within(footer).getByRole('link', { name: '← fizsar.com' })).toBeTruthy()
    expect(within(footer).getByRole('link', { name: 'Architecture' })).toBeTruthy()
  })
})
