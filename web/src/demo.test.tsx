import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Lang } from './api'
import App from './App'
import { showcaseDict } from './i18n.showcase'
import { AuthProvider } from './AuthProvider'
import { LangProvider } from './LangProvider'
import { resetEntryForTests } from './metrics'
import { json } from './test/fixtures'
import { stubFetch } from './test/visitor'
import { forbiddenProse, strip } from './test/forbidden'
import fizsarEn from './test/lab/fizsar.en.json'

type Handler = (url: string, init?: RequestInit) => Response | Promise<Response>

function renderAt(path: string, lang: Lang = 'es', api?: Handler, state?: unknown) {
  const posts: { url: string; body: Record<string, unknown> }[] = []
  stubFetch(async (url, init) => {
    if (init?.method === 'POST') posts.push({ url, body: JSON.parse(String(init.body)) })
    // The form is on in these tests (DEMO_ENABLED=true); demoOff.test.tsx covers it off.
    if (url === '/api/targets') return json({ hosts: ['fizsar.com'], port: 443, demo: true })
    if (api) return api(url, init)
    if (url === '/api/demo/challenge') return json({ error: { code: 'not_found', message: 'No encontrado.' } }, 404)
    if (url.startsWith('/api/demo')) return json({ status: 'received' }, 202)
    return json(fizsarEn)
  })
  const view = render(
    <MemoryRouter initialEntries={[state ? { pathname: path, state } : path]}>
      <LangProvider initial={lang}>
        <AuthProvider>
          <App />
        </AuthProvider>
      </LangProvider>
    </MemoryRouter>,
  )
  return { ...view, posts }
}

/** M1 events sent so far (sendBeacon stub of test/setup.ts). */
async function events(): Promise<Record<string, unknown>[]> {
  const beacon = navigator.sendBeacon as unknown as ReturnType<typeof vi.fn>
  return Promise.all(beacon.mock.calls.map(async ([, blob]) => JSON.parse(await (blob as Blob).text())))
}

async function fillAndSend(lang: Lang = 'es', accept = true) {
  const d = showcaseDict[lang]
  await userEvent.type(await screen.findByLabelText(d['demo.name']), 'Ana García')
  await userEvent.type(screen.getByLabelText(d['demo.email']), 'ana@example.org')
  await userEvent.type(screen.getByLabelText(d['demo.company']), 'Acme')
  await userEvent.type(screen.getByLabelText(d['demo.domain']), 'example.org')
  if (accept) await userEvent.click(screen.getByRole('checkbox'))
  await userEvent.click(screen.getByRole('button', { name: d['demo.submit'] }))
}

beforeEach(() => resetEntryForTests())

describe('/demo', () => {
  it('a form with every field, the box unchecked and the honeypot out of reach', async () => {
    const { container } = renderAt('/demo')
    expect(await screen.findByRole('heading', { level: 1, name: 'Solicitar demo gratis' })).toBeTruthy()
    for (const k of ['demo.name', 'demo.email', 'demo.company', 'demo.domain'] as const) {
      expect(screen.getByLabelText(showcaseDict.es[k])).toHaveProperty('required', true)
    }
    expect(screen.getByLabelText(/Mensaje/)).toHaveProperty('required', false)
    const box = screen.getByRole('checkbox', { name: /política de privacidad/ })
    expect((box as HTMLInputElement).checked).toBe(false)
    expect(within(box.parentElement!).getByRole('link', { name: 'política de privacidad' }).getAttribute('href')).toBe('/privacy')
    // The honeypot: in the form, hidden from people and assistive technology, not focusable.
    const hp = container.querySelector('input[name="website"]') as HTMLInputElement
    expect(hp.tabIndex).toBe(-1)
    expect(hp.closest('[aria-hidden="true"]')?.className).toBe('demo-hp')
    expect(screen.queryByRole('textbox', { name: 'Website' })).toBeNull()
    expect(container.querySelectorAll('[style], style, script')).toHaveLength(0)
  })

  it('sends the request without proof when the server asks for none, then counts where the button was', async () => {
    const { posts } = renderAt('/demo', 'es', undefined, { from: 'home' })
    await fillAndSend()
    expect((await screen.findByRole('status')).textContent).toContain('Solicitud enviada')
    expect(posts).toEqual([
      {
        url: '/api/demo?lang=es',
        body: { name: 'Ana García', email: 'ana@example.org', company: 'Acme', domain: 'example.org', message: '', privacy: true, lang: 'es', website: '' },
      },
    ])
    const evs = (await events()).filter((e) => e.e === 'demo_requested')
    expect(evs).toEqual([{ e: 'demo_requested', d: 'home' }]) // never the form's contents
  })

  it('opened directly: the event says "direct"', async () => {
    renderAt('/demo', 'en')
    await fillAndSend('en')
    await screen.findByRole('status')
    expect((await events()).filter((e) => e.e === 'demo_requested')).toEqual([{ e: 'demo_requested', d: 'direct' }])
  })

  it('solves the proof of work when the server asks for one', async () => {
    const salt = 'abc?expires=9999999999&'
    const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(salt + 7))
    const challenge = Array.from(new Uint8Array(digest), (b) => b.toString(16).padStart(2, '0')).join('')
    const { posts } = renderAt('/demo', 'es', (url) => {
      if (url === '/api/demo/challenge') return json({ algorithm: 'SHA-256', challenge, maxnumber: 50, salt, signature: 'sig' })
      return json({ status: 'received' }, 202)
    })
    await fillAndSend()
    await screen.findByRole('status')
    const proof = JSON.parse(atob(String(posts[0].body.altcha)))
    expect(proof).toEqual({ algorithm: 'SHA-256', challenge, number: 7, salt, signature: 'sig' })
  })

  it('shows the API message and counts nothing when the request is refused', async () => {
    renderAt('/demo', 'es', (url) =>
      url === '/api/demo/challenge'
        ? json({ error: { code: 'not_found', message: '' } }, 404)
        : json({ error: { code: 'privacy_required', message: 'Para enviar la solicitud tienes que aceptar la política de privacidad.' } }, 400),
    )
    await fillAndSend('es', false)
    await waitFor(() => expect(screen.getByRole('alert').textContent).toContain('tienes que aceptar la política de privacidad'))
    expect(screen.queryByRole('status')).toBeNull()
    expect((await events()).filter((e) => e.e === 'demo_requested')).toEqual([])
    expect(screen.getByRole('button', { name: 'Enviar solicitud' })).toHaveProperty('disabled', false) // can try again
  })

  it('a network failure gets a plain message', async () => {
    renderAt('/demo', 'en', () => Promise.reject(new Error('offline')))
    await fillAndSend('en')
    await waitFor(() => expect(screen.getByRole('alert').textContent).toContain(showcaseDict.en['demo.error.network']))
  })

  it('the button is on the home page, in the 403 block and on /examples, each with its source', async () => {
    renderAt('/')
    const home = await screen.findByRole('link', { name: 'Solicitar demo gratis' })
    expect(home.getAttribute('href')).toBe('/demo')
    await userEvent.click(home)
    expect(await screen.findByRole('heading', { level: 1, name: 'Solicitar demo gratis' })).toBeTruthy()
  })

  it('in the 403 block', async () => {
    renderAt('/', 'es', (url, init) => {
      if (url === '/api/targets') return json({ hosts: ['fizsar.com'], port: 443 })
      if (init?.method === 'POST') return json({ error: { code: 'host_not_allowed', message: 'x' } }, 403)
      return json({}, 404)
    })
    await userEvent.type(await screen.findByRole('textbox'), 'example.org')
    await userEvent.click(screen.getByRole('button', { name: 'Escanear' }))
    const block = await screen.findByRole('heading', { name: showcaseDict.es['hna.title'] })
    expect(within(block.parentElement!).getByRole('link', { name: 'Solicitar demo gratis' }).getAttribute('href')).toBe('/demo')
  })

  it('on /examples', async () => {
    renderAt('/examples', 'en')
    await screen.findByRole('heading', { level: 1, name: 'Examples' }, { timeout: 5000 })
    expect((await screen.findByRole('link', { name: 'Request a free demo' })).getAttribute('href')).toBe('/demo')
  })

  for (const lang of ['es', 'en'] as const) {
    it(`says nothing it must not (${lang})`, () => {
      const all = Object.entries(showcaseDict[lang])
        .filter(([k]) => k.startsWith('demo.'))
        .map(([, v]) => strip(v))
        .join('\n')
      for (const re of forbiddenProse) expect(all, String(re)).not.toMatch(re)
    })
  }
})
