import { useEffect, useId, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { Link, Navigate, useNavigate } from 'react-router'
import {
  ApiError,
  changePassword,
  forgotPassword,
  login,
  register,
  resendVerification,
  resetPassword,
  verifyEmail,
} from '../api'
import { authErrorMessage, useAuth, useHashToken } from '../auth'
import { useLang } from '../i18n'
import { usePageMeta } from '../usePageMeta'

const asError = (e: unknown) => (e instanceof ApiError ? e : new ApiError(0, 'network', ''))

function Field(props: {
  label: string
  type?: string
  value: string
  onChange: (v: string) => void
  autoComplete?: string
  help?: string
  required?: boolean
  maxLength?: number
  invalid?: boolean
  errorId?: string
}) {
  const id = useId()
  const helpId = useId()
  const described = [props.help ? helpId : '', props.invalid && props.errorId ? props.errorId : ''].filter(Boolean).join(' ')
  return (
    <div className="field">
      <label htmlFor={id} className="label">
        {props.label}
      </label>
      <input
        id={id}
        className="input"
        type={props.type ?? 'text'}
        value={props.value}
        onChange={(e) => props.onChange(e.target.value)}
        autoComplete={props.autoComplete}
        required={props.required}
        maxLength={props.maxLength}
        spellCheck={false}
        aria-invalid={props.invalid || undefined}
        aria-describedby={described || undefined}
      />
      {props.help && (
        <p id={helpId} className="muted small">
          {props.help}
        </p>
      )}
    </div>
  )
}

/** A titled terminal panel for the account pages. */
function Panel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="terminal auth" aria-labelledby="auth-title">
      <h1 id="auth-title" className="h-title">
        {title}
      </h1>
      {children}
    </section>
  )
}

/** Form state shared by the pages: busy flag, error and its alert. */
function useForm() {
  const errorId = useId()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<ApiError | null>(null)
  const run = async (f: () => Promise<void>) => {
    setBusy(true)
    setError(null)
    try {
      await f()
    } catch (e) {
      setError(asError(e))
    } finally {
      setBusy(false)
    }
  }
  return { busy, error, setError, run, errorId }
}

function ErrorLine({ id, error }: { id: string; error: ApiError | null }) {
  const { t } = useLang()
  return (
    <p id={id} className="error" role="alert">
      {error ? authErrorMessage(error, t) : ''}
    </p>
  )
}

export function LoginPage() {
  const { lang, t } = useLang()
  const { state, signedIn } = useAuth()
  const navigate = useNavigate()
  usePageMeta(t('meta.login'))
  const f = useForm()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [resent, setResent] = useState(false)

  if (state.status === 'user') return <Navigate to="/account" replace />

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    setResent(false)
    void f.run(async () => {
      signedIn(await login(email, password, lang))
      navigate('/account')
    })
  }
  const resend = () =>
    void f.run(async () => {
      await resendVerification(email, lang)
      setResent(true)
    })

  return (
    <Panel title={t('login.title')}>
      <form className="auth-form" onSubmit={onSubmit} noValidate>
        <Field label={t('auth.email')} type="email" value={email} onChange={setEmail} autoComplete="username" required maxLength={254} invalid={!!f.error} errorId={f.errorId} />
        <Field label={t('auth.password')} type="password" value={password} onChange={setPassword} autoComplete="current-password" required maxLength={128} invalid={!!f.error} errorId={f.errorId} />
        <button type="submit" className="btn" disabled={f.busy || !email || !password}>
          {f.busy ? t('auth.sending') : t('login.submit')}
        </button>
        <ErrorLine id={f.errorId} error={f.error} />
        {f.error?.code === 'email_not_verified' && (
          <button type="button" className="btn btn-link" onClick={resend} disabled={f.busy}>
            {t('login.resend')}
          </button>
        )}
        {resent && (
          <p className="ok" role="status">
            {t('resend.done')}
          </p>
        )}
      </form>
      <p>
        <Link to="/forgot">{t('login.forgot')}</Link>
      </p>
      <p>
        <Link to="/register">{t('login.noAccount')}</Link>
      </p>
    </Panel>
  )
}

export function RegisterPage() {
  const { lang, t } = useLang()
  usePageMeta(t('meta.register'))
  const f = useForm()
  const [v, setV] = useState({ email: '', password: '', name: '', organization: '', signup_code: '' })
  const [done, setDone] = useState('')
  const set = (k: keyof typeof v) => (x: string) => setV((s) => ({ ...s, [k]: x }))

  if (done) {
    return (
      <Panel title={t('register.title')}>
        <p className="ok" role="status">
          {t('register.done', { email: done })}
        </p>
      </Panel>
    )
  }
  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    void f.run(async () => {
      await register(v, lang)
      setDone(v.email.trim())
    })
  }
  const invalid = (codes: string[]) => !!f.error && codes.some((c) => f.error!.code === c || f.error!.code.startsWith(c))
  return (
    <Panel title={t('register.title')}>
      <p className="muted">{t('auth.codeHelp')}</p>
      <form className="auth-form" onSubmit={onSubmit} noValidate>
        <Field label={t('auth.code')} value={v.signup_code} onChange={set('signup_code')} autoComplete="off" required maxLength={200} invalid={invalid(['invalid_signup_code'])} errorId={f.errorId} />
        <Field label={t('auth.email')} type="email" value={v.email} onChange={set('email')} autoComplete="email" required maxLength={254} invalid={invalid(['invalid_email'])} errorId={f.errorId} />
        <Field label={t('auth.password')} type="password" value={v.password} onChange={set('password')} autoComplete="new-password" required maxLength={128} help={t('auth.passwordHelp')} invalid={invalid(['password_'])} errorId={f.errorId} />
        <Field label={t('auth.name')} value={v.name} onChange={set('name')} autoComplete="name" maxLength={100} invalid={invalid(['invalid_name'])} errorId={f.errorId} />
        <Field label={t('auth.org')} value={v.organization} onChange={set('organization')} autoComplete="organization" maxLength={100} invalid={invalid(['invalid_organization'])} errorId={f.errorId} />
        <button type="submit" className="btn" disabled={f.busy || !v.email || !v.password || !v.signup_code}>
          {f.busy ? t('auth.sending') : t('register.submit')}
        </button>
        <ErrorLine id={f.errorId} error={f.error} />
      </form>
      <p>
        <Link to="/login">{t('register.haveAccount')}</Link>
      </p>
    </Panel>
  )
}

export function VerifyPage() {
  const { lang, t } = useLang()
  usePageMeta(t('meta.verify'), true)
  const token = useHashToken()
  const [state, setState] = useState<'working' | 'ok' | ApiError>(token ? 'working' : new ApiError(400, 'invalid_token', t('verify.noToken')))
  const sent = useRef(false)

  useEffect(() => {
    // Once, even in StrictMode: the token works only once.
    if (!token || sent.current) return
    sent.current = true
    verifyEmail(token, lang).then(
      () => setState('ok'),
      (e: unknown) => setState(asError(e)),
    )
  }, [token, lang])

  return (
    <Panel title={t('verify.title')}>
      {state === 'working' && (
        <p className="progress" role="status">
          {t('verify.working')}
        </p>
      )}
      {state === 'ok' && (
        <>
          <p className="ok" role="status">
            {t('verify.ok')}
          </p>
          <Link className="btn" to="/login">
            {t('login.submit')}
          </Link>
        </>
      )}
      {state instanceof ApiError && (
        <>
          <p className="error" role="alert">
            {authErrorMessage(state, t)}
          </p>
          <Link to="/login">{t('login.resend')}</Link>
        </>
      )}
    </Panel>
  )
}

export function ForgotPage() {
  const { lang, t } = useLang()
  usePageMeta(t('meta.forgot'))
  const f = useForm()
  const [email, setEmail] = useState('')
  const [done, setDone] = useState(false)
  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    void f.run(async () => {
      await forgotPassword(email, lang)
      setDone(true)
    })
  }
  return (
    <Panel title={t('forgot.title')}>
      {done ? (
        <p className="ok" role="status">
          {t('forgot.done')}
        </p>
      ) : (
        <>
          <p className="lead">{t('forgot.lead')}</p>
          <form className="auth-form" onSubmit={onSubmit} noValidate>
            <Field label={t('auth.email')} type="email" value={email} onChange={setEmail} autoComplete="email" required maxLength={254} invalid={!!f.error} errorId={f.errorId} />
            <button type="submit" className="btn" disabled={f.busy || !email}>
              {f.busy ? t('auth.sending') : t('forgot.submit')}
            </button>
            <ErrorLine id={f.errorId} error={f.error} />
          </form>
        </>
      )}
    </Panel>
  )
}

export function ResetPage() {
  const { lang, t } = useLang()
  const { signedOut } = useAuth()
  usePageMeta(t('meta.reset'), true)
  const token = useHashToken()
  const f = useForm()
  const [password, setPassword] = useState('')
  const [done, setDone] = useState(false)

  if (!token) {
    return (
      <Panel title={t('reset.title')}>
        <p className="error" role="alert">
          {t('verify.noToken')}
        </p>
        <Link to="/forgot">{t('reset.again')}</Link>
      </Panel>
    )
  }
  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    void f.run(async () => {
      await resetPassword(token, password, lang)
      signedOut() // every session ended, this one too
      setDone(true)
    })
  }
  return (
    <Panel title={t('reset.title')}>
      {done ? (
        <>
          <p className="ok" role="status">
            {t('reset.ok')}
          </p>
          <Link className="btn" to="/login">
            {t('login.submit')}
          </Link>
        </>
      ) : (
        <form className="auth-form" onSubmit={onSubmit} noValidate>
          <Field label={t('auth.passwordNew')} type="password" value={password} onChange={setPassword} autoComplete="new-password" required maxLength={128} help={t('auth.passwordHelp')} invalid={!!f.error} errorId={f.errorId} />
          <button type="submit" className="btn" disabled={f.busy || !password}>
            {f.busy ? t('auth.sending') : t('reset.submit')}
          </button>
          <ErrorLine id={f.errorId} error={f.error} />
          {f.error?.code === 'invalid_token' && <Link to="/forgot">{t('reset.again')}</Link>}
        </form>
      )}
    </Panel>
  )
}

export function AccountPage() {
  const { lang, t } = useLang()
  const { state, logout, signedOut } = useAuth()
  usePageMeta(t('meta.account'), true)
  const f = useForm()
  const out = useForm()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [changed, setChanged] = useState(false)

  if (state.status === 'loading') return <p className="muted">{t('auth.loading')}</p>
  if (state.status === 'anon') {
    return (
      <Panel title={t('account.title')}>
        <p>{t('account.needLogin')}</p>
        <Link className="btn" to="/login">
          {t('login.submit')}
        </Link>
      </Panel>
    )
  }
  const { me } = state
  const onChange = (e: FormEvent) => {
    e.preventDefault()
    setChanged(false)
    void f.run(async () => {
      try {
        await changePassword(current, next, me.csrf_token, lang)
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) signedOut()
        throw err
      }
      setCurrent('')
      setNext('')
      setChanged(true)
    })
  }
  return (
    <Panel title={t('account.title')}>
      <p className="host">{me.user.email}</p>
      <h2 className="h-section">{t('account.orgs')}</h2>
      <ul className="orgs">
        {me.organizations.map((o) => (
          <li key={o.id}>
            {o.name} <span className="muted">· {t(`account.role.${o.role}` as const)}</span>
          </li>
        ))}
      </ul>

      <h2 className="h-section">{t('account.change')}</h2>
      <form className="auth-form" onSubmit={onChange} noValidate>
        <Field label={t('auth.passwordCurrent')} type="password" value={current} onChange={setCurrent} autoComplete="current-password" required maxLength={128} invalid={f.error?.code === 'wrong_password'} errorId={f.errorId} />
        <Field label={t('auth.passwordNew')} type="password" value={next} onChange={setNext} autoComplete="new-password" required maxLength={128} help={t('auth.passwordHelp')} invalid={!!f.error && f.error.code.startsWith('password_')} errorId={f.errorId} />
        <button type="submit" className="btn" disabled={f.busy || !current || !next}>
          {f.busy ? t('auth.sending') : t('account.changeSubmit')}
        </button>
        <ErrorLine id={f.errorId} error={f.error} />
        {changed && (
          <p className="ok" role="status">
            {t('account.changed')}
          </p>
        )}
      </form>

      <button type="button" className="btn btn-link" disabled={out.busy} onClick={() => void out.run(logout)}>
        {t('account.logout')}
      </button>
      <ErrorLine id={out.errorId} error={out.error} />
    </Panel>
  )
}
