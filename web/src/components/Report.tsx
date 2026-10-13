import { Link } from 'react-router'
import type { Finding, Report as ReportData, ScanView } from '../api'
import { useLang } from '../i18n'
import { headline } from '../verdict'

export function Report({ scan, report }: { scan: ScanView; report: ReportData }) {
  const { lang, t } = useLang()
  const verdict = headline(report)
  const when = scan.finished_at ?? scan.created_at
  const date = new Intl.DateTimeFormat(lang === 'es' ? 'es-ES' : 'en-GB', { dateStyle: 'long', timeStyle: 'short' }).format(new Date(when))

  return (
    <article className="report" aria-labelledby="verdict">
      <p className="prompt">
        <span className="user">guest@quantumready</span>:<span className="path">~</span>$ ./scan {scan.host}
      </p>
      <h1 id="verdict" className={`verdict tone-${verdict.tone}`}>
        {t(verdict.key)}
      </h1>

      <section className="summary" aria-label={t('grade.label')}>
        <div className={`grade grade-${report.grade}`}>
          <span className="grade-letter" aria-hidden="true">
            {report.grade}
          </span>
          <span className="sr-only">
            {t('grade.label')}: {report.grade}
          </span>
          <span className="grade-score">{t('grade.score', { score: report.score })}</span>
        </div>
        <div className="summary-facts">
          <p className="host">{scan.host}</p>
          <p className={`badge ${report.pqc ? 'badge-yes' : 'badge-no'}`}>{report.pqc ? t('pqc.yes') : t('pqc.no')}</p>
          <p className="muted">{report.selected_group ? t('pqc.group', { group: report.selected_group }) : t('pqc.noGroup')}</p>
          {report.tls_version && <p className="muted">{report.tls_version}</p>}
          {report.cap && (
            <p className="cap">{t('grade.capped', { grade: report.cap.grade, reasons: report.cap.findings.map((f) => f.label).join(', ') })}</p>
          )}
        </div>
      </section>

      <div className="axes">
        <Axis axis="quantum" level={report.quantum_risk.label} findings={report.findings} />
        <Axis axis="current" level={report.current_risk.label} findings={report.findings} />
      </div>

      <section className="panel" aria-labelledby="breakdown">
        <h2 id="breakdown" className="h-section">
          {t('breakdown.title')}
        </h2>
        <ul className="blocks">
          {report.blocks.map((b) => (
            <li key={b.id} className="block">
              <div className="block-head">
                <span>{b.label}</span>
                <span className="pts">{t('breakdown.points', { points: b.points, max: b.max })}</span>
              </div>
              <meter className="bar" min={0} max={b.max} value={b.points} aria-label={b.label} />
              {b.items && (
                <ul className="items">
                  {b.items.map((it) => (
                    <li key={it.id}>
                      <span>{it.label}</span>
                      <span className="pts">{t('breakdown.points', { points: it.points, max: it.max })}</span>
                    </li>
                  ))}
                </ul>
              )}
            </li>
          ))}
          {!!report.floor_points && (
            <li className="block block-floor">
              <div className="block-head">
                <span>{t('breakdown.floor')}</span>
                <span className="pts">+{report.floor_points}</span>
              </div>
              <p className="muted small">{t('breakdown.floorHelp')}</p>
            </li>
          )}
        </ul>
      </section>

      <section className="panel" aria-labelledby="noteval">
        <h2 id="noteval" className="h-section">
          {t('noteval.title')}
        </h2>
        <p className="muted">{t('noteval.lead')}</p>
        <ul className="noteval">
          {report.not_evaluated.map((n) => (
            <li key={n.code}>{n.label}</li>
          ))}
        </ul>
      </section>

      <footer className="report-footer">
        <span>{t('footer.scanned', { date })}</span>
        <span>{t('footer.model', { version: report.scoring_version })}</span>
        <Link to="/">{t('footer.newScan')}</Link>
      </footer>
    </article>
  )
}

function Axis({ axis, level, findings }: { axis: 'quantum' | 'current'; level: string; findings: Finding[] }) {
  const { t } = useLang()
  const mine = findings.filter((f) => f.axis === axis) // already sorted by severity
  // Risks (and informational notes) first; what is done right at the end, apart.
  const risks = mine.filter((f) => f.severity !== 'pass')
  const good = mine.filter((f) => f.severity === 'pass')
  const id = `axis-${axis}`
  return (
    <section className="panel axis" aria-labelledby={id}>
      <h2 id={id} className="h-section">
        {t(`axis.${axis}.title`)} <span className="level">{t('axis.level', { level })}</span>
      </h2>
      <p className="muted small">{t(`axis.${axis}.help`)}</p>
      {risks.length > 0 ? <FindingList findings={risks} /> : <p className="no-risks">{t('axis.noRisks')}</p>}
      {good.length > 0 && (
        <div className="good" role="group" aria-labelledby={`${id}-good`}>
          <h3 id={`${id}-good`} className="h-good">
            {t('axis.good')}
          </h3>
          <FindingList findings={good} />
        </div>
      )}
    </section>
  )
}

function FindingList({ findings }: { findings: Finding[] }) {
  const { t } = useLang()
  return (
    <ul className="findings">
      {findings.map((f) => (
        <li key={f.code} className={`finding sev-${f.severity}`}>
          <details>
            <summary>
              <span className={`sev sev-tag-${f.severity}`}>{f.severity_label}</span>
              <span className="finding-title">{f.title}</span>
            </summary>
            <p>{f.detail}</p>
            {f.remediation && (
              <p className="fix">
                <strong>{t('finding.fix')}:</strong> {f.remediation}
              </p>
            )}
          </details>
        </li>
      ))}
    </ul>
  )
}
