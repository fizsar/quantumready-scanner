import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router'
import { ApiError, acceptInvitation, login, previewInvitation, registerInvited, type InvitationPreview } from '../api'
import { authErrorMessage, useAuth, useHashToken } from '../auth'
import { ErrorLine, Field, Panel } from '../components/Form'
import { useLang } from '../i18n'
import { asError, useForm } from '../useForm'
import { usePageMeta } from '../usePageMeta'

/** /invite#token=…: what the invitation offers, then accept, sign in or sign up. */
export function InvitePage() {
  const { lang, t } = useLang()
  const { state, signedIn, refresh, logout } = useAuth()
  const navigate = useNavigate()
  usePageMeta(t('meta.invite'), true)
  const token = useHashToken()
  const [preview, setPreview] = useState<InvitationPreview | ApiError | null>(
    token ? null : new ApiError(400, 'invalid_token', t('verify.noToken')),
  )
  const asked = useRef(false)
  const f = useForm()
  const [password, setPassword] = useState('')
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [notice, setNotice] = useState('')

  useEffect(() => {
    if (!token || asked.current) return
    asked.current = true
    previewInvitation(token, lang).then(setPreview, (e: unknown) => setPreview(asError(e)))
  }, [token, lang])

  if (preview === null) {
    return (
      <Panel title={t('invite.title')}>
        <p className="progress" role="status">
          {t('invite.checking')}
        </p>
      </Panel>
    )
  }
  if (preview instanceof ApiError) {
    return (
      <Panel title={t('invite.title')}>
        <p className="error" role="alert">
          {authErrorMessage(preview, t)}
        </p>
        <Link to="/">{t('footer.newScan')}</Link>
      </Panel>
    )
  }

  const p = preview
  const vars = { org: p.organization, role: t(`account.role.${p.role}` as const), inviter: p.inviter_email ?? '' }
  const lead = p.inviter_email ? t('invite.lead', vars) : t('invite.leadNoInviter', vars)
  const accept = (csrf: string) =>
    f.run(async () => {
      await acceptInvitation(token, csrf, lang)
      await refresh()
      navigate('/org')
    })

  let body
  if (state.status === 'loading') {
    body = <p className="muted">{t('auth.loading')}</p>
  } else if (state.status === 'user' && state.me.user.email === p.email) {
    body = (
      <button type="button" className="btn" disabled={f.busy} onClick={() => void accept(state.me.csrf_token)}>
        {t('invite.accept')}
      </button>
    )
  } else if (state.status === 'user') {
    body = (
      <>
        <p>{t('invite.otherAccount', { email: p.email, current: state.me.user.email })}</p>
        <button type="button" className="btn btn-link" onClick={() => void logout()}>
          {t('account.logout')}
        </button>
      </>
    )
  } else {
    const submit = (e: FormEvent) => {
      e.preventDefault()
      setNotice('')
      void f.run(async () => {
        if (mode === 'register') {
          const r = await registerInvited(p.email, password, token, lang)
          if (r.status !== 'registered') {
            // The address already has an account: sign in instead.
            setMode('login')
            setNotice(t('invite.exists'))
            return
          }
          // Joined already; just open the session.
          signedIn(await login(p.email, password, lang))
          navigate('/org')
          return
        }
        signedIn(await login(p.email, password, lang)) // then the Accept button shows
      })
    }
    body = (
      <form className="auth-form" onSubmit={submit} noValidate>
        <Field label={t('auth.email')} type="email" value={p.email} onChange={() => {}} readOnly autoComplete="username" />
        <Field
          label={t('auth.password')}
          type="password"
          value={password}
          onChange={setPassword}
          autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
          help={mode === 'register' ? t('auth.passwordHelp') : undefined}
          maxLength={128}
          invalid={!!f.error}
          errorId={f.errorId}
        />
        <button type="submit" className="btn" disabled={f.busy || !password}>
          {mode === 'login' ? t('login.submit') : t('invite.createSubmit')}
        </button>
        <ErrorLine id={f.errorId} error={f.error} />
        {notice && (
          <p className="ok" role="status">
            {notice}
          </p>
        )}
        <button type="button" className="btn btn-link" onClick={() => (setMode(mode === 'login' ? 'register' : 'login'), f.setError(null))}>
          {mode === 'login' ? t('invite.newAccount') : t('invite.haveAccount')}
        </button>
      </form>
    )
  }

  return (
    <Panel title={t('invite.title')}>
      <p className="lead">{lead}</p>
      {body}
      {state.status === 'user' && state.me.user.email === p.email && <ErrorLine id={f.errorId} error={f.error} />}
    </Panel>
  )
}
