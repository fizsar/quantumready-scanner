import { useId, useState, type FormEvent } from 'react'
import { Link, useLocation } from 'react-router'
import { solveAltcha } from '../altcha'
import { ApiError, createDemo, getDemoChallenge } from '../api'
import { useShowcaseT } from '../i18n.showcase'
import { track, type DemoSource } from '../metrics'
import { usePageMeta } from '../usePageMeta'
import './DemoPage.css'

const SOURCES: DemoSource[] = ['home', 'host_not_allowed', 'examples']

function sourceOf(state: unknown): DemoSource {
  const from = (state as { from?: unknown } | null)?.from
  return SOURCES.find((s) => s === from) ?? 'direct'
}

const field = (f: FormData, name: string) => String(f.get(name) ?? '')

/** /demo: "Request a free demo". POST /api/demo; nothing is kept in the browser. */
export function DemoPage() {
  const { t, lang } = useShowcaseT()
  usePageMeta(t('demo.meta.title'), false, t('demo.meta.description'))
  const location = useLocation()
  const id = useId()
  const [state, setState] = useState<'idle' | 'proving' | 'sending' | 'sent'>('idle')
  const [error, setError] = useState('')

  const onSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const f = new FormData(e.currentTarget)
    setError('')
    try {
      setState('proving')
      const challenge = await getDemoChallenge()
      const altcha = challenge ? await solveAltcha(challenge) : undefined
      setState('sending')
      await createDemo({
        name: field(f, 'name'),
        email: field(f, 'email'),
        company: field(f, 'company'),
        domain: field(f, 'domain'),
        message: field(f, 'message'),
        privacy: f.get('privacy') === 'on',
        lang,
        website: field(f, 'website'),
        ...(altcha ? { altcha } : {}),
      })
      track({ e: 'demo_requested', d: sourceOf(location.state) })
      setState('sent')
    } catch (err) {
      setError(err instanceof ApiError && err.code !== 'network' && err.code !== 'internal' ? err.message : t('demo.error.network'))
      setState('idle')
    }
  }

  const busy = state === 'proving' || state === 'sending'
  return (
    <article className="demo" aria-labelledby="demo-title">
      <p className="prompt">
        <span className="user">guest@quantumready</span>:<span className="path">~</span>$ ./request-demo
      </p>
      <h1 id="demo-title" className="h-title">
        {t('demo.title')}
      </h1>
      {state === 'sent' ? (
        <section className="panel" role="status" aria-labelledby={`${id}-sent`}>
          <h2 id={`${id}-sent`} className="h-section">
            {t('demo.sent.title')}
          </h2>
          <p>{t('demo.sent.text')}</p>
          <p>
            <Link to="/examples">{t('demo.sent.examples')}</Link>
          </p>
        </section>
      ) : (
        <section className="panel">
          <p>{t('demo.lead')}</p>
          <p className="muted small">{t('demo.ownership')}</p>
          <form className="demo-form" onSubmit={(e) => void onSubmit(e)} noValidate aria-describedby={error ? `${id}-err` : undefined}>
            <label htmlFor={`${id}-name`}>{t('demo.name')}</label>
            <input id={`${id}-name`} name="name" className="input" type="text" autoComplete="name" required maxLength={100} />
            <label htmlFor={`${id}-email`}>{t('demo.email')}</label>
            <input id={`${id}-email`} name="email" className="input" type="email" autoComplete="email" required maxLength={254} />
            <label htmlFor={`${id}-company`}>{t('demo.company')}</label>
            <input id={`${id}-company`} name="company" className="input" type="text" autoComplete="organization" required maxLength={100} />
            <label htmlFor={`${id}-domain`}>{t('demo.domain')}</label>
            <input
              id={`${id}-domain`}
              name="domain"
              className="input"
              type="text"
              inputMode="url"
              spellCheck={false}
              placeholder={t('demo.domainHint')}
              required
              maxLength={253}
            />
            <label htmlFor={`${id}-message`}>
              {t('demo.message')} <span className="muted">{t('demo.optional')}</span>
            </label>
            <textarea id={`${id}-message`} name="message" className="input" rows={4} maxLength={2000} />
            {/* Honeypot: hidden from people (CSS, not a style attribute: CSP). */}
            <div className="demo-hp" aria-hidden="true">
              <label htmlFor={`${id}-website`}>Website</label>
              <input id={`${id}-website`} name="website" type="text" tabIndex={-1} autoComplete="off" />
            </div>
            <div className="demo-consent">
              <input id={`${id}-privacy`} name="privacy" type="checkbox" required />
              <label htmlFor={`${id}-privacy`}>
                {t('demo.consent')}{' '}
                <Link to="/privacy" target="_blank" rel="noopener">
                  {t('demo.consentLink')}
                </Link>
              </label>
            </div>
            <p className="muted small">{t('demo.keep')}</p>
            <button type="submit" className="btn" disabled={busy}>
              {state === 'proving' ? t('demo.proving') : state === 'sending' ? t('demo.sending') : t('demo.submit')}
            </button>
            <p id={`${id}-err`} className="error" role="alert">
              {error}
            </p>
          </form>
        </section>
      )}
    </article>
  )
}
