import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { solveAltcha, type AltchaChallenge } from './altcha'
import type { Domain, Me, Org } from './api'
import { json, reportA, scanView } from './test/fixtures'
import { stubFetch } from './test/accounts'
import { renderAt } from './test/render'

async function sha256hex(s: string) {
  const d = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(s))
  return Array.from(new Uint8Array(d), (b) => b.toString(16).padStart(2, '0')).join('')
}

/** A challenge whose answer is n (the server picks it at random). */
async function challenge(n: number): Promise<AltchaChallenge> {
  const salt = 'abc123?expires=9999999999&'
  return { algorithm: 'SHA-256', challenge: await sha256hex(salt + n), maxnumber: 50, salt, signature: 'sig' }
}

const unauth = () => json({ error: { code: 'unauthenticated', message: 'x' } }, 401)

describe('altcha solver', () => {
  it('finds the number and builds the payload', async () => {
    const payload = JSON.parse(atob(await solveAltcha(await challenge(37))))
    expect(payload).toEqual({ algorithm: 'SHA-256', challenge: (await challenge(37)).challenge, number: 37, salt: 'abc123?expires=9999999999&', signature: 'sig' })
  })
  it('refuses other algorithms and impossible challenges', async () => {
    await expect(solveAltcha({ ...(await challenge(1)), algorithm: 'SHA-1' })).rejects.toThrow()
    await expect(solveAltcha({ ...(await challenge(1)), challenge: '00' })).rejects.toThrow('no solution')
  })
})

describe('open public scanning', () => {
  it('any domain, with the proof of work solved in the browser', async () => {
    const ch = await challenge(12)
    const fetch = stubFetch((url, init) => {
      if (url === '/api/targets') return json({ hosts: ['fizsar.com'], port: 443, open: true, proof: true })
      if (url === '/api/altcha') return json(ch)
      if (url.startsWith('/api/scans?') && init?.method === 'POST') return json({ ...scanView('queued'), host: 'example.org' }, 202)
      if (url.startsWith('/api/auth/')) return unauth()
      return json({ ...scanView('running'), host: 'example.org' })
    })
    renderAt('/')
    expect(await screen.findByText(/^Escribe cualquier dominio para analizarlo: solo negociamos TLS/)).toBeTruthy()
    // No demo domains when scanning is open, and the way to /about-scans.
    expect(screen.queryByText(/demostración/)).toBeNull()
    expect(screen.getByRole('link', { name: 'Qué hace el escáner, desde qué IP analiza y cómo excluir tu dominio' }).getAttribute('href')).toBe('/about-scans')
    await userEvent.type(screen.getByLabelText('Dominio'), 'example.org')
    await userEvent.click(screen.getByRole('button', { name: 'Escanear' }))
    await waitFor(() => expect(screen.getByRole('status').textContent).toContain('Escaneando example.org'))
    const post = fetch.mock.calls.find(([u, i]) => String(u).startsWith('/api/scans?') && i?.method === 'POST')!
    const body = JSON.parse(String(post[1]!.body))
    expect(body.host).toBe('example.org')
    expect(JSON.parse(atob(body.altcha)).number).toBe(12)
  })

  it('a recent result handed out instead of a new scan says how old it is', async () => {
    stubFetch((url, init) => {
      if (url === '/api/targets') return json({ hosts: [], port: 443, open: true, proof: false })
      if (url.startsWith('/api/scans?') && init?.method === 'POST')
        return json({ ...scanView('done', { report: reportA }), reused: true, age_seconds: 1500 })
      if (url.startsWith('/api/auth/')) return unauth()
      return json(scanView('done', { report: reportA }))
    })
    renderAt('/')
    await userEvent.type(await screen.findByLabelText('Dominio'), 'fizsar.com')
    await userEvent.click(screen.getByRole('button', { name: 'Escanear' }))
    expect((await screen.findByText(/^Resultado de hace 25 min/)).getAttribute('role')).toBe('status')
  })

  it.each([
    [1, 3, 'En cola: el tuyo es el siguiente (espera aproximada: menos de un minuto).'],
    [12, 36, 'En cola: hay 11 análisis antes que el tuyo (espera aproximada: menos de un minuto).'],
    [45, 135, 'En cola: hay 44 análisis antes que el tuyo (espera aproximada: unos 3 min).'],
  ])('queued at position %i: shows the place and the wait', async (position, eta, text) => {
    stubFetch((url) => (url.startsWith('/api/auth/') ? unauth() : json({ ...scanView('queued'), queue_position: position, queue_eta_seconds: eta })))
    renderAt('/scans/abc')
    await waitFor(() => expect(screen.getByRole('status').textContent).toBe(text))
  })

  it('closed: no proof of work, the demo text', async () => {
    const fetch = stubFetch((url) => (url === '/api/targets' ? json({ hosts: ['fizsar.com'], port: 443 }) : unauth()))
    renderAt('/')
    expect(await screen.findByText(/solo está abierto a estos dominios de demostración/)).toBeTruthy()
    expect(fetch.mock.calls.some(([u]) => String(u) === '/api/altcha')).toBe(false)
  })

  it('an excluded domain: the API message', async () => {
    stubFetch((url, init) => {
      if (url === '/api/targets') return json({ hosts: [], port: 443, open: true, proof: false })
      if (url.startsWith('/api/scans?') && init?.method === 'POST')
        return json({ error: { code: 'scan_not_allowed', message: 'Este dominio está excluido de los análisis públicos.' } }, 403)
      return unauth()
    })
    renderAt('/')
    await userEvent.type(await screen.findByLabelText('Dominio'), 'excluded.example')
    await userEvent.click(screen.getByRole('button', { name: 'Escanear' }))
    expect((await screen.findByRole('alert')).textContent).toBe('Este dominio está excluido de los análisis públicos.')
  })
})

describe('about the scanner', () => {
  it('explains what, from where, how often and how to opt out', async () => {
    stubFetch(() => unauth())
    renderAt('/about-scans')
    expect(await screen.findByRole('heading', { level: 1, name: 'Cómo funciona el escáner' })).toBeTruthy()
    for (const h of ['Qué hace', 'Desde dónde', 'Con qué frecuencia, como máximo', 'Qué se guarda', 'Cómo excluir tu dominio']) {
      expect(screen.getByRole('heading', { level: 2, name: h })).toBeTruthy()
    }
    expect(screen.getByText(/149\.56\.13\.98, cuyo nombre inverso es quantumready\.fizsar\.com/)).toBeTruthy()
    expect(screen.getByText(/durante 30 días \(y en las copias de seguridad cifradas, hasta 6 meses\)\. Nada de quién lo pidió/)).toBeTruthy()
    expect(screen.getByText(/Solo nuestros propios dominios \(fizsar\.com y quantumready\.fizsar\.com\) reciben además el análisis completo/)).toBeTruthy()
    expect(screen.getByText(/Escríbenos a soporte@fizsar\.com/)).toBeTruthy()
    expect(screen.getByRole('link', { name: 'security.txt' }).getAttribute('href')).toBe('/.well-known/security.txt')
    expect(screen.getAllByRole('link', { name: 'Cómo funciona el escáner' }).length).toBeGreaterThan(0) // footer
  })
  it('in English', async () => {
    stubFetch(() => unauth())
    renderAt('/about-scans', 'en')
    expect(await screen.findByText(/It does not request pages/)).toBeTruthy()
  })
})

describe('opt-out of public scans', () => {
  const me: Me = {
    user: { id: 'u1', email: 'ana@example.com', lang: 'es' },
    organizations: [{ id: 'oA', name: 'Acme', role: 'owner' }],
    active_org_id: 'oA', csrf_token: 'csrf-1', can_create_organizations: true,
  }
  const owner: Org = { id: 'oA', name: 'Acme', role: 'owner', permissions: ['view', 'rename', 'invite', 'revoke_invitation', 'change_role', 'remove_member', 'manage_domains'] }
  const verified: Domain = {
    id: 'd1', name: 'fizsar.com', status: 'verified', verified: true, check_pending: false, block_public: false,
    record: { type: 'TXT', name: '_quantumready.fizsar.com', value: 'qr-verify=x' },
  }

  it('a block that outlived the verification is shown, without the switch', async () => {
    stubFetch((url) => {
      if (url.startsWith('/api/auth/me')) return json(me)
      if (url.startsWith('/api/org?')) return json(owner)
      if (url.startsWith('/api/org/domains?')) return json({ domains: [{ ...verified, status: 'pending', verified: false, block_public: true }] })
      if (url.startsWith('/api/org/members')) return json({ members: [] })
      if (url.startsWith('/api/org/invitations')) return json({ invitations: [] })
      if (url.startsWith('/api/org/scans')) return json({ scans: [] })
      return new Response(null, { status: 204 })
    })
    renderAt('/org')
    expect(await screen.findByText(/siguen bloqueados\. Para quitar el bloqueo, verifica el dominio de nuevo/)).toBeTruthy()
    expect(screen.queryByLabelText(/Bloquear los análisis públicos/)).toBeNull()
  })

  it('the owner of a verified domain blocks public scans', async () => {
    const fetch = stubFetch((url, init) => {
      const method = init?.method ?? 'GET'
      if (url.startsWith('/api/auth/me')) return json(me)
      if (url.startsWith('/api/org?')) return json(owner)
      if (url.startsWith('/api/org/domains/d1') && method === 'PATCH') return json({ ...verified, block_public: true })
      if (url.startsWith('/api/org/domains?')) return json({ domains: [verified] })
      if (url.startsWith('/api/org/members')) return json({ members: [] })
      if (url.startsWith('/api/org/invitations')) return json({ invitations: [] })
      if (url.startsWith('/api/org/scans')) return json({ scans: [] })
      return new Response(null, { status: 204 })
    })
    renderAt('/org')
    const item = (await screen.findByText('fizsar.com')).closest('li')!
    const box = within(item).getByLabelText('Bloquear los análisis públicos de este dominio (y sus subdominios)') as HTMLInputElement
    expect(box.checked).toBe(false)
    await userEvent.click(box)
    await waitFor(() => expect(box.checked).toBe(true))
    const patch = fetch.mock.calls.find(([u, i]) => String(u).startsWith('/api/org/domains/d1') && i?.method === 'PATCH')!
    expect(JSON.parse(String(patch[1]!.body))).toEqual({ block_public: true })
    expect((patch[1]!.headers as Record<string, string>)['X-QR-CSRF']).toBe('csrf-1')
  })
})
