import { DemoCta } from '../components/DemoButton'
import { useEffect } from 'react'
import { useLocation } from 'react-router'
import { Report } from '../components/Report'
import { uthAnchor } from '../exampleAnchors'
import { EXAMPLES } from '../examples'
import { useShowcaseT, type ShowcaseKey } from '../i18n.showcase'
import { usePageMeta } from '../usePageMeta'
import './Examples.css'

/** /examples: real reports with the same component as a scan, each explained. */
export default function Examples() {
  const { lang, t } = useShowcaseT()
  const { hash } = useLocation()
  usePageMeta(t('examples.meta.title'), false, t('examples.meta.description'))
  // The visitor's time zone, as the report footer below uses: both show the same day.
  const date = new Intl.DateTimeFormat(lang === 'es' ? 'es-ES' : 'en-GB', { dateStyle: 'long' })

  // The page renders at once (no fetch), but after the browser looked for the
  // #fragment: scroll to it ourselves.
  useEffect(() => {
    if (hash) document.getElementById(decodeURIComponent(hash.slice(1)))?.scrollIntoView?.()
  }, [hash])

  return (
    <div className="examples">
      <section className="panel" aria-labelledby="examples-title">
        <p className="prompt">
          <span className="user">guest@quantumready</span>:<span className="path">~</span>$ ls examples/
        </p>
        <h1 id="examples-title" className="h-title">
          {t('examples.title')}
        </h1>
        <p>{t('examples.lead')}</p>
        <p className="muted small">{t('examples.labNote')}</p>
        <DemoCta from="examples" />
        <nav aria-label={t('examples.nav')}>
          <ul className="examples-toc">
            {EXAMPLES.map(({ key, scans }) => {
              const s = scans[lang]
              return (
                <li key={key}>
                  <a href={`#${key}`}>{t('examples.heading', { host: s.host, grade: s.report!.grade, score: s.report!.score })}</a>
                </li>
              )
            })}
          </ul>
        </nav>
      </section>

      {EXAMPLES.map(({ key, lab, scans }) => {
        const scan = scans[lang]
        const report = scan.report!
        const when = date.format(new Date(scan.finished_at ?? scan.created_at))
        return (
          <section key={key} id={key} className="example" aria-labelledby={`${key}-title`}>
            <div className="panel example-intro">
              <h2 id={`${key}-title`} className="h-section">
                {t('examples.heading', { host: scan.host, grade: report.grade, score: report.score })}
              </h2>
              {lab ? <p className="example-lab">{t('examples.lab')}</p> : null}
              <p className="muted small">{lab ? t('examples.labDate', { date: when }) : t('examples.real', { date: when })}</p>
              <dl className="example-notes">
                <dt>{t('examples.config')}</dt>
                <dd>{t(`examples.${key}.config` as ShowcaseKey)}</dd>
                <dt>{t('examples.why')}</dt>
                <dd>{t(`examples.${key}.why` as ShowcaseKey)}</dd>
                <dt>{t('examples.fix')}</dt>
                <dd>{t(`examples.${key}.fix` as ShowcaseKey)}</dd>
              </dl>
              <a href={`#${uthAnchor(key)}`}>{t('examples.uthLink')}</a>
            </div>
            <Report scan={scan} report={report} level={2} uthAnchor={uthAnchor(key)} />
          </section>
        )
      })}
    </div>
  )
}
