import { Link } from 'react-router'
import { useAccountsEnabled } from '../features'
import { useLang } from '../i18n'
import { usePageMeta } from '../usePageMeta'

// The IP the scans come from (the VPS) and its reverse DNS name.
const SCANNER_IP = '149.56.13.98'
const SCANNER_PTR = 'quantumready.fizsar.com'

/** /about-scans: what the scanner does, from where, how often, and how to opt out. */
export function AboutScans() {
  const { t } = useLang()
  const accounts = useAccountsEnabled()
  usePageMeta(t('meta.about'))
  const sections: [string, string[]][] = [
    [t('about.what.title'), [t('about.what.1'), t('about.what.2'), t('about.what.3')]],
    [t('about.from.title'), [t('about.from.1', { ip: SCANNER_IP, ptr: SCANNER_PTR }), t('about.from.2')]],
    [t('about.often.title'), [t('about.often.1'), t('about.often.2'), t('about.often.3')]],
    [t('about.keep.title'), [t('about.keep.1')]],
    // The owner's opt-out from /org needs an account: without accounts, only the email.
    [t('about.optout.title'), accounts ? [t('about.optout.1'), t('about.optout.2')] : [t('about.optout.1')]],
  ]
  return (
    <section className="terminal" aria-labelledby="about-title">
      <h1 id="about-title" className="h-title">
        {t('about.title')}
      </h1>
      <p className="lead">{t('about.lead')}</p>
      {sections.map(([title, paragraphs]) => (
        <section key={title} className="org-section" aria-label={title}>
          <h2 className="h-section">{title}</h2>
          {paragraphs.map((p) => (
            <p key={p}>{p}</p>
          ))}
        </section>
      ))}
      <p className="muted small">
        <a href="/.well-known/security.txt">security.txt</a> · <a href="mailto:soporte@fizsar.com">soporte@fizsar.com</a> ·{' '}
        <Link to="/">{t('footer.newScan')}</Link>
      </p>
    </section>
  )
}
