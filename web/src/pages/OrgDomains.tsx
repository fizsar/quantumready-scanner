import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router'
import {
  addDomain,
  createOrgScan,
  deleteDomain,
  getDomain,
  listDomains,
  listOrgScans,
  verifyDomain,
  type Domain,
  type Org,
  type ScanSummary,
} from '../api'
import { ErrorLine, Field } from '../components/Form'
import { useLang } from '../i18n'
import { asError, useForm } from '../useForm'

const POLL_MS = 2000
const POLL_MAX = 15 // ~30 s: the worker checks within seconds

function useDate() {
  const { lang } = useLang()
  return (s: string) => new Date(s).toLocaleDateString(lang === 'es' ? 'es-ES' : 'en-GB', { day: 'numeric', month: 'long', year: 'numeric' })
}

/** Domains of the active organization: add, verify (TXT), remove. */
export function DomainsSection({ org, csrf, onChange }: { org: Org; csrf: string; onChange: (ds: Domain[]) => void }) {
  const { lang, t } = useLang()
  const f = useForm()
  const [domains, setDomains] = useState<Domain[] | null>(null)
  const [name, setName] = useState('')
  const manage = org.permissions.includes('manage_domains')
  const alive = useRef(true)

  const update = (ds: Domain[]) => {
    setDomains(ds)
    onChange(ds)
  }
  useEffect(() => {
    alive.current = true
    listDomains(lang).then(
      (ds) => alive.current && update(ds),
      (e: unknown) => alive.current && f.setError(asError(e)),
    )
    return () => {
      alive.current = false
    }
    // Load once per organization and language.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [org.id, lang])

  const replace = (d: Domain) => update((domains ?? []).map((x) => (x.id === d.id ? d : x)))

  const add = (e: FormEvent) => {
    e.preventDefault()
    void f.run(async () => {
      const d = await addDomain(name, csrf, lang)
      setName('')
      update([...(domains ?? []), d].sort((a, b) => a.name.localeCompare(b.name)))
    })
  }
  const check = (d: Domain) =>
    void f.run(async () => {
      let cur = await verifyDomain(d.id, csrf, lang)
      replace(cur)
      // The worker checks the record within seconds; wait for its answer.
      for (let i = 0; i < POLL_MAX && cur.check_pending && alive.current; i++) {
        await new Promise((r) => setTimeout(r, POLL_MS))
        cur = await getDomain(d.id, lang)
        if (alive.current) replace(cur)
      }
    })
  const remove = (d: Domain) =>
    window.confirm(t('domains.deleteConfirm', { name: d.name })) &&
    void f.run(async () => {
      await deleteDomain(d.id, csrf, lang)
      update((domains ?? []).filter((x) => x.id !== d.id))
    })

  return (
    <section aria-labelledby="domains-title" className="org-section">
      <h2 id="domains-title" className="h-section">
        {t('domains.title')}
      </h2>
      <p className="muted small">{t('domains.lead')}</p>
      {domains === null ? (
        <p className="muted">{t('auth.loading')}</p>
      ) : domains.length === 0 ? (
        <p className="muted">{t('domains.none')}</p>
      ) : (
        <ul className="member-list">
          {domains.map((d) => (
            <DomainItem key={d.id} d={d} manage={manage} busy={f.busy} onCheck={() => check(d)} onRemove={() => remove(d)} />
          ))}
        </ul>
      )}
      {manage && (
        <form className="auth-form" onSubmit={add} noValidate>
          <Field label={t('domains.add')} value={name} onChange={setName} autoComplete="off" maxLength={253} invalid={!!f.error} errorId={f.errorId} />
          <button type="submit" className="btn" disabled={f.busy || !name.trim()}>
            {t('domains.addSubmit')}
          </button>
        </form>
      )}
      <ErrorLine id={f.errorId} error={f.error} />
    </section>
  )
}

function DomainItem(props: { d: Domain; manage: boolean; busy: boolean; onCheck: () => void; onRemove: () => void }) {
  const { t } = useLang()
  const date = useDate()
  const { d } = props
  const [copied, setCopied] = useState('')
  const copy = (what: string, text: string) => {
    void navigator.clipboard?.writeText(text).then(() => setCopied(what), () => {})
  }
  let status = t('domains.status.pending')
  if (d.status === 'verified') status = t('domains.status.verified')
  if (d.status === 'lost') status = t('domains.status.lost', { date: d.grace_until ? date(d.grace_until) : '' })
  const tone = d.status === 'verified' ? 'badge-yes' : 'badge-no'
  return (
    <li className="domain">
      <div className="member">
        <span className="member-who">{d.name}</span>
        <span className={`badge ${tone}`}>{status}</span>
        {props.manage && (
          <button type="button" className="btn btn-link" disabled={props.busy} aria-label={t('domains.deleteLabel', { name: d.name })} onClick={props.onRemove}>
            {t('domains.delete')}
          </button>
        )}
      </div>
      {d.check_pending && (
        <p className="progress small" role="status">
          <span className="spinner" aria-hidden="true" />
          {t('domains.checking')}
        </p>
      )}
      {!d.check_pending && d.last_check_error && d.status !== 'verified' && (
        <p className="error small" role="alert">
          {d.last_check_error.message}
        </p>
      )}
      {d.status !== 'verified' && props.manage && (
        <div className="record">
          <p className="small">{t('domains.instructions')}</p>
          <dl className="record-fields">
            <dt>{t('domains.recordType')}</dt>
            <dd>
              <code>TXT</code>
            </dd>
            <dt>{t('domains.recordName')}</dt>
            <dd>
              <code>{d.record.name}</code>{' '}
              <button type="button" className="btn btn-link small" onClick={() => copy('name', d.record.name)}>
                {copied === 'name' ? t('domains.copied') : t('domains.copy')}
              </button>
            </dd>
            <dt>{t('domains.recordValue')}</dt>
            <dd>
              <code>{d.record.value}</code>{' '}
              <button type="button" className="btn btn-link small" onClick={() => copy('value', d.record.value)}>
                {copied === 'value' ? t('domains.copied') : t('domains.copy')}
              </button>
            </dd>
          </dl>
          <button type="button" className="btn" disabled={props.busy || d.check_pending} onClick={props.onCheck}>
            {t('domains.check')}
          </button>
        </div>
      )}
      {d.status === 'verified' && (
        <p className="muted small">
          {d.last_checked_at ? t('domains.checkedAt', { date: date(d.last_checked_at) }) + ' ' : ''}
          {t('domains.keepRecord')}
        </p>
      )}
    </li>
  )
}

/** Full scans of verified domains, and the organization's history. */
export function OrgScansSection({ org, csrf, verified }: { org: Org; csrf: string; verified: string[] }) {
  const { lang, t } = useLang()
  const date = useDate()
  const navigate = useNavigate()
  const f = useForm()
  const hostId = useId()
  const [host, setHost] = useState('')
  const [scans, setScans] = useState<ScanSummary[] | null>(null)

  useEffect(() => {
    let cancelled = false
    listOrgScans(lang).then(
      (s) => !cancelled && setScans(s),
      () => !cancelled && setScans([]),
    )
    return () => {
      cancelled = true
    }
  }, [org.id, lang])

  const submit = (e: FormEvent) => {
    e.preventDefault()
    void f.run(async () => {
      const s = await createOrgScan(host, csrf, lang)
      navigate(`/org/scans/${s.id}`)
    })
  }
  const statusText = (s: ScanSummary) =>
    s.status === 'done' && s.grade ? t('scans.grade', { grade: s.grade, score: s.score ?? 0 }) : t(`scans.status.${s.status}` as const)

  return (
    <section aria-labelledby="scans-title" className="org-section">
      <h2 id="scans-title" className="h-section">
        {t('scans.title')}
      </h2>
      {org.permissions.includes('manage_domains') &&
        (verified.length === 0 ? (
          <p className="muted small">{t('scans.needDomain')}</p>
        ) : (
          <form className="auth-form" onSubmit={submit} noValidate>
            <p className="muted small">{t('scans.lead', { domains: verified.join(', ') })}</p>
            <label htmlFor={hostId} className="label">
              {t('scans.host')}
            </label>
            <div className="row">
              <input
                id={hostId}
                className="input"
                value={host}
                placeholder={verified[0]}
                onChange={(e) => setHost(e.target.value)}
                spellCheck={false}
                maxLength={253}
                aria-invalid={f.error ? true : undefined}
                aria-describedby={f.error ? f.errorId : undefined}
              />
              <button type="submit" className="btn" disabled={f.busy || !host.trim()}>
                {t('scans.submit')}
              </button>
            </div>
            <ErrorLine id={f.errorId} error={f.error} />
          </form>
        ))}
      <h3 className="label">{t('scans.history')}</h3>
      {scans === null ? (
        <p className="muted">{t('auth.loading')}</p>
      ) : scans.length === 0 ? (
        <p className="muted">{t('scans.none')}</p>
      ) : (
        <ul className="member-list">
          {scans.map((s) => (
            <li key={s.id} className="member">
              <Link className="member-who" to={`/org/scans/${s.id}`}>
                {s.host}
              </Link>
              <span>{statusText(s)}</span>
              <span className="muted small">
                {date(s.created_at)} · {t(`scans.intensity.${s.intensity}` as const)}
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
