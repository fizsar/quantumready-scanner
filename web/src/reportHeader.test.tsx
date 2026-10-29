import { render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Lang } from './api'
import App from './App'
import { LangProvider } from './LangProvider'
import { json, stubFetch } from './test/fixtures'
import exportEs from './test/lab/export.es.json'
import fizsarEn from './test/lab/fizsar.en.json'
import fizsarEs from './test/lab/fizsar.es.json'
import fizsarOld from './test/v2/fizsar-2026.2.es.json'

// The grade header (2.8.7 visual review): with model 2026.3, two cards of the
// same weight, each with its title, its value and its points as separate
// blocks. Before, the readiness card was three unstyled inline spans and
// read "Preparación cuánticaPreparado75 de 100 puntos".

function show(scan: unknown, lang: Lang) {
  stubFetch(() => json(scan))
  render(
    <MemoryRouter initialEntries={['/scans/abc']}>
      <LangProvider initial={lang}>
        <App />
      </LangProvider>
    </MemoryRouter>,
  )
}

afterEach(() => vi.restoreAllMocks())

/** The texts of a card, one per block, in order. */
function blocks(card: HTMLElement) {
  return [...card.children].filter((e) => e.tagName === 'P' && !e.classList.contains('sr-only')).map((e) => e.textContent)
}

describe('grade header, model 2026.3', () => {
  it('shows two named cards with title, value and points apart', async () => {
    show(fizsarEs, 'es')
    const summary = await screen.findByRole('region', { name: 'Nota' })
    const security = within(summary).getByRole('group', { name: 'Seguridad hoy' })
    const readiness = within(summary).getByRole('group', { name: 'Preparación cuántica' })
    expect(blocks(security)).toEqual(['Seguridad hoy', 'A', '100 de 100 puntos'])
    expect(blocks(readiness)).toEqual([
      'Preparación cuántica',
      'Preparado',
      '75 de 100 puntos',
      'Hoy lo habitual es como mucho 85: aún no hay certificados post-cuánticos en la web pública.',
    ])
    // The letter is read once, with its axis; the level is real text.
    expect(within(security).getByText('Seguridad hoy: A')).toBeTruthy()
    expect(within(security).getByText('A').getAttribute('aria-hidden')).toBe('true')
    expect(within(readiness).getByText('Preparado').getAttribute('aria-hidden')).toBeNull()
    // Same treatment: both values share the class that sizes them; the level is coloured by its code.
    expect(within(security).getByText('A').className).toContain('axis-card-value')
    expect(within(readiness).getByText('Preparado').className).toContain('axis-card-value')
    expect(readiness.className).toContain('readiness-ready')
    const bar = within(readiness).getByRole('meter', { name: 'Preparación cuántica' })
    expect(bar.getAttribute('value')).toBe('75')
    expect(within(security).getByRole('meter', { name: 'Seguridad hoy' }).getAttribute('value')).toBe('100')
  })

  it('in English, and with a cap', async () => {
    show(fizsarEn, 'en')
    const summary = await screen.findByRole('region', { name: 'Grade' })
    expect(blocks(within(summary).getByRole('group', { name: 'Quantum readiness' }))).toEqual([
      'Quantum readiness',
      'Ready',
      '75 out of 100 points',
      'Today 85 is usually the most: there are no post-quantum certificates on the public web yet.',
    ])
    vi.restoreAllMocks()
    document.body.innerHTML = ''
    show(exportEs, 'es')
    const capped = within(await screen.findByRole('region', { name: 'Nota' })).getByRole('group', { name: 'Seguridad hoy' })
    expect(blocks(capped)).toEqual(['Seguridad hoy', 'F', '35 de 100 puntos', 'Letra limitada a F (motivos debajo).'])
  })

  it('a scan judged with 2026.2 keeps one grade, no cards', async () => {
    show(fizsarOld, 'es')
    expect(await screen.findByText('Analizado con el modelo 2026.2.')).toBeTruthy()
    const summary = screen.getByRole('region', { name: 'Nota' })
    expect(within(summary).queryByRole('group')).toBeNull()
    expect(within(summary).getByText('Nota: A')).toBeTruthy()
    expect(within(summary).getByText('95 de 100 puntos')).toBeTruthy()
  })
})
