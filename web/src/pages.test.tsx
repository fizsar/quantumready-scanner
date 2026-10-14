import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'
import type { Lang } from './api'
import App from './App'
import { LangProvider } from './LangProvider'
import { json, reportA, reportD, scanView, stubFetch } from './test/fixtures'

function renderAt(path: string, lang: Lang = 'es') {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <LangProvider initial={lang}>
        <App />
      </LangProvider>
    </MemoryRouter>,
  )
}

describe('result page', () => {
  it('done, grade A: plain-language verdict, grade, PQC, axes, breakdown, footer', async () => {
    stubFetch(() => json(scanView('done', { report: reportA, finished_at: '2026-10-13T17:11:16Z' })))
    renderAt('/scans/abc')

    expect(await screen.findByRole('heading', { level: 1, name: /ya usa cifrado post-cuántico híbrido/ })).toBeTruthy()
    expect(screen.getByText('Nota: A')).toBeTruthy()
    expect(screen.getByText('95 de 100 puntos')).toBeTruthy()
    expect(screen.getByText('PQC: sí')).toBeTruthy()
    expect(screen.getByText('Grupo negociado: X25519MLKEM768')).toBeTruthy()

    const quantum = screen.getByRole('region', { name: /Riesgo cuántico/ })
    const titles = within(quantum).getAllByText(/Firma clásica|Un solo grupo|Intercambio de claves post-cuántico:/)
    expect(titles.map((t) => t.textContent)).toEqual([
      'Firma clásica (ECDSA P-256): riesgo cuántico futuro',
      'Un solo grupo híbrido',
      'Intercambio de claves post-cuántico: X25519MLKEM768',
    ]) // risks by severity, then what is done right
    expect(within(quantum).getByText('Añade SecP256r1MLKEM768.', { exact: false })).toBeTruthy() // remediation inside <details>

    // "Correcto" findings are grouped apart, after the risks.
    const good = within(quantum).getByRole('group', { name: 'Lo que está bien' })
    expect(within(good).getByText('Intercambio de claves post-cuántico: X25519MLKEM768')).toBeTruthy()
    expect(within(good).queryByText('Un solo grupo híbrido')).toBeNull()
    const current = screen.getByRole('region', { name: /Riesgo actual/ })
    expect(within(current).getByText('Nada que corregir en este eje.')).toBeTruthy()
    expect(within(within(current).getByRole('group', { name: 'Lo que está bien' })).getByText('Certificado válido')).toBeTruthy()

    const breakdown = screen.getByRole('region', { name: 'Desglose de la nota' })
    expect(within(breakdown).getAllByRole('meter')).toHaveLength(5)
    expect(within(breakdown).queryByText(/Ajuste: nada roto/)).toBeNull()

    expect(screen.getByRole('region', { name: 'No evaluado' })).toBeTruthy()
    expect(screen.getByText('Modelo de puntuación 2026.1')).toBeTruthy()
    expect(screen.getByText(/Escaneado el 13 de octubre de 2026/)).toBeTruthy()
    // The title is set in an effect after the report renders: wait for it (flaky under CI load otherwise).
    await waitFor(() => expect(document.title).toBe('Resultado de fizsar.com · QuantumReady'))
    expect(document.head.querySelector('meta[name="robots"]')?.getAttribute('content')).toBe('noindex, nofollow')
  })

  it('done, grade D: future-risk verdict and the floor adjustment', async () => {
    stubFetch(() => json(scanView('done', { report: reportD })))
    renderAt('/scans/abc')
    expect(await screen.findByRole('heading', { level: 1, name: /segura hoy, pero el tráfico/ })).toBeTruthy()
    expect(screen.getByText('Nota: D')).toBeTruthy()
    expect(screen.getByText('PQC: no')).toBeTruthy()
    const breakdown = screen.getByRole('region', { name: 'Desglose de la nota' })
    expect(within(breakdown).getByText(/Ajuste: nada roto \(mínimo D\)/)).toBeTruthy()
    expect(within(breakdown).getByText('+10')).toBeTruthy()
  })

  it('renders in English', async () => {
    stubFetch(() => json(scanView('done', { report: reportA, lang: 'en' })))
    renderAt('/scans/abc', 'en')
    expect(await screen.findByRole('heading', { level: 1, name: /already uses hybrid post-quantum encryption/ })).toBeTruthy()
    expect(screen.getByText('95 out of 100 points')).toBeTruthy()
    const quantum = screen.getByRole('region', { name: /Quantum risk/ })
    expect(within(quantum).getByRole('group', { name: "What's working well" })).toBeTruthy()
    expect(within(screen.getByRole('region', { name: /Current risk/ })).getByText('Nothing to fix on this axis.')).toBeTruthy()
  })

  it('failed: friendly failure with the reason', async () => {
    stubFetch(() => json(scanView('failed', { error: { code: 'connection_refused', message: 'El servidor rechazó la conexión en el puerto 443.' } })))
    renderAt('/scans/abc')
    expect(await screen.findByRole('heading', { name: 'No hemos podido completar el escaneo' })).toBeTruthy()
    expect(screen.getByRole('alert').textContent).toBe('El servidor rechazó la conexión en el puerto 443.')
    expect(screen.getByRole('link', { name: 'Probar otra vez' })).toBeTruthy()
  })

  it('in progress: an aria-live status', async () => {
    stubFetch(() => json(scanView('running')))
    renderAt('/scans/abc')
    const status = await screen.findByRole('status')
    expect(status.getAttribute('aria-live')).toBe('polite')
    expect(status.textContent).toContain('Escaneando fizsar.com')
  })

  it('unknown scan: not found message', async () => {
    stubFetch(() => json({ error: { code: 'not_found', message: 'No encontrado.' } }, 404))
    renderAt('/scans/nope')
    expect((await screen.findByRole('alert')).textContent).toContain('No encontramos ese escaneo')
  })
})

// Real TLS lab scans scored with model 2026.2, as the API returns them
// (internal/httpapi TestLabFixtures writes test/lab/*.json).
describe('result page with lab scans (model 2026.2)', () => {
  it.each([
    ['export', 'es', /acepta conexiones que no cifran nada/, 'F', '20 de 100 puntos', /Diffie-Hellman demasiado débil \(512 bits\)/],
    ['export', 'en', /accepts connections that encrypt nothing/, 'F', '20 out of 100 points', /Diffie-Hellman far too weak \(512 bits\)/],
    ['kyber', 'es', /versión experimental del cifrado post-cuántico/, 'D', '45 de 100 puntos', /Solo PQC experimental: X25519Kyber768Draft00/],
    ['kyber', 'en', /experimental version of post-quantum encryption/, 'D', '45 out of 100 points', /Experimental PQC only: X25519Kyber768Draft00/],
  ] as const)('%s (%s)', async (server, lang, verdict, grade, score, finding) => {
    const view = (await import(`./test/lab/${server}.${lang}.json`)).default
    stubFetch(() => json(view))
    renderAt('/scans/abc', lang)
    expect(await screen.findByRole('heading', { level: 1, name: verdict })).toBeTruthy()
    expect(screen.getByText(`${lang === 'es' ? 'Nota' : 'Grade'}: ${grade}`)).toBeTruthy()
    expect(screen.getByText(score)).toBeTruthy()
    expect(screen.getAllByText(finding).length).toBeGreaterThan(0)
    expect(screen.queryByText(/\{[a-z]+\}/)).toBeNull() // no unfilled placeholder anywhere
  })
})

describe('home page', () => {
  it('lists the allowed domains from the API and starts a scan', async () => {
    const fetch = stubFetch((url, init) => {
      if (url === '/api/targets') return json({ hosts: ['fizsar.com', 'quantumready.fizsar.com'], port: 443 })
      if (url.startsWith('/api/scans?') && init?.method === 'POST') return json(scanView('queued'), 202)
      return json(scanView('queued'))
    })
    renderAt('/')
    await userEvent.click(await screen.findByRole('button', { name: 'Escanear quantumready.fizsar.com' }))
    expect(await screen.findByRole('status')).toBeTruthy() // navigated to the result page
    const post = fetch.mock.calls.find(([, init]) => init?.method === 'POST')!
    expect(JSON.parse(String(post[1]!.body))).toEqual({ host: 'quantumready.fizsar.com' })
  })

  it('429: tells when to retry from Retry-After', async () => {
    stubFetch((url) =>
      url === '/api/targets'
        ? json({ hosts: ['fizsar.com'], port: 443 })
        : json({ error: { code: 'rate_limited', message: 'Demasiados escaneos.' } }, 429, { 'Retry-After': '42' }),
    )
    renderAt('/')
    await userEvent.click(await screen.findByRole('button', { name: 'Escanear fizsar.com' }))
    expect((await screen.findByRole('alert')).textContent).toBe(
      'Has hecho muchos escaneos seguidos. Podrás volver a intentarlo en 42 segundos.',
    )
  })

  it.each([
    [400, 'port_not_allowed', 'Por ahora solo se escanea el puerto 443'],
    [400, 'invalid_host', 'Ese dominio no parece válido'],
    [403, 'host_not_allowed', 'solo se pueden escanear los dominios de demostración'],
    [413, 'body_too_large', 'El texto es demasiado largo'],
  ])('%i %s: friendly message', async (status, code, text) => {
    stubFetch((url) => (url === '/api/targets' ? json({ hosts: ['fizsar.com'], port: 443 }) : json({ error: { code, message: 'x' } }, status)))
    renderAt('/')
    await userEvent.type(await screen.findByLabelText('Dominio'), 'example.com:8443')
    await userEvent.click(screen.getByRole('button', { name: 'Escanear' }))
    expect((await screen.findByRole('alert')).textContent).toContain(text)
    expect(screen.getByLabelText('Dominio').getAttribute('aria-invalid')).toBe('true')
  })
})
