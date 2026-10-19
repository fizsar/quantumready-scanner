import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { LangProvider } from './LangProvider'
import { resetEntryForTests, routeOf, track } from './metrics'
import { json, scanView, stubFetch } from './test/fixtures'
import fizsarEn from './test/lab/fizsar.en.json'

/** The events sent so far, parsed from the sendBeacon stub (test/setup.ts). */
async function sent(): Promise<Record<string, unknown>[]> {
  const beacon = navigator.sendBeacon as unknown as ReturnType<typeof vi.fn>
  return Promise.all(beacon.mock.calls.map(async ([url, blob]) => {
    expect(url).toBe('/api/e')
    return JSON.parse(await (blob as Blob).text())
  }))
}

function api() {
  return stubFetch(async (url, init) => {
    if (url === '/api/targets') return json({ hosts: ['fizsar.com', 'quantumready.fizsar.com'], port: 443 })
    if (url.startsWith('/api/scans?') && init?.method === 'POST') return json(scanView('queued', { host: 'fizsar.com' }), 202)
    return json(fizsarEn)
  })
}

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <LangProvider initial="es">
        <App />
      </LangProvider>
    </MemoryRouter>,
  )
}

beforeEach(() => resetEntryForTests())
afterEach(() => {
  vi.restoreAllMocks()
  Object.defineProperty(navigator, 'globalPrivacyControl', { value: undefined, configurable: true })
  Object.defineProperty(document, 'referrer', { value: '', configurable: true })
})

describe('M1 metrics', () => {
  it('counts a scan result as one bucket, never its id', () => {
    expect(routeOf('/')).toBe('/')
    expect(routeOf('/examples')).toBe('/examples')
    expect(routeOf('/architecture')).toBe('/architecture')
    expect(routeOf('/privacy')).toBe('/privacy')
    expect(routeOf('/scans/11111111-2222-3333-4444-555555555555')).toBe('scan_report')
    expect(routeOf('/admin')).toBe('not_found')
  })

  it('the first page view is the entry, with the referring site (host only)', async () => {
    Object.defineProperty(document, 'referrer', { value: 'https://www.linkedin.com/feed/?trk=x&id=123', configurable: true })
    api()
    renderAt('/')
    await screen.findByRole('heading', { level: 2 })
    await userEvent.click(screen.getAllByRole('link', { name: 'Ejemplos' })[0])
    await screen.findByRole('heading', { level: 1, name: 'Ejemplos' })
    const evs = await sent()
    expect(evs.filter((e) => e.e === 'page_view')).toEqual([
      { e: 'page_view', p: '/', entry: true, r: 'www.linkedin.com' },
      { e: 'page_view', p: '/examples' },
    ])
    expect(JSON.stringify(evs)).not.toContain('feed') // never the URL
  })

  it('no referrer from our own site', async () => {
    Object.defineProperty(document, 'referrer', { value: `${window.location.origin}/examples`, configurable: true })
    api()
    renderAt('/architecture')
    await screen.findByRole('heading', { level: 1 })
    expect((await sent())[0]).toEqual({ e: 'page_view', p: '/architecture', entry: true })
  })

  it('scan started, language switched, panel opened and JSON downloaded', async () => {
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    URL.createObjectURL = vi.fn(() => 'blob:x')
    URL.revokeObjectURL = vi.fn()
    api()
    renderAt('/')
    await userEvent.click(await screen.findByRole('button', { name: 'Escanear fizsar.com' }))
    await userEvent.click(screen.getByRole('button', { name: 'EN' }))
    await userEvent.click(screen.getByRole('button', { name: 'EN' })) // already English: not counted
    const toggle = await screen.findByRole('button', { name: 'Under the hood' })
    await userEvent.click(toggle)
    await userEvent.click(screen.getByRole('button', { name: 'Download JSON' }))
    const evs = (await sent()).filter((e) => e.e !== 'page_view')
    expect(evs).toEqual([
      { e: 'scan_started', d: 'fizsar.com' },
      { e: 'lang_switch', d: 'en' },
      { e: 'under_the_hood_open', d: 'scan_report' },
      { e: 'json_download', d: 'scan_report' },
    ])
  })

  it('sends nothing with Global Privacy Control or under automation', async () => {
    Object.defineProperty(navigator, 'globalPrivacyControl', { value: true, configurable: true })
    track({ e: 'lang_switch', d: 'en' })
    Object.defineProperty(navigator, 'globalPrivacyControl', { value: undefined, configurable: true })
    Object.defineProperty(navigator, 'webdriver', { value: true, configurable: true })
    track({ e: 'lang_switch', d: 'en' })
    Object.defineProperty(navigator, 'webdriver', { value: false, configurable: true })
    expect(await sent()).toEqual([])
    track({ e: 'lang_switch', d: 'es' })
    expect(await sent()).toEqual([{ e: 'lang_switch', d: 'es' }])
  })

  it('falls back to fetch keepalive, and a failure never breaks the page', async () => {
    Object.defineProperty(navigator, 'sendBeacon', { value: undefined, configurable: true, writable: true })
    const fetch = stubFetch(() => Promise.reject(new Error('offline')))
    expect(() => track({ e: 'lang_switch', d: 'en' })).not.toThrow()
    const [url, init] = fetch.mock.calls[0]
    expect([url, init?.method, init?.keepalive, init?.body]).toEqual(['/api/e', 'POST', true, '{"e":"lang_switch","d":"en"}'])
  })
})
