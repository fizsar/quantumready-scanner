import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Lang, ScanView } from './api'
import App from './App'
import { AuthProvider } from './AuthProvider'
import { LangProvider } from './LangProvider'
import { json, scanView } from './test/fixtures'
import { stubFetch } from './test/visitor'
import fizsarEn from './test/lab/fizsar.en.json'
import fizsarEs from './test/lab/fizsar.es.json'
import kyberEs from './test/lab/kyber.es.json'
import fizsarOld from './test/v2/fizsar-2026.2.es.json'

// Model 2026.3 in the report: two axes, Shor and Grover apart. And a scan
// judged with 2026.2 (internal/httpapi/testdata/apijson/fizsar.com-2026.2):
// its own judgement, the notice and the button to run it again.

function show(scan: unknown, lang: Lang) {
  render(
    <MemoryRouter initialEntries={['/scans/abc']}>
      <LangProvider initial={lang}>
        <AuthProvider>
          <App />
        </AuthProvider>
      </LangProvider>
    </MemoryRouter>,
  )
  return scan as ScanView
}

afterEach(() => vi.restoreAllMocks())

describe('report, model 2026.3', () => {
  it('shows security today and quantum readiness, each with its breakdown', async () => {
    stubFetch(() => json(fizsarEs))
    show(fizsarEs, 'es')
    const summary = await screen.findByRole('region', { name: 'Nota' })
    expect(summary.textContent).toContain('Seguridad hoy')
    expect(summary.textContent).toContain('Preparación cuántica')
    expect(summary.textContent).toContain('Preparado')
    expect(within(summary).getByText('Seguridad hoy: A')).toBeTruthy()
    expect(within(summary).getByText('100 de 100 puntos')).toBeTruthy()
    expect(within(summary).getByText('75 de 100 puntos')).toBeTruthy()
    const breakdown = screen.getByRole('region', { name: 'Desglose de la nota' })
    for (const label of ['TLS 1.3', 'Intercambio de claves post-cuántico', 'Renovación automatizada del certificado', 'Certificado post-cuántico (ML-DSA, SLH-DSA)']) {
      expect(within(breakdown).getAllByText(label).length).toBeGreaterThan(0)
    }
    expect(breakdown.textContent).toContain('lo habitual es como mucho 85')
    expect(breakdown.textContent).not.toContain('nada roto (mínimo D)') // no floor
    expect(screen.queryByRole('button', { name: 'Repetir el análisis' })).toBeNull()
  })

  it('separates Shor and Grover in the quantum risk', async () => {
    stubFetch(() => json(fizsarEn))
    show(fizsarEn, 'en')
    const quantum = (await screen.findByRole('heading', { name: /Quantum risk/ })).closest('section')!
    const shor = within(quantum).getByRole('heading', { name: 'Shor: key exchange and signatures' })
    const grover = within(quantum).getByRole('heading', { name: 'Grover: symmetric encryption (informational)' })
    expect(shor.compareDocumentPosition(grover) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    // AES-128 under Grover, after its heading; the hybrid group under Shor.
    const aes = within(quantum).getByText(/AES-128 and Grover/)
    expect(grover.compareDocumentPosition(aes) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('shows the Kyber penalty as negative points', async () => {
    stubFetch(() => json(kyberEs))
    show(kyberEs, 'es')
    const breakdown = await screen.findByRole('region', { name: 'Desglose de la nota' })
    const row = within(breakdown).getByText('Kyber obsoleto aceptado (penalización)').closest('li')!
    expect(row.textContent).toContain('-15')
    expect(row.querySelector('meter')).toBeNull()
  })
})

describe('a scan judged with model 2026.2', () => {
  it('keeps its single grade, says so and offers to run it again', async () => {
    const fetch = stubFetch((url, init) => {
      if (url.startsWith('/api/scans?') && init?.method === 'POST') return json(scanView('queued'), 202)
      if (url.startsWith('/api/scans/abc')) return json(fizsarOld)
      return json(scanView('queued'))
    })
    show(fizsarOld, 'es')
    expect(await screen.findByText('Analizado con el modelo 2026.2.')).toBeTruthy()
    expect(screen.getByText('Nota: A')).toBeTruthy()
    expect(screen.getByText('95 de 100 puntos')).toBeTruthy()
    expect(screen.queryByText('Preparación cuántica')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Repetir el análisis' }))
    const post = fetch.mock.calls.find(([, init]) => init?.method === 'POST')!
    expect(JSON.parse(String(post[1]!.body))).toEqual({ host: 'fizsar.com' })
    expect(await screen.findByRole('status')).toBeTruthy() // navigated to the new scan, queued
  })

  it('says so if the new scan could not be started', async () => {
    stubFetch((_url, init) => {
      if (init?.method === 'POST') return json({ error: { code: 'internal', message: 'x' } }, 500)
      return json(fizsarOld)
    })
    show(fizsarOld, 'es')
    await userEvent.click(await screen.findByRole('button', { name: 'Repetir el análisis' }))
    expect(await screen.findByText(/No se pudo repetir el análisis/)).toBeTruthy()
  })
})
