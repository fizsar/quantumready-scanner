import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Domain, Me, Org, ScanSummary } from './api'
import { json, scanView, stubFetch } from './test/fixtures'
import { renderAt } from './test/render'

const me: Me = {
  user: { id: 'u1', email: 'ana@example.com', lang: 'es' },
  organizations: [{ id: 'oA', name: 'Acme', role: 'owner' }],
  active_org_id: 'oA',
  csrf_token: 'csrf-1',
  can_create_organizations: true,
}
const owner: Org = {
  id: 'oA', name: 'Acme', role: 'owner',
  permissions: ['view', 'rename', 'invite', 'revoke_invitation', 'change_role', 'remove_member', 'manage_domains'],
}
const viewer: Org = { id: 'oA', name: 'Acme', role: 'viewer', permissions: ['view'] }
const pendingDomain: Domain = {
  id: 'd1', name: 'fizsar.com', status: 'pending', verified: false, check_pending: false,
  record: { type: 'TXT', name: '_quantumready.fizsar.com', value: 'qr-verify=abcdefghijklmnopqrstuvwxyz' },
}
const verifiedDomain: Domain = { ...pendingDomain, status: 'verified', verified: true, last_checked_at: '2026-10-16T10:00:00Z', verified_at: '2026-10-16T10:00:00Z' }
const history: ScanSummary[] = [
  { id: 's1', host: 'quantumready.fizsar.com', status: 'done', intensity: 'full', created_at: '2026-10-15T09:00:00Z', score: 95, grade: 'A' },
  { id: 's2', host: 'fizsar.com', status: 'queued', intensity: 'full', created_at: '2026-10-16T09:00:00Z' },
]

type Handler = (url: string, init?: RequestInit) => Response | undefined
function api(org: Org, domains: Domain[], extra: Handler = () => undefined, scans: ScanSummary[] = []) {
  return stubFetch((url, init) => {
    const r = extra(url, init)
    if (r) return r
    const method = init?.method ?? 'GET'
    if (url.startsWith('/api/auth/me')) return json(me)
    if (url.startsWith('/api/org?')) return json(org)
    if (url.startsWith('/api/org/members')) return json({ members: [] })
    if (url.startsWith('/api/org/invitations') && method === 'GET') return json({ invitations: [] })
    if (url.startsWith('/api/org/domains?') && method === 'GET') return json({ domains })
    if (url.startsWith('/api/org/scans?') && method === 'GET') return json({ scans })
    return new Response(null, { status: 204 })
  })
}

function sent(fetch: ReturnType<typeof stubFetch>, method: string, prefix: string) {
  const c = fetch.mock.calls.find(([u, i]) => String(u).startsWith(prefix) && (i?.method ?? 'GET') === method)
  expect(c, `${method} ${prefix}`).toBeTruthy()
  return { body: c![1]!.body ? JSON.parse(String(c![1]!.body)) : undefined, headers: c![1]!.headers as Record<string, string> }
}

beforeEach(() => {
  vi.spyOn(window, 'confirm').mockReturnValue(true)
})

describe('domains', () => {
  it('shows the TXT record and checks it, waiting for the worker', async () => {
    let checks = 0
    const fetch = api(owner, [pendingDomain], (url, init) => {
      if (url.startsWith('/api/org/domains/d1/verify')) return json({ ...pendingDomain, check_pending: true }, 202)
      if (url.startsWith('/api/org/domains/d1?') && (init?.method ?? 'GET') === 'GET') return json(++checks < 2 ? { ...pendingDomain, check_pending: true } : verifiedDomain)
      return undefined
    })
    renderAt('/org')
    const item = (await screen.findByText('fizsar.com')).closest('li')!
    expect(within(item).getByText('sin verificar')).toBeTruthy()
    expect(within(item).getByText('_quantumready.fizsar.com')).toBeTruthy()
    expect(within(item).getByText('qr-verify=abcdefghijklmnopqrstuvwxyz')).toBeTruthy()
    await userEvent.click(within(item).getByRole('button', { name: 'Comprobar ahora' }))
    expect(sent(fetch, 'POST', '/api/org/domains/d1/verify').headers['X-QR-CSRF']).toBe('csrf-1')
    expect(await within(item).findByText('Comprobando el DNS…')).toBeTruthy()
    await waitFor(() => expect(within(item).getByText('verificado')).toBeTruthy(), { timeout: 6000 })
    expect(within(item).queryByRole('button', { name: 'Comprobar ahora' })).toBeNull()
    expect(within(item).getByText(/lo comprobamos cada semana/)).toBeTruthy()
  }, 10000)

  it('the record was not found: the reason is shown', async () => {
    api(owner, [{ ...pendingDomain, last_check_error: { code: 'dns_record_missing', message: 'No encontramos el registro TXT de verificación.' } }])
    renderAt('/org')
    expect((await screen.findByText(/No encontramos el registro TXT/)).getAttribute('role')).toBe('alert')
  })

  it('adds a domain; the API explains a refusal', async () => {
    let first = true
    const fetch = api(owner, [], (url, init) => {
      if (!url.startsWith('/api/org/domains?') || init?.method !== 'POST') return undefined
      if (first) return ((first = false), json({ error: { code: 'domain_public_suffix', message: 'Es un sufijo público (como .com o .co.uk).' } }, 400))
      return json(pendingDomain, 201)
    })
    renderAt('/org')
    const input = await screen.findByLabelText('Dominio (por ejemplo, ejemplo.com)')
    await userEvent.type(input, 'co.uk')
    await userEvent.click(screen.getByRole('button', { name: 'Añadir dominio' }))
    expect(await screen.findByText(/Es un sufijo público/)).toBeTruthy()
    await userEvent.clear(input)
    await userEvent.type(input, 'fizsar.com')
    await userEvent.click(screen.getByRole('button', { name: 'Añadir dominio' }))
    expect(await screen.findByText('_quantumready.fizsar.com')).toBeTruthy()
    expect(sent(fetch, 'POST', '/api/org/domains').body).toEqual({ name: 'co.uk' })
  })

  it('lost: verified until the end of the grace period', async () => {
    api(owner, [{ ...verifiedDomain, status: 'lost', verified: true, lost_at: '2026-10-10T00:00:00Z', grace_until: '2026-10-24T00:00:00Z' }])
    renderAt('/org')
    expect(await screen.findByText('registro no encontrado: verificado hasta el 24 de octubre de 2026')).toBeTruthy()
  })

  it('viewers see the domains but cannot change them', async () => {
    api(viewer, [pendingDomain])
    renderAt('/org')
    expect(await screen.findByText('fizsar.com')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Comprobar ahora' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Quitar el dominio fizsar.com' })).toBeNull()
    expect(screen.queryByLabelText('Dominio (por ejemplo, ejemplo.com)')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Analizar' })).toBeNull()
  })

  it('removes a domain after confirming', async () => {
    const fetch = api(owner, [pendingDomain])
    renderAt('/org')
    await userEvent.click(await screen.findByRole('button', { name: 'Quitar el dominio fizsar.com' }))
    await waitFor(() => sent(fetch, 'DELETE', '/api/org/domains/d1'))
    await waitFor(() => expect(screen.queryByText('_quantumready.fizsar.com')).toBeNull())
  })
})

describe('full scans', () => {
  it('without a verified domain: explains what to do', async () => {
    api(owner, [pendingDomain])
    renderAt('/org')
    expect(await screen.findByText('Verifica un dominio para poder hacer análisis completos.')).toBeTruthy()
  })

  it('history and a new full scan of a verified domain', async () => {
    const fetch = api(owner, [verifiedDomain], (url, init) => {
      if (url.startsWith('/api/org/scans?') && init?.method === 'POST') return json({ ...scanView('queued'), id: 'new', intensity: 'full' }, 202)
      if (url.startsWith('/api/org/scans/new')) return json({ ...scanView('running'), id: 'new', host: 'quantumready.fizsar.com', intensity: 'full' })
      return undefined
    }, history)
    renderAt('/org')
    const first = await screen.findByRole('link', { name: 'quantumready.fizsar.com' })
    expect(first.getAttribute('href')).toBe('/org/scans/s1')
    expect(within(first.closest('li')!).getByText('Nota A · 95/100')).toBeTruthy()
    expect(screen.getByText('en cola')).toBeTruthy()
    expect(screen.getByText(/Puedes analizar a fondo estos dominios verificados y sus subdominios: fizsar.com/)).toBeTruthy()

    await userEvent.type(screen.getByLabelText('Dominio o subdominio'), 'quantumready.fizsar.com')
    await userEvent.click(screen.getByRole('button', { name: 'Analizar' }))
    await waitFor(() => expect(screen.getByRole('status').textContent).toContain('Escaneando quantumready.fizsar.com'))
    expect(sent(fetch, 'POST', '/api/org/scans').body).toEqual({ host: 'quantumready.fizsar.com' })
    expect(fetch.mock.calls.some(([u]) => String(u).startsWith('/api/org/scans/new'))).toBe(true) // the org endpoint, not /api/scans
  })
})
