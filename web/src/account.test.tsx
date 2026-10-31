import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it } from 'vitest'
import type { Me } from './api'
import { json } from './test/fixtures'
import { stubFetch } from './test/accounts'
import { renderAt } from './test/render'

const me: Me = {
  user: { id: 'u1', email: 'ana@example.com', lang: 'es' },
  organizations: [{ id: 'o1', name: 'Acme', role: 'owner' }],
  active_org_id: 'o1',
  csrf_token: 'csrf-123',
  can_create_organizations: true,
}

const unauthenticated = () => json({ error: { code: 'unauthenticated', message: 'Tienes que iniciar sesión.' } }, 401)
const apiError = (status: number, code: string, message: string, headers: Record<string, string> = {}) =>
  json({ error: { code, message } }, status, headers)

/** The body and headers of the request to path. */
function sent(fetch: ReturnType<typeof stubFetch>, path: string) {
  const call = fetch.mock.calls.find(([url]) => String(url).startsWith(path))
  expect(call, `no request to ${path}`).toBeTruthy()
  const init = call![1]!
  return { body: init.body ? JSON.parse(String(init.body)) : undefined, headers: init.headers as Record<string, string> }
}

afterEach(() => window.history.replaceState(null, '', '/'))

describe('header', () => {
  it('offers to sign in without a session and shows the account with one', async () => {
    stubFetch((url) => (url.startsWith('/api/auth/me') ? unauthenticated() : json({ hosts: ['fizsar.com'], port: 443 })))
    const { unmount } = renderAt('/')
    expect(await screen.findByRole('link', { name: 'Entrar' })).toBeTruthy()
    unmount()
    stubFetch((url) => (url.startsWith('/api/auth/me') ? json(me) : json({ hosts: ['fizsar.com'], port: 443 })))
    renderAt('/')
    expect(await screen.findByRole('link', { name: 'Mi cuenta' })).toBeTruthy()
  })
})

describe('login', () => {
  it('signs in, shows the account and signs out with the CSRF token', async () => {
    let signedIn = false
    const fetch = stubFetch((url) => {
      if (url.startsWith('/api/auth/me')) return unauthenticated()
      if (url.startsWith('/api/auth/login')) return ((signedIn = true), json(me))
      if (url.startsWith('/api/auth/logout')) return new Response(null, { status: 204 })
      return json({ hosts: [], port: 443 })
    })
    renderAt('/login')
    await userEvent.type(await screen.findByLabelText('Email'), 'ana@example.com')
    await userEvent.type(screen.getByLabelText('Contraseña'), 'tranvia-azul-1987-luna')
    await userEvent.click(screen.getByRole('button', { name: 'Entrar' }))

    expect(await screen.findByRole('heading', { name: 'Mi cuenta' })).toBeTruthy()
    expect(signedIn).toBe(true)
    expect(sent(fetch, '/api/auth/login?lang=es').body).toEqual({ email: 'ana@example.com', password: 'tranvia-azul-1987-luna' })
    expect(screen.getByText('ana@example.com')).toBeTruthy()
    expect(screen.getByText('· propietario')).toBeTruthy()

    await userEvent.click(screen.getByRole('button', { name: 'Cerrar sesión' }))
    expect(await screen.findByText('Entra para ver tu cuenta.')).toBeTruthy()
    expect(sent(fetch, '/api/auth/logout').headers['X-QR-CSRF']).toBe('csrf-123')
    expect(within(screen.getByRole('banner')).getByRole('link', { name: 'Entrar' })).toBeTruthy()
  })

  it('wrong credentials: the API message, fields marked invalid', async () => {
    stubFetch((url) => (url.startsWith('/api/auth/login') ? apiError(401, 'invalid_credentials', 'Email o contraseña incorrectos.') : unauthenticated()))
    renderAt('/login')
    await userEvent.type(await screen.findByLabelText('Email'), 'ana@example.com')
    await userEvent.type(screen.getByLabelText('Contraseña'), 'x')
    await userEvent.click(screen.getByRole('button', { name: 'Entrar' }))
    expect((await screen.findByRole('alert')).textContent).toBe('Email o contraseña incorrectos.')
    expect(screen.getByLabelText('Contraseña').getAttribute('aria-invalid')).toBe('true')
  })

  it('not verified: offers to resend the confirmation email', async () => {
    const fetch = stubFetch((url) => {
      if (url.startsWith('/api/auth/login')) return apiError(403, 'email_not_verified', 'Confirma tu email antes de entrar.')
      if (url.startsWith('/api/auth/resend-verification')) return json({ status: 'check_email' }, 202)
      return unauthenticated()
    })
    renderAt('/login')
    await userEvent.type(await screen.findByLabelText('Email'), 'ana@example.com')
    await userEvent.type(screen.getByLabelText('Contraseña'), 'tranvia-azul-1987-luna')
    await userEvent.click(screen.getByRole('button', { name: 'Entrar' }))
    await userEvent.click(await screen.findByRole('button', { name: 'Reenviar el email de confirmación' }))
    expect(await screen.findByText(/te hemos enviado otro enlace/)).toBeTruthy()
    expect(sent(fetch, '/api/auth/resend-verification').body).toEqual({ email: 'ana@example.com', lang: 'es' })
  })

  it('429: says when to retry', async () => {
    stubFetch((url) =>
      url.startsWith('/api/auth/login') ? apiError(429, 'too_many_attempts', 'x', { 'Retry-After': '120' }) : unauthenticated(),
    )
    renderAt('/login', 'en')
    await userEvent.type(await screen.findByLabelText('Email'), 'ana@example.com')
    await userEvent.type(screen.getByLabelText('Password'), 'x')
    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }))
    expect((await screen.findByRole('alert')).textContent).toBe('Too many attempts. You can try again in 120 seconds.')
  })
})

describe('register', () => {
  it('sends the form and asks to check the inbox', async () => {
    const fetch = stubFetch((url) => (url.startsWith('/api/auth/register') ? json({ status: 'check_email' }, 202) : unauthenticated()))
    renderAt('/register')
    await userEvent.type(await screen.findByLabelText('Código de invitación'), 'code-1')
    await userEvent.type(screen.getByLabelText('Email'), 'ana@example.com')
    await userEvent.type(screen.getByLabelText('Contraseña'), 'tranvia-azul-1987-luna')
    await userEvent.type(screen.getByLabelText('Organización (opcional)'), 'Acme')
    await userEvent.click(screen.getByRole('button', { name: 'Crear cuenta' }))
    expect((await screen.findByRole('status')).textContent).toContain('Te hemos enviado un email a ana@example.com')
    expect(sent(fetch, '/api/auth/register?lang=es').body).toEqual({
      email: 'ana@example.com', password: 'tranvia-azul-1987-luna', name: '', organization: 'Acme', signup_code: 'code-1', lang: 'es',
    })
  })

  it('a bad code marks that field', async () => {
    stubFetch((url) =>
      url.startsWith('/api/auth/register') ? apiError(400, 'invalid_signup_code', 'El código de invitación no es válido.') : unauthenticated(),
    )
    renderAt('/register')
    await userEvent.type(await screen.findByLabelText('Código de invitación'), 'nope')
    await userEvent.type(screen.getByLabelText('Email'), 'ana@example.com')
    await userEvent.type(screen.getByLabelText('Contraseña'), 'tranvia-azul-1987-luna')
    await userEvent.click(screen.getByRole('button', { name: 'Crear cuenta' }))
    expect((await screen.findByRole('alert')).textContent).toBe('El código de invitación no es válido.')
    expect(screen.getByLabelText('Código de invitación').getAttribute('aria-invalid')).toBe('true')
    expect(screen.getByLabelText('Email').getAttribute('aria-invalid')).toBeNull()
  })
})

describe('email links', () => {
  it('verify: sends the token from the fragment once and removes it from the URL', async () => {
    window.history.replaceState(null, '', '/verify#token=tok_123-abc')
    const fetch = stubFetch((url) => (url.startsWith('/api/auth/verify-email') ? json({ status: 'verified' }) : unauthenticated()))
    renderAt('/verify')
    // The page appears once the server's switches are known (accounts on).
    expect((await screen.findByRole('status')).textContent).toBe('Confirmando tu email…')
    expect(await screen.findByText('Email confirmado. Ya puedes entrar.')).toBeTruthy()
    expect(sent(fetch, '/api/auth/verify-email').body).toEqual({ token: 'tok_123-abc' })
    expect(fetch.mock.calls.filter(([u]) => String(u).startsWith('/api/auth/verify-email'))).toHaveLength(1)
    expect(window.location.hash).toBe('')
  })

  it('verify: without a token, no request', async () => {
    const fetch = stubFetch(() => unauthenticated())
    renderAt('/verify')
    expect((await screen.findByRole('alert')).textContent).toContain('Al enlace le falta el código')
    expect(fetch.mock.calls.some(([u]) => String(u).startsWith('/api/auth/verify-email'))).toBe(false)
  })

  it('reset: new password with the token; an expired link offers a new one', async () => {
    window.history.replaceState(null, '', '/reset#token=tok_9')
    let first = true
    const fetch = stubFetch((url) => {
      if (!url.startsWith('/api/auth/reset-password')) return unauthenticated()
      if (first) return ((first = false), apiError(400, 'password_common', 'Esa contraseña aparece en listas de contraseñas filtradas.'))
      return json({ status: 'password_reset' })
    })
    renderAt('/reset')
    await userEvent.type(await screen.findByLabelText('Nueva contraseña'), 'password1234')
    await userEvent.click(screen.getByRole('button', { name: 'Guardar contraseña' }))
    expect((await screen.findByRole('alert')).textContent).toContain('listas de contraseñas filtradas')
    await userEvent.clear(screen.getByLabelText('Nueva contraseña'))
    await userEvent.type(screen.getByLabelText('Nueva contraseña'), 'faro-de-niebla-2031-sur')
    await userEvent.click(screen.getByRole('button', { name: 'Guardar contraseña' }))
    expect((await screen.findByRole('status')).textContent).toContain('Contraseña cambiada')
    expect(sent(fetch, '/api/auth/reset-password').body.token).toBe('tok_9')
  })

  it('reset: invalid link', async () => {
    window.history.replaceState(null, '', '/reset#token=old')
    stubFetch((url) =>
      url.startsWith('/api/auth/reset-password') ? apiError(400, 'invalid_token', 'El enlace no es válido, ha caducado o ya se usó.') : unauthenticated(),
    )
    renderAt('/reset')
    await userEvent.type(await screen.findByLabelText('Nueva contraseña'), 'faro-de-niebla-2031-sur')
    await userEvent.click(screen.getByRole('button', { name: 'Guardar contraseña' }))
    expect((await screen.findByRole('alert')).textContent).toContain('ha caducado')
    expect(screen.getByRole('link', { name: 'Pedir otro enlace' })).toBeTruthy()
  })
})

describe('account', () => {
  it('changes the password with the CSRF token', async () => {
    const fetch = stubFetch((url) => (url.startsWith('/api/auth/change-password') ? new Response(null, { status: 204 }) : json(me)))
    renderAt('/account')
    await userEvent.type(await screen.findByLabelText('Contraseña actual'), 'tranvia-azul-1987-luna')
    await userEvent.type(screen.getByLabelText('Nueva contraseña'), 'faro-de-niebla-2031-sur')
    await userEvent.click(screen.getByRole('button', { name: 'Cambiar contraseña' }))
    expect((await screen.findByRole('status')).textContent).toContain('Hemos cerrado tus otras sesiones')
    const req = sent(fetch, '/api/auth/change-password')
    expect(req.headers['X-QR-CSRF']).toBe('csrf-123')
    expect(req.body).toEqual({ current_password: 'tranvia-azul-1987-luna', new_password: 'faro-de-niebla-2031-sur' })
    await waitFor(() => expect((screen.getByLabelText('Contraseña actual') as HTMLInputElement).value).toBe(''))
  })

  it('without a session, asks to sign in', async () => {
    stubFetch(() => unauthenticated())
    renderAt('/account', 'en')
    expect(await screen.findByText('Sign in to see your account.')).toBeTruthy()
  })
})
