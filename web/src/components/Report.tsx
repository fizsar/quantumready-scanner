import { useId, useState } from 'react'
import { Link, useNavigate } from 'react-router'
import { createScan, type Block, type Finding, type Report as ReportData, type ScanView } from '../api'
import { useLang } from '../i18n'
import { headline } from '../verdict'
import { UnderTheHood } from './UnderTheHood'

type Level = 1 | 2

/**
 * A finished scan's report. level 2 nests it under a page heading (several
 * reports on /examples); ids are unique per report. uthAnchor is the id of
 * its "Under the hood" panel, which a link to #<uthAnchor> opens.
 */
export function Report({ scan, report, level = 1, uthAnchor }: { scan: ScanView; report: ReportData; level?: Level; uthAnchor?: string }) {
  const { lang, t } = useLang()
  const uid = useId()
  const verdict = headline(report)
  const when = scan.finished_at ?? scan.created_at
  const date = new Intl.DateTimeFormat(lang === 'es' ? 'es-ES' : 'en-GB', { dateStyle: 'long', timeStyle: 'short' }).format(new Date(when))
  const H = level === 1 ? 'h1' : 'h2'
  const H2 = level === 1 ? 'h2' : 'h3'

  return (
    <article className="report" aria-labelledby={`${uid}-verdict`}>
      <p className="prompt">
        <span className="user">guest@quantumready</span>:<span className="path">~</span>$ ./scan {scan.host}
      </p>
      <H id={`${uid}-verdict`} className={`verdict tone-${verdict.tone}`}>
        {t(verdict.key)}
      </H>

      {report.outdated_model && <OutdatedModel scan={scan} version={report.scoring_version} />}

      <section className="summary" aria-label={t('grade.label')}>
        {report.readiness ? (
          // Model 2026.3: two axes, two cards of the same weight.
          <div className="axis-cards">
            <div className={`axis-card grade-${report.grade}`} role="group" aria-labelledby={`${uid}-sec`}>
              <p id={`${uid}-sec`} className="axis-card-title">
                {t('axis2.security')}
              </p>
              <p className="axis-card-value grade-letter" aria-hidden="true">
                {report.grade}
              </p>
              <p className="sr-only">
                {t('axis2.security')}: {report.grade}
              </p>
              <p className="grade-score">{t('grade.score', { score: report.score })}</p>
              <meter className="bar" min={0} max={100} value={report.score} aria-label={t('axis2.security')} />
              {report.cap && <p className="muted small">{t('grade.cappedShort', { grade: report.cap.grade })}</p>}
            </div>
            <div className={`axis-card readiness-${report.readiness.level.code}`} role="group" aria-labelledby={`${uid}-ready`}>
              <p id={`${uid}-ready`} className="axis-card-title">
                {t('axis2.readiness')}
              </p>
              <p className="axis-card-value readiness-level">{report.readiness.level.label}</p>
              <p className="grade-score">{t('grade.score', { score: report.readiness.score })}</p>
              <meter className="bar" min={0} max={100} value={report.readiness.score} aria-label={t('axis2.readiness')} />
              <p className="muted small">{t('axis2.cap85')}</p>
            </div>
          </div>
        ) : (
          <div className={`grade grade-${report.grade}`}>
            <span className="grade-letter" aria-hidden="true">
              {report.grade}
            </span>
            <span className="sr-only">
              {t('grade.label')}: {report.grade}
            </span>
            <span className="grade-score">{t('grade.score', { score: report.score })}</span>
          </div>
        )}
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
        <Axis uid={uid} h={level} axis="quantum" level={report.quantum_risk.label} findings={report.findings} />
        <Axis uid={uid} h={level} axis="current" level={report.current_risk.label} findings={report.findings} />
      </div>

      <section className="panel" aria-labelledby={`${uid}-breakdown`}>
        <H2 id={`${uid}-breakdown`} className="h-section">
          {t('breakdown.title')}
        </H2>
        {report.readiness && <h3 className="h-axis2">{t('axis2.security')}</h3>}
        <ul className="blocks">
          {report.blocks.map((b) => (
            <BlockRow key={b.id} b={b} />
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
        {report.readiness && (
          <>
            <h3 className="h-axis2">{t('axis2.readiness')}</h3>
            <ul className="blocks">
              {report.readiness.blocks.map((b) => (
                <BlockRow key={b.id} b={b} />
              ))}
            </ul>
            <p className="muted small">{t('axis2.readinessHelp')}</p>
          </>
        )}
      </section>

      <section className="panel" aria-labelledby={`${uid}-noteval`}>
        <H2 id={`${uid}-noteval`} className="h-section">
          {t('noteval.title')}
        </H2>
        <p className="muted">{t('noteval.lead')}</p>
        <ul className="noteval">
          {report.not_evaluated.map((n) => (
            <li key={n.code}>{n.label}</li>
          ))}
        </ul>
      </section>

      <UnderTheHood scan={scan} anchor={uthAnchor} level={level} />

      <footer className="report-footer">
        <span>{t('footer.scanned', { date })}</span>
        <span>{t('footer.model', { version: report.scoring_version })}</span>
        <Link to="/">{t('footer.newScan')}</Link>
      </footer>
    </article>
  )
}

function BlockRow({ b }: { b: Block }) {
  const { t } = useLang()
  const penalty = b.points < 0
  return (
    <li className={`block${penalty ? ' block-penalty' : ''}`}>
      <div className="block-head">
        <span>{b.label}</span>
        <span className="pts">{penalty ? t('breakdown.penalty', { points: b.points }) : t('breakdown.points', { points: b.points, max: b.max })}</span>
      </div>
      {!penalty && <meter className="bar" min={0} max={b.max} value={b.points} aria-label={b.label} />}
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
  )
}

/** A scan judged with an earlier model: say so, and offer to scan again. */
function OutdatedModel({ scan, version }: { scan: ScanView; version: string }) {
  const { lang, t } = useLang()
  const navigate = useNavigate()
  const [busy, setBusy] = useState(false)
  const [failed, setFailed] = useState(false)
  const rescan = async () => {
    setBusy(true)
    setFailed(false)
    try {
      const s = await createScan(scan.host, lang)
      navigate(`/scans/${s.id}`)
    } catch {
      setFailed(true)
    } finally {
      setBusy(false)
    }
  }
  return (
    <section className="panel outdated" aria-label={t('outdated.title', { version })}>
      <p>
        <strong>{t('outdated.title', { version })}</strong> {t('outdated.lead')}
      </p>
      <button type="button" className="btn" onClick={rescan} disabled={busy}>
        {t('outdated.rescan')}
      </button>
      {failed && <p className="bad small">{t('outdated.failed')}</p>}
    </section>
  )
}

function Axis({ uid, h, axis, level, findings }: { uid: string; h: Level; axis: 'quantum' | 'current'; level: string; findings: Finding[] }) {
  const { t } = useLang()
  const mine = findings.filter((f) => f.axis === axis) // already sorted by severity
  // Risks (and informational notes) first; what is done right at the end, apart.
  const risks = mine.filter((f) => f.severity !== 'pass')
  const good = mine.filter((f) => f.severity === 'pass')
  const id = `${uid}-axis-${axis}`
  const H2 = h === 1 ? 'h2' : 'h3'
  const H3 = h === 1 ? 'h3' : 'h4'
  return (
    <section className="panel axis" aria-labelledby={id}>
      <H2 id={id} className="h-section">
        {t(`axis.${axis}.title`)} <span className="level">{t('axis.level', { level })}</span>
      </H2>
      <p className="muted small">{t(`axis.${axis}.help`)}</p>
      {axis === 'quantum' && risks.some((f) => f.threat === 'grover') ? (
        <>
          <H3 className="h-threat">{t('threat.shor')}</H3>
          {risks.some((f) => f.threat !== 'grover') ? (
            <FindingList findings={risks.filter((f) => f.threat !== 'grover')} />
          ) : (
            <p className="no-risks">{t('axis.noRisks')}</p>
          )}
          <H3 className="h-threat">{t('threat.grover')}</H3>
          <FindingList findings={risks.filter((f) => f.threat === 'grover')} />
        </>
      ) : risks.length > 0 ? (
        <FindingList findings={risks} />
      ) : (
        <p className="no-risks">{t('axis.noRisks')}</p>
      )}
      {good.length > 0 && (
        <div className="good" role="group" aria-labelledby={`${id}-good`}>
          <H3 id={`${id}-good`} className="h-good">
            {t('axis.good')}
          </H3>
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
