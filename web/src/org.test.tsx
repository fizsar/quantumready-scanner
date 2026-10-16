import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Invitation, Me, Member, Org } from './api'
import { json, stubFetch } from './test/fixtures'
import { renderAt } from './test/render'

const me = (orgs: Me['organizations'] = [{ id: 'oA', name: 'Acme', role: 'owner' }]): Me => ({
  user: { id: 'u1', email: 'ana@example.com', lang: 'es' },
  organizations: orgs,
  active_org_id: orgs[0]?.id ?? null,
  csrf_token: 'csrf-1',
  can_create_organizations: true,
})
const owner: Org = {
  id: 'oA', name: 'Acme', role: 'owner',
  permissions: ['view', 'rename', 'invite', 'revoke_invitation', 'change_role', 'remove_member', 'manage_domains'],
}
const viewer: Org = { id: 'oA', name: 'Acme', role: 'viewer', permissions: ['view'] }
const members: Member[] = [
  { user_id: 'u1', email: 'ana@example.com', role: 'owner', joined_at: '2026-10-01T00:00:00Z', you: true },
  { user_id: 'u2', email: 'bea@example.com', role: 'viewer', joined_at: '2026-10-02T00:00:00Z', you: false },
]
const pending: Invitation[] = [
  { id: 'i1', email: 'dan@example.com', role: 'viewer', created_at: '2026-10-10T00:00:00Z', expires_at: '2026-10-17T00:00:00Z', expired: false },
]
const empty = (status = 204) => new Response(null, { status })

/** A fake API for the organization page; returns the fetch mock. */
function orgAPI(org: Org | null, opts: { me?: Me; extra?: (url: string, init?: RequestInit) => Response | undefined } = {}) {
  return stubFetch((url, init) => {
    const method = init?.method ?? 'GET'
    const r = opts.extra?.(url, init)
    if (r) return r
    if (url.startsWith('/api/auth/me')) return json(opts.me ?? me())
    if (url.startsWith('/api/org?') && method === 'GET')
      return org ? json(org) : json({ error: { code: 'no_organization', message: 'No perteneces a ninguna organización.' } }, 403)
    if (url.startsWith('/api/org/members?')) return json({ members })
    if (url.startsWith('/api/org/invitations?') && method === 'GET') return json({ invitations: pending })
    return empty()
  })
}

function call(fetch: ReturnType<typeof stubFetch>, method: string, prefix: string) {
  const c = fetch.mock.calls.find(([u, i]) => String(u).startsWith(prefix) && (i?.method ?? 'GET') === method)
  expect(c, `${method} ${prefix}`).toBeTruthy()
  return { body: c![1]!.body ? JSON.parse(String(c![1]!.body)) : undefined, headers: c![1]!.headers as Record<string, string> }
}

beforeEach(() => {
  vi.spyOn(window, 'confirm').mockReturnValue(true)
})
afterEach(() => window.history.replaceState(null, '', '/'))

describe('organization page', () => {
  it('owner: members with roles, change a role, remove a member, invite and revoke', async () => {
    const fetch = orgAPI(owner, {
      extra: (url, init) =>
        url.startsWith('/api/org/invitations?') && init?.method === 'POST'
          ? json({ ...pending[0], id: 'i2', email: 'eva@example.com' }, 201)
          : undefined,
    })
    renderAt('/org')
    expect(await screen.findByRole('heading', { level: 1, name: 'Acme' })).toBeTruthy()
    expect(screen.getByText('Tu rol: propietario')).toBeTruthy()

    await userEvent.selectOptions(screen.getByLabelText('Rol de bea@example.com'), 'admin')
    await waitFor(() => expect(call(fetch, 'PATCH', '/api/org/members/u2').body).toEqual({ role: 'admin' }))
    expect(call(fetch, 'PATCH', '/api/org/members/u2').headers['X-QR-CSRF']).toBe('csrf-1')

    expect(screen.queryByRole('button', { name: 'Quitar a ana@example.com' })).toBeNull() // yourself: "leave" instead
    await userEvent.click(screen.getByRole('button', { name: 'Quitar a bea@example.com' }))
    await waitFor(() => call(fetch, 'DELETE', '/api/org/members/u2'))

    await userEvent.type(screen.getByLabelText('Email de la persona'), 'eva@example.com')
    await userEvent.selectOptions(screen.getByLabelText('Rol'), 'admin')
    await userEvent.click(screen.getByRole('button', { name: 'Enviar invitación' }))
    expect((await screen.findByText(/Invitación enviada a eva@example.com/)).textContent).toContain('7 días')
    expect(call(fetch, 'POST', '/api/org/invitations').body).toEqual({ email: 'eva@example.com', role: 'admin', lang: 'es' })

    const list = screen.getByText('dan@example.com').closest('li')!
    expect(within(list).getByText(/caduca el 17 de octubre/)).toBeTruthy()
    await userEvent.click(within(list).getByRole('button', { name: 'Revocar la invitación a dan@example.com' }))
    await waitFor(() => call(fetch, 'DELETE', '/api/org/invitations/i1'))
  })

  it('viewer: read only, can leave', async () => {
    const fetch = orgAPI(viewer, { me: me([{ id: 'oA', name: 'Acme', role: 'viewer' }]) })
    renderAt('/org')
    expect(await screen.findByText('Tu rol: lectura')).toBeTruthy()
    expect(screen.getByText('bea@example.com')).toBeTruthy()
    expect(screen.queryByRole('combobox', { name: /Rol de/ })).toBeNull()
    expect(screen.queryByRole('button', { name: /Quitar a/ })).toBeNull()
    expect(screen.queryByText('Invitar a alguien')).toBeNull()
    expect(screen.queryByLabelText('Nombre de la organización')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Salir de la organización' }))
    await waitFor(() => expect(call(fetch, 'POST', '/api/org/leave').headers['X-QR-CSRF']).toBe('csrf-1'))
    expect(window.confirm).toHaveBeenCalledWith('¿Seguro que quieres salir de Acme?')
  })

  it('an API refusal is shown (last owner)', async () => {
    orgAPI(owner, {
      extra: (url) =>
        url.startsWith('/api/org/leave')
          ? json({ error: { code: 'last_owner', message: 'La organización necesita al menos un propietario: nombra antes a otro.' } }, 409)
          : undefined,
    })
    renderAt('/org')
    await userEvent.click(await screen.findByRole('button', { name: 'Salir de la organización' }))
    expect(await screen.findByText(/necesita al menos un propietario/)).toBeTruthy()
  })

  it('switches organization and creates one', async () => {
    const fetch = orgAPI(owner, { me: me([{ id: 'oA', name: 'Acme', role: 'owner' }, { id: 'oB', name: 'Beta', role: 'viewer' }]) })
    renderAt('/org', 'en')
    await userEvent.selectOptions(await screen.findByLabelText('Organization'), 'oB')
    await userEvent.click(screen.getByRole('button', { name: 'Switch' }))
    await waitFor(() => call(fetch, 'POST', '/api/organizations/oB/switch'))
    await userEvent.type(screen.getByLabelText('Name of a new organization'), 'Gamma')
    await userEvent.click(screen.getByRole('button', { name: 'Create organization' }))
    await waitFor(() => expect(call(fetch, 'POST', '/api/organizations?').body).toEqual({ name: 'Gamma' }))
  })

  it('invited account during the beta: no form to create organizations', async () => {
    orgAPI(viewer, { me: { ...me([{ id: 'oA', name: 'Acme', role: 'viewer' }]), can_create_organizations: false } })
    renderAt('/org')
    expect(await screen.findByText(/las cuentas creadas por invitación no pueden crear organizaciones/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Crear organización' })).toBeNull()
  })

  it('without an organization, offers to create one', async () => {
    const fetch = orgAPI(null, { me: me([]) })
    renderAt('/org')
    expect(await screen.findByText(/No perteneces a ninguna organización/)).toBeTruthy()
    await userEvent.type(screen.getByLabelText('Nombre de una organización nueva'), 'Mía')
    await userEvent.click(screen.getByRole('button', { name: 'Crear organización' }))
    await waitFor(() => call(fetch, 'POST', '/api/organizations'))
  })
})

describe('invitation link', () => {
  const preview = { organization: 'Acme', email: 'eva@example.com', role: 'viewer', inviter_email: 'ana@example.com', expires_at: '2026-10-23T00:00:00Z' }
  const unauth = () => json({ error: { code: 'unauthenticated', message: 'x' } }, 401)

  it('without an account: signs up with the invitation and lands in the organization', async () => {
    window.history.replaceState(null, '', '/invite#token=inv_tok')
    let signedIn = false
    const fetch = stubFetch((url) => {
      if (url.startsWith('/api/invitations/preview')) return json(preview)
      if (url.startsWith('/api/auth/register')) return json({ status: 'registered' }, 201)
      if (url.startsWith('/api/auth/login')) return ((signedIn = true), json(me([{ id: 'oA', name: 'Acme', role: 'viewer' }])))
      if (url.startsWith('/api/auth/me')) return signedIn ? json(me()) : unauth()
      if (url.startsWith('/api/org?')) return json(viewer)
      if (url.startsWith('/api/org/members')) return json({ members })
      return empty()
    })
    renderAt('/invite')
    expect(await screen.findByText('ana@example.com te invita a unirte a «Acme» con el rol lectura.')).toBeTruthy()
    expect(window.location.hash).toBe('')
    expect((screen.getByLabelText('Email') as HTMLInputElement).readOnly).toBe(true)
    await userEvent.click(screen.getByRole('button', { name: '¿No tienes cuenta? Créala aquí' }))
    await userEvent.type(screen.getByLabelText('Contraseña'), 'tranvia-azul-1987-luna')
    await userEvent.click(screen.getByRole('button', { name: 'Crear cuenta y unirme' }))
    expect(await screen.findByText('Tu rol: lectura')).toBeTruthy() // on /org
    expect(call(fetch, 'POST', '/api/auth/register').body).toEqual({
      email: 'eva@example.com', password: 'tranvia-azul-1987-luna', invitation_token: 'inv_tok', lang: 'es',
    })
  })

  it('signed in as the invited address: accept with the CSRF token', async () => {
    window.history.replaceState(null, '', '/invite#token=inv_tok')
    const eva: Me = { ...me(), user: { id: 'u5', email: 'eva@example.com', lang: 'es' } }
    const fetch = stubFetch((url) => {
      if (url.startsWith('/api/invitations/preview')) return json(preview)
      if (url.startsWith('/api/invitations/accept')) return json(viewer)
      if (url.startsWith('/api/auth/me')) return json(eva)
      if (url.startsWith('/api/org?')) return json(viewer)
      if (url.startsWith('/api/org/members')) return json({ members })
      return empty()
    })
    renderAt('/invite')
    await userEvent.click(await screen.findByRole('button', { name: 'Aceptar la invitación' }))
    expect(await screen.findByText('Tu rol: lectura')).toBeTruthy()
    const req = call(fetch, 'POST', '/api/invitations/accept')
    expect(req.body).toEqual({ token: 'inv_tok' })
    expect(req.headers['X-QR-CSRF']).toBe('csrf-1')
  })

  it('signed in as someone else: explains, no accept button', async () => {
    window.history.replaceState(null, '', '/invite#token=inv_tok')
    stubFetch((url) => (url.startsWith('/api/invitations/preview') ? json(preview) : json(me())))
    renderAt('/invite', 'en')
    expect(await screen.findByText(/This invitation is for eva@example.com and you are signed in as ana@example.com/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Accept the invitation' })).toBeNull()
  })

  it('a used or expired link', async () => {
    window.history.replaceState(null, '', '/invite#token=old')
    stubFetch((url) =>
      url.startsWith('/api/invitations/preview')
        ? json({ error: { code: 'invalid_token', message: 'El enlace no es válido, ha caducado o ya se usó. Pide uno nuevo.' } }, 400)
        : unauth(),
    )
    renderAt('/invite')
    expect((await screen.findByRole('alert')).textContent).toContain('ha caducado')
  })
})
