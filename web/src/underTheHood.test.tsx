import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Lang, ScanView } from './api'
import App from './App'
import { LangProvider } from './LangProvider'
import { json, reportA, scanView, stubFetch } from './test/fixtures'
import exportEn from './test/lab/export.en.json'
import exportEs from './test/lab/export.es.json'
import fizsarEn from './test/lab/fizsar.en.json'
import fizsarEs from './test/lab/fizsar.es.json'
import kyberEn from './test/lab/kyber.en.json'
import kyberEs from './test/lab/kyber.es.json'
import { otherRawProbes, parseError, probeRows, resultOf, type ScanResult } from './underTheHood'

// Real scans: fizsar.com as the scanner saw it, and two servers of the
// isolated TLS lab (test/tlslab), through the API (internal/httpapi/fixtures_test.go).
const scans = {
  fizsar: { es: fizsarEs, en: fizsarEn },
  export: { es: exportEs, en: exportEn },
  kyber: { es: kyberEs, en: kyberEn },
} as unknown as Record<'fizsar' | 'export' | 'kyber', Record<Lang, ScanView>>

const titles = {
  es: { panel: 'Bajo el capó', sections: ['Escaneo', 'Versiones de TLS', 'Grupos de intercambio de claves', 'Suites de cifrado aceptadas', 'Sondas, una a una'] },
  en: { panel: 'Under the hood', sections: ['Scan', 'TLS versions', 'Key exchange groups', 'Accepted cipher suites', 'Probes, one by one'] },
}

async function openPanel(scan: ScanView, lang: Lang) {
  stubFetch(() => json(scan))
  render(
    <MemoryRouter initialEntries={[`/scans/${scan.id}`]}>
      <LangProvider initial={lang}>
        <App />
      </LangProvider>
    </MemoryRouter>,
  )
  const toggle = await screen.findByRole('button', { name: titles[lang].panel })
  await userEvent.click(toggle)
  return screen.getByRole('region', { name: titles[lang].panel })
}

afterEach(() => vi.restoreAllMocks())

describe('Under the hood panel', () => {
  it('is closed by default and opens and closes from the keyboard', async () => {
    const user = userEvent.setup()
    stubFetch(() => json(scans.fizsar.es))
    render(
      <MemoryRouter initialEntries={['/scans/x']}>
        <LangProvider initial="es">
          <App />
        </LangProvider>
      </MemoryRouter>,
    )
    const toggle = await screen.findByRole('button', { name: 'Bajo el capó' })
    const body = document.getElementById(toggle.getAttribute('aria-controls') ?? '')!
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    expect(body.hidden).toBe(true)

    while (document.activeElement !== toggle) await user.tab() // reachable with Tab
    await user.keyboard('{Enter}')
    expect(toggle.getAttribute('aria-expanded')).toBe('true')
    expect(body.hidden).toBe(false)
    await user.keyboard(' ')
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    expect(body.hidden).toBe(true)
  })

  it('opens on load when the link points at it', async () => {
    stubFetch(() => json(scans.fizsar.en))
    render(
      <MemoryRouter initialEntries={['/scans/x#under-the-hood']}>
        <LangProvider initial="en">
          <App />
        </LangProvider>
      </MemoryRouter>,
    )
    const toggle = await screen.findByRole('button', { name: 'Under the hood' })
    expect(toggle.getAttribute('aria-expanded')).toBe('true')
    expect(toggle.closest('section')?.id).toBe('under-the-hood')
  })

  for (const name of ['fizsar', 'export', 'kyber'] as const) {
    for (const lang of ['es', 'en'] as const) {
      it(`${name} (${lang}): every section renders from report.result`, async () => {
        const scan = scans[name][lang]
        const r = resultOf(scan.report!)!
        const panel = await openPanel(scan, lang)
        for (const s of titles[lang].sections) expect(within(panel).getByRole('region', { name: s }), s).toBeTruthy()

        const facts = within(panel).getByRole('region', { name: titles[lang].sections[0] })
        expect(within(facts).getAllByText(r.ip).length).toBeGreaterThan(0) // scanned (and resolved)
        expect(within(facts).getByText(String(r.connections))).toBeTruthy()

        const groups = within(panel).getByRole('region', { name: titles[lang].sections[2] })
        expect(within(groups).getAllByRole('row')).toHaveLength((r.key_exchange.groups ?? []).length + 1)

        const probes = within(panel).getByRole('region', { name: titles[lang].sections[4] })
        expect(within(probes).getAllByRole('listitem')).toHaveLength(probeRows(r).length)

        // Nothing beyond the response: no scan id, no stack traces or settings.
        expect(panel.textContent).not.toContain(scan.id)
        expect(panel.textContent).not.toMatch(/goroutine|\.go:\d|DATABASE_URL|worker|lease/i)
      })
    }
  }

  it('fizsar.com (A): ServerHello with the hybrid group, alerts with code and name', async () => {
    const panel = await openPanel(scans.fizsar.en, 'en')
    expect(scans.fizsar.en.report!.grade).toBe('A')
    const probes = within(panel).getByRole('region', { name: 'Probes, one by one' })
    const first = within(probes).getAllByRole('listitem')[0]
    expect(first.textContent).toContain('ServerHello · TLS 1.3 · TLS_AES_128_GCM_SHA256 · X25519MLKEM768')
    expect(within(probes).getAllByText('Alert 70 protocol_version').length).toBeGreaterThan(0) // TLS 1.0/1.1 and SSLv3
    expect(within(probes).getAllByText('Alert 40 handshake_failure').length).toBeGreaterThan(0) // unsupported groups
    // The response names the alerts; the codes come from the IANA registry, and the panel says so.
    const alerts = within(probes).getAllByText(/^Alert \d+ /)
    expect(alerts.every((a) => a.nextElementSibling?.textContent === ' (code per IANA)')).toBe(true)
    expect(within(panel).getByText('Accepted: TLS 1.2, TLS 1.3')).toBeTruthy()
    expect(within(panel).getByText('Chosen by the server: X25519MLKEM768')).toBeTruthy()
  })

  it('export (F): alerts and closes by name, the weak suites and the DH prime', async () => {
    const panel = await openPanel(scans.export.es, 'es')
    expect(scans.export.es.report!.grade).toBe('F')
    const probes = within(panel).getByRole('region', { name: 'Sondas, una a una' })
    expect(within(probes).getAllByText('Alerta 40 handshake_failure').length).toBeGreaterThan(0)
    expect(within(probes).getByText('Cierre de la conexión')).toBeTruthy() // SSLv2: EOF
    expect(within(probes).getAllByText('(código según IANA)', { exact: false }).length).toBeGreaterThan(0)
    const suites = within(panel).getByRole('region', { name: 'Suites de cifrado aceptadas' })
    expect(within(suites).getByText('TLS_RSA_EXPORT_WITH_RC4_40_MD5')).toBeTruthy()
    expect(within(suites).getAllByText(/EXPORT · 40 bits/).length).toBeGreaterThan(0)
    expect(within(suites).getAllByText(/sin cifrado/).length).toBeGreaterThan(0)
    expect(within(suites).getAllByText(/primo de \d+ bits/).length).toBeGreaterThan(0)
  })

  it('kyber: the experimental group and its replies', async () => {
    const scan = scans.kyber.en
    const panel = await openPanel(scan, 'en')
    expect(scan.report!.grade).toBe('D')
    const r = resultOf(scan.report!) as ScanResult
    const groups = within(panel).getByRole('region', { name: 'Key exchange groups' })
    for (const g of r.key_exchange.groups ?? []) expect(within(groups).getByText(g.name)).toBeTruthy()
    expect(within(panel).getAllByText(/^Alert \d+ [a-z_]+$/).length).toBeGreaterThan(0)
  })

  it('downloads exactly the API response', async () => {
    let saved: Blob | undefined
    const create = vi.fn((b: Blob) => {
      saved = b
      return 'blob:test'
    })
    vi.stubGlobal('URL', Object.assign(URL, { createObjectURL: create, revokeObjectURL: vi.fn() }))
    let name = ''
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
      name = this.download
    })
    const scan = scans.export.en
    const panel = await openPanel(scan, 'en')
    await userEvent.click(within(panel).getByRole('button', { name: 'Download JSON' }))
    expect(click).toHaveBeenCalledOnce()
    expect(name).toBe(`quantumready-export.tlslab.test-${scan.id.slice(0, 8)}.json`)
    expect(saved?.type).toBe('application/json')
    expect(JSON.parse(await saved!.text())).toEqual(scan)
  })

  it('is absent when the response has no scanner facts', async () => {
    stubFetch(() => json(scanView('done', { report: reportA })))
    render(
      <MemoryRouter initialEntries={['/scans/x']}>
        <LangProvider initial="es">
          <App />
        </LangProvider>
      </MemoryRouter>,
    )
    await screen.findByText('Nota: A')
    expect(screen.queryByRole('button', { name: 'Bajo el capó' })).toBeNull()
  })
})

describe('raw probes in the panel', () => {
  it('lists the raw version probes oldest first and counts the rest exactly', async () => {
    const r = resultOf(scans.fizsar.en.report!)!
    const raw = probeRows(r).filter((p) => p.source === 'raw').map((p) => (p.offer.kind === 'version' ? p.offer.version : ''))
    expect(raw).toEqual(['SSLv2', 'SSLv3', 'TLS 1.0', 'TLS 1.1'])
    expect(r.raw!.probes).toBe(11)
    expect(otherRawProbes(r)).toBe(7) // 11 connections, 4 listed one by one

    const panel = await openPanel(scans.fizsar.en, 'en')
    expect(within(panel).getByText('The other 7 raw probes enumerate suites and groups; the API keeps what they found, in the tables above.')).toBeTruthy()
  })

  it('export: 38 raw connections, 5 listed', async () => {
    const r = resultOf(scans.export.es.report!)!
    expect(otherRawProbes(r)).toBe(r.raw!.probes - 2 - (r.raw!.versions ?? []).length)
    expect(otherRawProbes(r)).toBe(33)
    const panel = await openPanel(scans.export.es, 'es')
    expect(within(panel).getByText(/^Las otras 33 sondas en crudo/)).toBeTruthy()
  })

  it('a probe the budget refused made no connection; numbers that do not add up get no number', () => {
    const base = resultOf(scans.fizsar.en.report!)!
    const budget = 'scanner: raw probe budget exhausted'
    const refused = { ...base, raw: { ...base.raw!, probes: 3, versions: [{ version: 'TLS 1.0', id: 769, status: 'error' as const, error: budget }] } }
    expect(otherRawProbes(refused)).toBe(1) // 3 connections: SSLv2, SSLv3 and one more; TLS 1.0 was never sent
    const odd = { ...base, raw: { ...base.raw!, probes: 2 } } // fewer connections than rows listed
    expect(otherRawProbes(odd)).toBeUndefined()
  })
})

describe('parseError', () => {
  it('reads alerts, closes and timeouts as the scanner writes them', () => {
    // Named alerts get the IANA code for the name (byName); alert(N) carries the real code.
    expect(parseError('remote error: tls: handshake failure')).toEqual({ kind: 'alert', code: 40, name: 'handshake_failure', byName: true })
    expect(parseError('remote error: tls: protocol version not supported')).toEqual({ kind: 'alert', code: 70, name: 'protocol_version', byName: true })
    expect(parseError('alert protocol_version')).toEqual({ kind: 'alert', code: 70, name: 'protocol_version', byName: true })
    expect(parseError('alert(41)')).toEqual({ kind: 'alert', code: 41, name: 'unknown' })
    expect(parseError('remote error: tls: something new')).toEqual({ kind: 'alert', name: 'something new' })
    expect(parseError('EOF')).toEqual({ kind: 'closed' })
    expect(parseError('read tcp 10.0.0.1:1->10.0.0.2:443: i/o timeout')).toEqual({ kind: 'timeout' })
    expect(parseError('server answered SSLv3')).toEqual({ kind: 'other', text: 'server answered SSLv3' })
    expect(parseError(undefined)).toEqual({ kind: 'refused' })
  })
})
