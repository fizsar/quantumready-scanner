import { Link, useParams } from 'react-router'
import { Report } from '../components/Report'
import { useLang } from '../i18n'
import { usePageMeta } from '../usePageMeta'
import { useScan } from '../useScan'
import { errorMessage } from '../verdict'

/** A scan result. org: one of the active organization's scans (/org/scans/:id). */
export function ScanPage({ org = false }: { org?: boolean }) {
  const { id = '' } = useParams()
  const { lang, t } = useLang()
  const { scan, error, reconnecting } = useScan(id, lang, org)
  const back = org ? '/org' : '/'
  usePageMeta(t('meta.scan', { host: scan?.host ?? '…' }), true)

  if (error && !scan) {
    return (
      <section className="terminal" aria-labelledby="scan-error">
        <h1 id="scan-error" className="h-title">
          {t('failed.title')}
        </h1>
        <p className="error" role="alert">
          {errorMessage(error, t)}
        </p>
        <Link className="btn" to={back}>
          {t('footer.newScan')}
        </Link>
      </section>
    )
  }

  if (scan?.status === 'done' && scan.report) {
    return <Report scan={scan} report={scan.report} />
  }

  if (scan?.status === 'failed') {
    return (
      <section className="terminal" aria-labelledby="scan-failed">
        <p className="prompt">
          <span className="user">guest@quantumready</span>:<span className="path">~</span>$ ./scan {scan.host}
        </p>
        <h1 id="scan-failed" className="h-title tone-bad">
          {t('failed.title')}
        </h1>
        <p role="alert">{scan.error?.message}</p>
        <Link className="btn" to={back}>
          {t('failed.again')}
        </Link>
      </section>
    )
  }

  // Loading, queued or running: progress, announced to screen readers.
  let progress = t('progress.queued')
  if (scan?.status === 'running') progress = t('progress.running', { host: scan.host })
  if (scan?.status === 'queued' && scan.retries > 0) progress = t('progress.retrying')
  if (reconnecting) progress = t('progress.reconnecting')
  return (
    <section className="terminal" aria-busy="true" aria-labelledby="scan-progress">
      <p className="prompt">
        <span className="user">guest@quantumready</span>:<span className="path">~</span>$ ./scan {scan?.host ?? ''}
      </p>
      <p id="scan-progress" className="progress" role="status" aria-live="polite">
        <span className="spinner" aria-hidden="true" />
        {progress}
      </p>
    </section>
  )
}
