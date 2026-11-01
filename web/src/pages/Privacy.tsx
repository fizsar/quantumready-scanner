import type { ReactNode } from 'react'
import { useDemoEnabled } from '../features'
import { useShowcaseT, type ShowcaseKey } from '../i18n.showcase'
import { usePageMeta } from '../usePageMeta'
import './Privacy.css'

const CONTACT = 'soporte@fizsar.com'

/** /privacy: what the site measures today (M1), in plain words. Not the full policy. */
export function Privacy() {
  const { t } = useShowcaseT()
  usePageMeta(t('privacy.meta.title'), false, t('privacy.meta.description'))
  const demo = useDemoEnabled() === true

  const list = (keys: ShowcaseKey[]) => (
    <ul className="privacy-list">
      {keys.map((k) => (
        <li key={k}>{t(k)}</li>
      ))}
    </ul>
  )

  return (
    <article className="privacy" aria-labelledby="privacy-title">
      <p className="prompt">
        <span className="user">guest@quantumready</span>:<span className="path">~</span>$ cat PRIVACY.md
      </p>
      <h1 id="privacy-title" className="h-title">
        {t('privacy.title')}
      </h1>
      <p className="muted">{t('privacy.lead')}</p>

      <Section id="privacy-counted" title={t('privacy.counted.title')}>
        <p>{t('privacy.counted.lead')}</p>
        {list(['privacy.counted.pages', 'privacy.counted.source', 'privacy.counted.events', ...(demo ? (['privacy.counted.demo'] as const) : [])])}
        <p>{t('privacy.counted.how')}</p>
      </Section>

      <Section id="privacy-not" title={t('privacy.not.title')}>
        {list(['privacy.not.cookies', 'privacy.not.ip', 'privacy.not.thirdParty'])}
      </Section>

      <Section id="privacy-gpc" title={t('privacy.gpc.title')}>
        <p>{t('privacy.gpc.p')}</p>
      </Section>

      <Section id="privacy-blocked" title={t('privacy.blocked.title')}>
        <p>{t('privacy.blocked.p1')}</p>
        <p>{t('privacy.blocked.p2')}</p>
      </Section>

      <Section id="privacy-scans" title={t('privacy.scans.title')}>
        <p>{t('privacy.scans.p')}</p>
        <p>{t('privacy.scans.how')}</p>
      </Section>

      {demo && (
        <Section id="privacy-demo" title={t('privacy.demo.title')}>
          <p>{t('privacy.demo.what')}</p>
          <p>{t('privacy.demo.keep')}</p>
          <p>{t('privacy.demo.rights')}</p>
          <p>{t('privacy.demo.spam')}</p>
        </Section>
      )}

      <Section id="privacy-where" title={t('privacy.where.title')}>
        <p>{t('privacy.where.p')}</p>
      </Section>

      <Section id="privacy-contact" title={t('privacy.contact.title')}>
        <p>
          {t('privacy.contact.p')} <a href={`mailto:${CONTACT}`}>{CONTACT}</a>
        </p>
      </Section>
      {/* The date is PrivacyVersion in internal/httpapi/demo.go: change both together. */}
      <p className="muted small">{t('privacy.version')}</p>
    </article>
  )
}

function Section({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  return (
    <section className="panel privacy-section" aria-labelledby={id}>
      <h2 id={id} className="h-section">
        {title}
      </h2>
      {children}
    </section>
  )
}
