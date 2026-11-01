import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Lang, ScanView } from './api'
import App from './App'
import { AuthProvider } from './AuthProvider'
import { LangProvider } from './LangProvider'
import { json } from './test/fixtures'
import { stubFetch } from './test/visitor'
import fizsarEn from './test/lab/fizsar.en.json'
import fizsarEs from './test/lab/fizsar.es.json'
import labEn from './test/lab/export.en.json'
import labEs from './test/lab/export.es.json'
import fizsarV2 from './test/v2/fizsar.result.json'
import mldsaOnlyV2 from './test/v2/mldsa-only.result.json'
import tls12V2 from './test/v2/tls12.result.json'
import { expandOffer, groupName, isGrease, offeredVersions, signatureName, versionName, type ProbeLog, type ScanResult } from './underTheHood'

// Schema 2 results (milestone 2.8.4) written by the scanner itself: a real
// scan of fizsar.com and the lab's tls12 server (internal/scoring/testdata/lab_v2),
// put inside an API answer of the same shape as the v1 ones (fizsar.com's,
// and for tls12 the one of another lab server).
const withResult = (scan: unknown, result: unknown) => {
  const s = scan as ScanView & { report: Record<string, unknown> }
  return { ...s, report: { ...s.report, result } } as unknown as ScanView
}
const v2 = {
  fizsar: { es: withResult(fizsarEs, fizsarV2), en: withResult(fizsarEn, fizsarV2) },
  tls12: { es: withResult(labEs, tls12V2), en: withResult(labEn, tls12V2) },
}
const panelName = { es: 'Bajo el capó', en: 'Under the hood' }
const probesTitle = { es: 'Sondas, una a una', en: 'Probes, one by one' }

async function probeList(scan: ScanView, lang: Lang) {
  stubFetch(() => json(scan))
  render(
    <MemoryRouter initialEntries={[`/scans/${scan.id}`]}>
      <LangProvider initial={lang}>
        <AuthProvider>
          <App />
        </AuthProvider>
      </LangProvider>
    </MemoryRouter>,
  )
  await userEvent.click(await screen.findByRole('button', { name: panelName[lang] }))
  const section = within(screen.getByRole('region', { name: panelName[lang] })).getByRole('region', { name: probesTitle[lang] })
  return { section, items: within(section).getAllByRole('listitem') }
}

afterEach(() => vi.restoreAllMocks())

describe('Under the hood with the probe log (schema 2)', () => {
  it('lists every connection, in order, with the offer read from the bytes', async () => {
    const log = (fizsarV2 as unknown as ScanResult).probe_log!
    const { section, items } = await probeList(v2.fizsar.es, 'es')
    expect(items).toHaveLength(log.probes.length)
    expect(section.textContent).toContain(`Las ${log.probes.length} conexiones del escaneo`)
    const first = items[0].textContent ?? ''
    for (const s of ['crypto/tls', 'Oferta tipo navegador', 'TLS 1.3/TLS 1.2', 'grupos X25519MLKEM768, x25519, secp256r1, secp384r1', 'key share X25519MLKEM768, x25519', 'ServerHello', 'X25519MLKEM768']) {
      expect(first).toContain(s)
    }
    // The full offer, as sent: Go 1.27 offers ML-DSA in signature_algorithms.
    const details = within(items[0]).getByText('Oferta completa').closest('details')!
    expect(details.textContent).toContain('mldsa44')
    // v1 wording about raw probes kept only as findings is gone.
    expect(section.textContent).not.toContain('enumeran suites y grupos')
  })

  it('shows an alert with its code and level, and the stable error code', async () => {
    const { items } = await probeList(v2.tls12.es, 'es')
    const alert = items.find((li) => li.textContent?.includes('Solo TLS 1.3'))!
    expect(alert.textContent).toContain('Alerta 70 protocol_version')
    expect(alert.textContent).toContain('(fatal)')
    expect(alert.textContent).toContain('alert:70')
    expect(items.some((li) => li.textContent?.includes('en crudo'))).toBe(true)
  })

  it('in English', async () => {
    const { items } = await probeList(v2.tls12.en, 'en')
    expect(items[0].textContent).toContain('Browser-like offer')
    expect(items.find((li) => li.textContent?.includes('Only TLS 1.3'))?.textContent).toContain('(fatal)')
  })

  it('keeps v1 scans as they were (no log, the probes rebuilt from the facts)', async () => {
    const { section } = await probeList(fizsarEs as unknown as ScanView, 'es')
    expect(section.querySelector('details')).toBeNull()
    expect(section.textContent).not.toContain('conexiones del escaneo')
  })
})

describe('expandOffer', () => {
  it('expands an enumeration stored as differences, as the Go side does', () => {
    const log: ProbeLog = {
      offers: [
        { legacy_version: 0x0303, suites: [1, 2, 3, 4, 5], groups: [29, 23, 24] },
        { like: 0, suites_removed: [3] },
        { like: 1, suites_removed: [5] },
        { like: 0, groups_removed: [23] },
        { like: 3, suites_removed: [1] },
      ],
      probes: [],
    }
    expect(expandOffer(log, 2)).toEqual({ legacy_version: 0x0303, suites: [1, 2, 4], groups: [29, 23, 24] })
    expect(expandOffer(log, 4)).toEqual({ legacy_version: 0x0303, suites: [2, 3, 4, 5], groups: [29, 24] })
    expect(expandOffer({ offers: [{ like: 0 }], probes: [] }, 0)).toBeUndefined()
    expect(expandOffer(log, 9)).toBeUndefined()
  })

  it('every offer of the real logs expands, and each step removes only what it says', () => {
    for (const r of [fizsarV2, tls12V2] as unknown as ScanResult[]) {
      const log = r.probe_log!
      log.offers.forEach((o, i) => {
        const full = expandOffer(log, i)
        expect(full).toBeDefined()
        if (o.like !== undefined) {
          const base = expandOffer(log, o.like)!
          for (const s of o.suites_removed ?? []) expect(full!.suites).not.toContain(s)
          expect((full!.suites ?? []).length + (o.suites_removed ?? []).length).toBe((base.suites ?? []).length)
        }
      })
    }
    expect((tls12V2 as unknown as ScanResult).probe_log!.offers.some((o) => o.like !== undefined)).toBe(true)
  })
})

describe('names', () => {
  it('reads IANA code points and GREASE', () => {
    expect(isGrease(0x5a5a)).toBe(true)
    expect(isGrease(0x0a1a)).toBe(false)
    expect(groupName(0x11ec)).toBe('X25519MLKEM768')
    expect(groupName(0x6399)).toBe('X25519Kyber768Draft00')
    expect(groupName(0x9999)).toBe('0x9999')
    expect(signatureName(0x0905)).toBe('mldsa65')
    expect(signatureName(0xfafa)).toBe('GREASE')
    expect(versionName(0x0304)).toBe('TLS 1.3')
    expect(versionName(0x0002)).toBe('SSLv2')
    expect(offeredVersions({ legacy_version: 0x0303, supported_versions: [0x7a7a, 0x0304, 0x0303] })).toEqual([0x0304, 0x0303])
    expect(offeredVersions({ legacy_version: 0x0300 })).toEqual([0x0300])
  })
})

// Milestone 2.8.5: the lab's mldsa-only server (OpenSSL 3.5, an ML-DSA-65
// certificate and no classical one), as the scanner saw it.
describe('Under the hood: certificates by offer and the details 2.8.5 adds', () => {
  const panel = async (scan: ScanView) => {
    const { items } = await probeList(scan, 'es')
    return { items, region: screen.getByRole('region', { name: 'Bajo el capó' }) }
  }

  it('says which certificate the server presents with and without ML-DSA', async () => {
    const { items, region } = await panel(withResult(fizsarEs, mldsaOnlyV2))
    const certs = within(region).getByRole('region', { name: 'Certificado según la oferta' })
    expect(certs.textContent).toContain('Ofreciendo firmas ML-DSA, como Chrome: ML-DSA-65 (post-cuántico; OID 2.16.840.1.101.3.4.3.18)')
    expect(certs.textContent).toContain('Sin firmas ML-DSA, como Firefox o un cliente antiguo: el servidor rechazó la conexión')
    expect(items[0].textContent).toContain('certificado ML-DSA-65')
    const probe = items.find((li) => li.textContent?.includes('Certificado sin ML-DSA'))!
    expect(probe.textContent).toContain('Alerta 40 handshake_failure')
    expect(probe.textContent).toContain('(fatal)')
    expect(probe.textContent).toContain('alert:40')
  })

  it('an alert whose level is not known says so; a second ClientHello is shown', async () => {
    const r = structuredClone(fizsarV2) as unknown as ScanResult
    const log = r.probe_log!
    log.probes[0].response = { type: 'server_hello', version: 0x0304, alert: { code: 116, name: 'certificate_required', encrypted: true } }
    log.offers.push({ legacy_version: 0x0303, supported_versions: [0x0304], suites: [0x1301], groups: [0x001d, 0x0017], key_shares: [0x0017] })
    log.probes[1].response = { type: 'hello_retry_request', retry_group: 0x0017, retry_group_name: 'secp256r1', version: 0x0304 }
    log.probes[1].retry_offer = log.offers.length - 1
    const { items } = await panel(withResult(fizsarEs, r))
    expect(items[0].textContent).toContain('Alerta 116 certificate_required (nivel desconocido: alerta cifrada)')
    expect(items[1].textContent).toContain('segundo ClientHello con key share secp256r1')
    expect(within(items[1]).getByText('Segundo ClientHello (tras el HelloRetryRequest)')).toBeTruthy()
  })

  it('names the Chrome 156 probe (2.8.7)', async () => {
    const r = structuredClone(fizsarV2) as unknown as ScanResult
    r.probe_log!.probes[1].purpose = 'chrome'
    Object.assign(r.probe_log!.probes[2], { purpose: 'tls13_suite_order', subject: 'first' })
    Object.assign(r.probe_log!.probes[3], { purpose: 'tls13_suite_order', subject: 'reversed' })
    const { items } = await panel(withResult(fizsarEs, r))
    expect(items[1].textContent).toContain('Oferta de Chrome 156 (byte a byte)')
    expect(items[2].textContent).toContain('Orden de suites de TLS 1.3: en el orden encontrado')
    expect(items[3].textContent).toContain('Orden de suites de TLS 1.3: en orden inverso')
  })

  it('scans from before show no certificate section', async () => {
    const { region } = await panel(withResult(fizsarEs, fizsarV2))
    expect(within(region).queryByRole('region', { name: 'Certificado según la oferta' })).toBeNull()
  })
})
