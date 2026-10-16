import { useCallback, useEffect, useId, useState, type FormEvent } from 'react'
import { Link } from 'react-router'
import {
  ApiError,
  changeRole,
  createInvitation,
  createOrganization,
  getOrg,
  leaveOrg,
  listInvitations,
  listMembers,
  removeMember,
  renameOrg,
  revokeInvitation,
  switchOrganization,
  type Invitation,
  type Lang,
  type Member,
  type Org,
  type Role,
} from '../api'
import { useAuth } from '../auth'
import { ErrorLine, Field, Panel } from '../components/Form'
import { useLang } from '../i18n'
import { asError, useForm } from '../useForm'
import { usePageMeta } from '../usePageMeta'

const ROLES: Role[] = ['owner', 'admin', 'viewer']

/** Who may remove whom (the API decides; this only hides buttons). */
function canRemove(actor: Role, target: Role, self: boolean) {
  return !self && (actor === 'owner' || (actor === 'admin' && target === 'viewer'))
}

/** The active organization, its members and (if allowed) its pending invitations. */
async function fetchOrgData(lang: Lang) {
  const o = await getOrg(lang)
  const [ms, inv] = await Promise.all([
    listMembers(lang),
    o.permissions.includes('revoke_invitation') ? listInvitations(lang) : Promise.resolve([] as Invitation[]),
  ])
  return { o, ms, inv }
}

export function OrgPage() {
  const { lang, t } = useLang()
  const { state, refresh } = useAuth()
  usePageMeta(t('meta.org'), true)
  const [org, setOrg] = useState<Org | null>(null)
  const [members, setMembers] = useState<Member[]>([])
  const [invitations, setInvitations] = useState<Invitation[]>([])
  const [loadError, setLoadError] = useState<ApiError | null>(null)
  const csrf = state.status === 'user' ? state.me.csrf_token : ''

  const load = useCallback(
    () =>
      fetchOrgData(lang).then(
        ({ o, ms, inv }) => {
          setOrg(o)
          setMembers(ms)
          setInvitations(inv)
          setLoadError(null)
        },
        (e: unknown) => {
          setOrg(null)
          setLoadError(asError(e))
        },
      ),
    [lang],
  )

  useEffect(() => {
    if (state.status !== 'user') return
    let cancelled = false
    fetchOrgData(lang).then(
      ({ o, ms, inv }) => {
        if (cancelled) return
        setOrg(o)
        setMembers(ms)
        setInvitations(inv)
        setLoadError(null)
      },
      (e: unknown) => !cancelled && setLoadError(asError(e)),
    )
    return () => {
      cancelled = true
    }
  }, [state.status, lang])

  /** After a change of organization: the session and this page. */
  const reloadAll = async () => {
    await refresh()
    await load()
  }

  if (state.status === 'loading') return <p className="muted">{t('auth.loading')}</p>
  if (state.status === 'anon') {
    return (
      <Panel title={t('nav.org')}>
        <p>{t('account.needLogin')}</p>
        <Link className="btn" to="/login">
          {t('login.submit')}
        </Link>
      </Panel>
    )
  }
  if (loadError?.code === 'no_organization') {
    return (
      <Panel title={t('nav.org')}>
        <p>{t('org.noOrg')}</p>
        <CreateOrg csrf={csrf} onDone={reloadAll} />
      </Panel>
    )
  }
  if (loadError) {
    return (
      <Panel title={t('nav.org')}>
        <ErrorLine id="org-load" error={loadError} />
      </Panel>
    )
  }
  if (!org) return <p className="muted">{t('auth.loading')}</p>

  const can = (a: Org['permissions'][number]) => org.permissions.includes(a)
  return (
    <Panel title={org.name} wide>
      <p className="muted">{t('org.yourRole', { role: t(`account.role.${org.role}` as const) })}</p>
      {can('rename') && <Rename org={org} csrf={csrf} onDone={reloadAll} />}
      <Members org={org} members={members} csrf={csrf} onChange={load} onLeft={reloadAll} />
      {can('invite') && <Invitations org={org} invitations={invitations} csrf={csrf} onChange={load} />}
      <Switcher key={org.id} current={org.id} csrf={csrf} onDone={reloadAll} />
    </Panel>
  )
}

function Rename({ org, csrf, onDone }: { org: Org; csrf: string; onDone: () => Promise<void> }) {
  const { lang, t } = useLang()
  const f = useForm()
  const [name, setName] = useState(org.name)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    void f.run(async () => {
      await renameOrg(name, csrf, lang)
      await onDone()
    })
  }
  return (
    <form className="auth-form" onSubmit={submit} noValidate>
      <Field label={t('org.name')} value={name} onChange={setName} maxLength={100} invalid={!!f.error} errorId={f.errorId} />
      <button type="submit" className="btn" disabled={f.busy || !name.trim() || name === org.name}>
        {t('org.renameSubmit')}
      </button>
      <ErrorLine id={f.errorId} error={f.error} />
    </form>
  )
}

function Members(props: { org: Org; members: Member[]; csrf: string; onChange: () => Promise<void>; onLeft: () => Promise<void> }) {
  const { lang, t } = useLang()
  const f = useForm()
  const { org, csrf } = props
  const act = (fn: () => Promise<unknown>) =>
    void f.run(async () => {
      await fn()
      await props.onChange()
    })
  const leave = () => {
    if (!window.confirm(t('org.leaveConfirm', { name: org.name }))) return
    void f.run(async () => {
      await leaveOrg(csrf, lang)
      await props.onLeft()
    })
  }
  return (
    <section aria-labelledby="members-title" className="org-section">
      <h2 id="members-title" className="h-section">
        {t('org.members')}
      </h2>
      <p className="muted small">{t('org.roleHelp')}</p>
      <ul className="member-list">
        {props.members.map((m) => (
          <li key={m.user_id} className="member">
            <span className="member-who">
              {m.email} {m.you && <span className="muted">{t('org.you')}</span>}
            </span>
            {org.permissions.includes('change_role') ? (
              <select
                className="input select"
                aria-label={t('org.roleOf', { email: m.email })}
                value={m.role}
                disabled={f.busy}
                onChange={(e) => act(() => changeRole(m.user_id, e.target.value as Role, csrf, lang))}
              >
                {ROLES.map((r) => (
                  <option key={r} value={r}>
                    {t(`account.role.${r}` as const)}
                  </option>
                ))}
              </select>
            ) : (
              <span className="muted">{t(`account.role.${m.role}` as const)}</span>
            )}
            {canRemove(org.role, m.role, m.you) && (
              <button
                type="button"
                className="btn btn-link"
                disabled={f.busy}
                aria-label={t('org.removeLabel', { email: m.email })}
                onClick={() => window.confirm(t('org.removeConfirm', { email: m.email })) && act(() => removeMember(m.user_id, csrf, lang))}
              >
                {t('org.remove')}
              </button>
            )}
          </li>
        ))}
      </ul>
      <ErrorLine id={f.errorId} error={f.error} />
      <button type="button" className="btn btn-link" onClick={leave} disabled={f.busy}>
        {t('org.leave')}
      </button>
    </section>
  )
}

function Invitations(props: { org: Org; invitations: Invitation[]; csrf: string; onChange: () => Promise<void> }) {
  const { lang, t } = useLang()
  const f = useForm()
  const roleId = useId()
  const [email, setEmail] = useState('')
  const [role, setRole] = useState<Role>('viewer')
  const [sent, setSent] = useState('')
  const submit = (e: FormEvent) => {
    e.preventDefault()
    setSent('')
    void f.run(async () => {
      const inv = await createInvitation(email, role, props.csrf, lang)
      setSent(inv.email)
      setEmail('')
      await props.onChange()
    })
  }
  const revoke = (inv: Invitation) =>
    window.confirm(t('org.revokeConfirm', { email: inv.email })) &&
    void f.run(async () => {
      await revokeInvitation(inv.id, props.csrf, lang)
      await props.onChange()
    })
  const date = (s: string) => new Date(s).toLocaleDateString(lang === 'es' ? 'es-ES' : 'en-GB', { day: 'numeric', month: 'long' })
  return (
    <section aria-labelledby="inv-title" className="org-section">
      <h2 id="inv-title" className="h-section">
        {t('org.invitations')}
      </h2>
      <form className="auth-form" onSubmit={submit} noValidate>
        <Field label={t('org.inviteEmail')} type="email" value={email} onChange={setEmail} autoComplete="off" maxLength={254} invalid={!!f.error} errorId={f.errorId} />
        <div className="field">
          <label htmlFor={roleId} className="label">
            {t('org.inviteRole')}
          </label>
          <select id={roleId} className="input select" value={role} onChange={(e) => setRole(e.target.value as Role)}>
            <option value="viewer">{t('account.role.viewer')}</option>
            <option value="admin">{t('account.role.admin')}</option>
          </select>
        </div>
        <button type="submit" className="btn" disabled={f.busy || !email}>
          {f.busy ? t('auth.sending') : t('org.inviteSubmit')}
        </button>
        <ErrorLine id={f.errorId} error={f.error} />
        {sent && (
          <p className="ok" role="status">
            {t('org.invited', { email: sent })}
          </p>
        )}
      </form>
      <h3 className="label">{t('org.pending')}</h3>
      {props.invitations.length === 0 ? (
        <p className="muted">{t('org.noPending')}</p>
      ) : (
        <ul className="member-list">
          {props.invitations.map((inv) => (
            <li key={inv.id} className="member">
              <span className="member-who">{inv.email}</span>
              <span className="muted">
                {t(`account.role.${inv.role}` as const)} · {inv.expired ? t('org.expired') : t('org.expires', { date: date(inv.expires_at) })}
              </span>
              {props.org.permissions.includes('revoke_invitation') && (
                <button type="button" className="btn btn-link" aria-label={t('org.revokeLabel', { email: inv.email })} onClick={() => revoke(inv)}>
                  {t('org.revoke')}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

function Switcher({ current, csrf, onDone }: { current: string; csrf: string; onDone: () => Promise<void> }) {
  const { lang, t } = useLang()
  const { state } = useAuth()
  const f = useForm()
  const selectId = useId()
  const orgs = state.status === 'user' ? state.me.organizations : []
  const [target, setTarget] = useState(current) // reset by key when the active organization changes
  const submit = (e: FormEvent) => {
    e.preventDefault()
    void f.run(async () => {
      await switchOrganization(target, csrf, lang)
      await onDone()
    })
  }
  return (
    <section aria-labelledby="switch-title" className="org-section">
      <h2 id="switch-title" className="h-section">
        {t('org.switch')}
      </h2>
      {orgs.length > 1 && (
        <form className="auth-form" onSubmit={submit} noValidate>
          <div className="field">
            <label htmlFor={selectId} className="label">
              {t('nav.org')}
            </label>
            <select id={selectId} className="input select" value={target} onChange={(e) => setTarget(e.target.value)}>
              {orgs.map((o) => (
                <option key={o.id} value={o.id}>
                  {o.name} · {t(`account.role.${o.role}` as const)}
                </option>
              ))}
            </select>
          </div>
          <button type="submit" className="btn" disabled={f.busy || target === current}>
            {t('org.switchSubmit')}
          </button>
          <ErrorLine id={f.errorId} error={f.error} />
        </form>
      )}
      <CreateOrg csrf={csrf} onDone={onDone} />
    </section>
  )
}

function CreateOrg({ csrf, onDone }: { csrf: string; onDone: () => Promise<void> }) {
  const { lang, t } = useLang()
  const f = useForm()
  const [name, setName] = useState('')
  const submit = (e: FormEvent) => {
    e.preventDefault()
    void f.run(async () => {
      await createOrganization(name, csrf, lang)
      setName('')
      await onDone()
    })
  }
  return (
    <form className="auth-form" onSubmit={submit} noValidate>
      <Field label={t('org.createName')} value={name} onChange={setName} maxLength={100} invalid={!!f.error} errorId={f.errorId} />
      <button type="submit" className="btn" disabled={f.busy || !name.trim()}>
        {t('org.createSubmit')}
      </button>
      <ErrorLine id={f.errorId} error={f.error} />
    </form>
  )
}
