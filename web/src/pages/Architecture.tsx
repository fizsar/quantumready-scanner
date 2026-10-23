import { Link } from 'react-router'
import { FlowDiagram } from '../components/FlowDiagram'
import { uthAnchor } from '../exampleAnchors'
import { useShowcaseT, type ShowcaseKey } from '../i18n.showcase'
import { usePageMeta } from '../usePageMeta'
import './Architecture.css'

const CONTACT = 'soporte@fizsar.com'

/** /architecture: how the service works, at a high level (no versions, limits or settings). */
export function Architecture() {
  const { t } = useShowcaseT()
  usePageMeta(t('arch.meta.title'), false, t('arch.meta.description'))

  const list = (keys: ShowcaseKey[]) => (
    <ul className="arch-list">
      {keys.map((k) => (
        <li key={k}>{t(k)}</li>
      ))}
    </ul>
  )

  return (
    <article className="arch" aria-labelledby="arch-title">
      <p className="prompt">
        <span className="user">guest@quantumready</span>:<span className="path">~</span>$ cat ARCHITECTURE.md
      </p>
      <h1 id="arch-title" className="h-title">
        {t('arch.title')}
      </h1>
      <p className="muted">{t('arch.lead')}</p>

      <Section id="arch-flow" title={t('arch.flow.title')}>
        <FlowDiagram />
        <ol className="arch-steps">
          {(['arch.flow.step1', 'arch.flow.step2', 'arch.flow.step3', 'arch.flow.step4'] as const).map((k) => (
            <li key={k}>{t(k)}</li>
          ))}
        </ol>
      </Section>

      <Section id="arch-engine" title={t('arch.engine.title')}>
        <p>{t('arch.engine.p1')}</p>
        <p>{t('arch.engine.p2')}</p>
        <p>{t('arch.engine.p3')}</p>
        <p>{t('arch.engine.p4')}</p>
      </Section>

      <Section id="arch-scoring" title={t('arch.scoring.title')}>
        <p>{t('arch.scoring.p1')}</p>
        <p>{t('arch.scoring.p2')}</p>
        <p>{t('arch.scoring.p3')}</p>
        <p>{t('arch.scoring.p4')}</p>
      </Section>

      <Section id="arch-security" title={t('arch.security.title')}>
        {list(['arch.security.ssrf', 'arch.security.containers', 'arch.security.supply'])}
      </Section>

      <Section id="arch-quality" title={t('arch.quality.title')}>
        {list(['arch.quality.lab', 'arch.quality.testssl', 'arch.quality.fuzz'])}
      </Section>

      <Section id="arch-ops" title={t('arch.ops.title')}>
        {list(['arch.ops.backup', 'arch.ops.restore', 'arch.ops.alerts', 'arch.ops.deploy'])}
      </Section>

      <Section id="arch-dev" className="arch-dev" title={t('arch.dev.title')}>
        <p className="muted">{t('arch.dev.lead')}</p>
        {list(['arch.dev.accounts', 'arch.dev.domains', 'arch.dev.public', 'arch.dev.idor'])}
      </Section>

      <Section id="arch-see" title={t('arch.see.title')}>
        <ul className="arch-list">
          <li>
            <Link to="/examples">{t('arch.see.examples')}</Link>
            {t('arch.see.examplesText')}
          </li>
          <li>
            <Link to={`/examples#${uthAnchor('fizsar')}`}>{t('arch.see.report')}</Link>
            {t('arch.see.reportText')}
          </li>
        </ul>
      </Section>

      <Section id="arch-contact" title={t('arch.contact.title')}>
        <p>
          {t('arch.contact.text')} <a href={`mailto:${CONTACT}`}>{CONTACT}</a>
        </p>
      </Section>
    </article>
  )
}

function Section({ id, title, className = '', children }: { id: string; title: React.ReactNode; className?: string; children: React.ReactNode }) {
  return (
    <section className={`panel arch-section ${className}`} aria-labelledby={id}>
      <h2 id={id} className="h-section">
        {title}
      </h2>
      {children}
    </section>
  )
}
