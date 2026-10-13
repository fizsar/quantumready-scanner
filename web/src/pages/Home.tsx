import { useEffect, useId, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { ApiError, createScan, getTargets } from '../api'
import { useLang } from '../i18n'
import { usePageMeta } from '../usePageMeta'
import { errorMessage } from '../verdict'

export function Home() {
  const { lang, t } = useLang()
  const navigate = useNavigate()
  const inputId = useId()
  const errorId = useId()
  usePageMeta(t('meta.home'))

  const [targets, setTargets] = useState<string[] | null>(null)
  const [host, setHost] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<ApiError | null>(null)

  useEffect(() => {
    let cancelled = false
    getTargets()
      .then((r) => !cancelled && setTargets(r.hosts))
      .catch((e: unknown) => !cancelled && (setTargets([]), setError(e instanceof ApiError ? e : new ApiError(0, 'network', ''))))
    return () => {
      cancelled = true
    }
  }, [])

  const scan = async (h: string) => {
    setBusy(true)
    setError(null)
    try {
      const s = await createScan(h, lang)
      navigate(`/scans/${s.id}`)
    } catch (e) {
      setError(e instanceof ApiError ? e : new ApiError(0, 'network', ''))
      setBusy(false)
    }
  }

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (host.trim()) void scan(host)
  }

  return (
    <div className="home">
      <h1 className="brand">
        <span className="logo" aria-hidden="true">
          Q&#770;
        </span>
        <span className="wordmark">QuantumReady</span>
      </h1>

      <section className="terminal" aria-labelledby="home-title">
        <p className="prompt">
          <span className="user">guest@quantumready</span>:<span className="path">~</span>$ ./scan --pqc
        </p>
        <h2 id="home-title" className="h-title">
          {t('home.title')}
        </h2>
        <p className="lead">{t('home.lead')}</p>

        <p className="muted">{t('home.demo')}</p>
        {targets === null ? (
          <p className="muted">{t('home.loadingTargets')}</p>
        ) : (
          <ul className="targets">
            {targets.map((h) => (
              <li key={h}>
                <button type="button" className="btn btn-target" disabled={busy} onClick={() => void scan(h)} aria-label={t('home.scanThis', { host: h })}>
                  {h}
                </button>
              </li>
            ))}
          </ul>
        )}

        <form className="scan-form" onSubmit={onSubmit} noValidate>
          <label htmlFor={inputId} className="label">
            {t('home.formLabel')}
          </label>
          <div className="row">
            <input
              id={inputId}
              className="input"
              type="text"
              inputMode="url"
              autoComplete="url"
              spellCheck={false}
              placeholder={t('home.placeholder')}
              value={host}
              onChange={(e) => setHost(e.target.value)}
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? errorId : undefined}
              maxLength={253}
            />
            <button type="submit" className="btn" disabled={busy || !host.trim()}>
              {busy ? t('home.submitting') : t('home.submit')}
            </button>
          </div>
          <p id={errorId} className="error" role="alert">
            {error ? errorMessage(error, t) : ''}
          </p>
        </form>
      </section>
    </div>
  )
}
