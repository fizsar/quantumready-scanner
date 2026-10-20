import { Link } from 'react-router'
import { useShowcaseT, type ShowcaseKey } from '../i18n.showcase'
import { FlowDiagram } from './FlowDiagram'
import './HowItWorks.css'

const POINTS: ShowcaseKey[] = ['how.handshakes', 'how.raw', 'how.ssrf', 'how.scoring', 'how.lab']

/** "How it works" under the home form: the /architecture diagram and the key points. */
export function HowItWorks() {
  const { t } = useShowcaseT()
  return (
    <section className="panel how" aria-labelledby="how-title">
      <h2 id="how-title" className="h-section">
        {t('how.title')}
      </h2>
      <p className="muted">{t('how.lead')}</p>
      <FlowDiagram />
      <ul className="how-list">
        {POINTS.map((k) => (
          <li key={k}>{t(k)}</li>
        ))}
      </ul>
      <p>
        <Link to="/architecture">{t('how.more')}</Link>
      </p>
    </section>
  )
}
