import { useId } from 'react'
import { Link } from 'react-router'
import { useShowcaseT } from '../i18n.showcase'
import { DemoCta } from './DemoButton'
import './HostNotAllowed.css'

const CONTACT = 'soporte@fizsar.com'

/**
 * Shown instead of a bare error when the API answers 403 host_not_allowed:
 * what happens and why, the demo domains to scan instead and where to read
 * more. targets is the home page's list from GET /api/targets, the only list
 * of demo domains in the web, so it always matches the allowlist in force.
 */
export function HostNotAllowed({ targets, busy, onScan }: { targets: string[] | null; busy: boolean; onScan: (host: string) => void }) {
  const { t } = useShowcaseT()
  const id = useId()
  return (
    <section className="hna" aria-labelledby={`${id}-t`} aria-live="polite">
      <h3 id={`${id}-t`} className="hna-title">
        {t('hna.title')}
      </h3>
      <p>{t('hna.why')}</p>
      <DemoCta from="host_not_allowed" />
      {targets && targets.length > 0 && (
        <>
          <p>{t('hna.meanwhile')}</p>
          <ul className="hna-targets">
            {targets.map((h) => (
              <li key={h}>
                <button type="button" className="btn btn-target" disabled={busy} onClick={() => onScan(h)}>
                  {t('hna.scan', { host: h })}
                </button>
              </li>
            ))}
          </ul>
        </>
      )}
      <ul className="hna-links">
        <li>
          <Link to="/examples">{t('hna.examples')}</Link>
        </li>
        <li>
          <Link to="/architecture">{t('hna.architecture')}</Link>
        </li>
        <li>
          {t('hna.contact')} <a href={`mailto:${CONTACT}`}>{CONTACT}</a>
        </li>
      </ul>
    </section>
  )
}
