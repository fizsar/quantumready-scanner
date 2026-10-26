import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Lang, ScanView } from './api'
import App from './App'
import { LangProvider } from './LangProvider'
import { json, stubFetch } from './test/fixtures'
import fizsarEn from './test/lab/fizsar.en.json'
import fizsarEs from './test/lab/fizsar.es.json'
import labEn from './test/lab/export.en.json'
import labEs from './test/lab/export.es.json'
import fizsarV2 from './test/v2/fizsar.result.json'
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
        <App />
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
