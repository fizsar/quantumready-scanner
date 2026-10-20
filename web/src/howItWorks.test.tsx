import { render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'
import type { Lang } from './api'
import App from './App'
import { showcaseDict } from './i18n.showcase'
import { LangProvider } from './LangProvider'
import { json, stubFetch } from './test/fixtures'
import { forbiddenProse, strip } from './test/forbidden'

function renderAt(path: string, lang: Lang) {
  stubFetch(() => json({ hosts: ['fizsar.com'], port: 443 }))
  return render(
    <MemoryRouter initialEntries={[path]}>
      <LangProvider initial={lang}>
        <App />
      </LangProvider>
    </MemoryRouter>,
  )
}

describe('home: how it works', () => {
  for (const lang of ['es', 'en'] as const) {
    it(`sits under the scan form with the diagram, five points and a link (${lang})`, async () => {
      const { container } = renderAt('/', lang)
      const how = await screen.findByRole('region', { name: showcaseDict[lang]['how.title'] })
      // Under the form: the terminal section with the form comes first.
      const form = container.querySelector('form.scan-form')!
      expect(form.compareDocumentPosition(how) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
      // The /architecture diagram itself, in both layouts (wide and mobile).
      const svgs = how.querySelectorAll('svg.arch-diagram')
      expect([...svgs].map((s) => s.getAttribute('class'))).toEqual(['arch-diagram arch-diagram-wide', 'arch-diagram arch-diagram-tall'])
      expect(within(how).getAllByRole('img', { name: showcaseDict[lang]['arch.flow.svgTitle'] })).toHaveLength(2)
      expect(within(how).getAllByRole('listitem')).toHaveLength(5)
      expect(within(how).getByRole('link', { name: showcaseDict[lang]['how.more'] }).getAttribute('href')).toBe('/architecture')
    })

    it(`says nothing it must not (${lang})`, async () => {
      renderAt('/', lang)
      const how = await screen.findByRole('region', { name: showcaseDict[lang]['how.title'] })
      for (const re of forbiddenProse) expect(strip(how.textContent ?? ''), String(re)).not.toMatch(re)
      const all = Object.entries(showcaseDict[lang])
        .filter(([k]) => k.startsWith('how.'))
        .map(([, v]) => strip(v))
        .join('\n')
      for (const re of forbiddenProse) expect(all, String(re)).not.toMatch(re)
    })
  }

  it('the same component as /architecture, with ids unique on the page (CSP: no inline styles)', async () => {
    const { container, unmount } = renderAt('/', 'en')
    await screen.findByRole('region', { name: 'How it works' })
    expect(container.querySelectorAll('[style], style, script')).toHaveLength(0)
    const ids = [...container.querySelectorAll('[id]')].map((e) => e.id)
    expect(new Set(ids).size).toBe(ids.length)
    const homeSvg = container.querySelector('svg.arch-diagram-wide')!.innerHTML.replace(/id="[^"]*"|url\(#[^)]*\)|aria-[a-z]+="[^"]*"/g, '')
    unmount()
    const arch = renderAt('/architecture', 'en')
    await screen.findByRole('heading', { level: 1, name: 'Architecture' })
    const archSvg = arch.container.querySelector('svg.arch-diagram-wide')!.innerHTML.replace(/id="[^"]*"|url\(#[^)]*\)|aria-[a-z]+="[^"]*"/g, '')
    expect(homeSvg).toBe(archSvg)
  })
})
