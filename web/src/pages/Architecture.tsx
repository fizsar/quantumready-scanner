import { useId } from 'react'
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

      <Section
        id="arch-dev"
        className="arch-dev"
        title={
          <>
            {t('arch.dev.title')} <span className="arch-badge">{t('arch.dev.badge')}</span>
          </>
        }
      >
        <p className="muted">{t('arch.dev.lead')}</p>
        {list(['arch.dev.accounts', 'arch.dev.domains', 'arch.dev.public', 'arch.dev.idor'])}
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

// Boxes of the diagram: column, row, label and sublabel keys.
const W = 128
const H = 52
const X = [8, 148, 288, 428, 568, 708]
const Y = [24, 112, 200]
type Box = { col: number; row: number; label: ShowcaseKey; sub?: ShowcaseKey; kind?: string }
const BOXES: Record<string, Box> = {
  browser: { col: 0, row: 1, label: 'arch.flow.browser', kind: 'ext' },
  caddy: { col: 1, row: 1, label: 'arch.flow.caddy', sub: 'arch.flow.caddySub' },
  web: { col: 2, row: 0, label: 'arch.flow.web', sub: 'arch.flow.webSub' },
  api: { col: 2, row: 2, label: 'arch.flow.api', sub: 'arch.flow.apiSub' },
  db: { col: 3, row: 2, label: 'arch.flow.db', sub: 'arch.flow.dbSub', kind: 'db' },
  worker: { col: 4, row: 2, label: 'arch.flow.worker', sub: 'arch.flow.workerSub' },
  scanner: { col: 4, row: 1, label: 'arch.flow.scanner', sub: 'arch.flow.scannerSub' },
  target: { col: 5, row: 1, label: 'arch.flow.target', sub: 'arch.flow.targetSub', kind: 'ext' },
}
// Arrows: from box, side; to box, side.
type Side = 'l' | 'r' | 't' | 'b'
const ARROWS: [string, Side, string, Side][] = [
  ['browser', 'r', 'caddy', 'l'],
  ['caddy', 't', 'web', 'l'],
  ['caddy', 'b', 'api', 'l'],
  ['api', 'r', 'db', 'l'],
  ['worker', 'l', 'db', 'r'],
  ['worker', 't', 'scanner', 'b'],
  ['scanner', 'r', 'target', 'l'],
]

function anchor(b: Box, side: Side): [number, number] {
  const x = X[b.col]
  const y = Y[b.row]
  switch (side) {
    case 'l':
      return [x, y + H / 2]
    case 'r':
      return [x + W, y + H / 2]
    case 't':
      return [x + W / 2, y]
    case 'b':
      return [x + W / 2, y + H]
  }
}

/** Inline SVG, classes only (the CSP forbids inline styles); colours in Architecture.css. */
function FlowDiagram() {
  const { t } = useShowcaseT()
  const id = useId()
  return (
    <figure className="arch-figure">
      <div className="arch-scroll">
        <svg className="arch-diagram" viewBox="0 0 844 272" role="img" aria-labelledby={`${id}-t`} aria-describedby={`${id}-d`}>
          <title id={`${id}-t`}>{t('arch.flow.svgTitle')}</title>
          <desc id={`${id}-d`}>{t('arch.flow.svgDesc')}</desc>
          <defs>
            <marker id={`${id}-arrow`} viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
              <path className="arch-arrowhead" d="M0 0 L10 5 L0 10 z" />
            </marker>
          </defs>
          <rect className="arch-bg" x="0" y="0" width="844" height="272" rx="12" />
          {ARROWS.map(([from, fs, to, ts]) => {
            const [x1, y1] = anchor(BOXES[from], fs)
            const [x2, y2] = anchor(BOXES[to], ts)
            // Elbow when the boxes are on different rows and the arrow leaves from the top or bottom.
            const d = fs === 't' || fs === 'b' ? (x1 === x2 ? `M${x1} ${y1} L${x2} ${y2}` : `M${x1} ${y1} L${x1} ${y2} L${x2} ${y2}`) : `M${x1} ${y1} L${x2} ${y2}`
            return <path key={from + to} className="arch-edge" d={d} markerEnd={`url(#${id}-arrow)`} />
          })}
          {Object.entries(BOXES).map(([key, b]) => (
            <g key={key} className={`arch-node ${b.kind ? `arch-node-${b.kind}` : ''}`}>
              <rect x={X[b.col]} y={Y[b.row]} width={W} height={H} rx="8" />
              <text className="arch-label" x={X[b.col] + W / 2} y={Y[b.row] + (b.sub ? 22 : 31)} textAnchor="middle">
                {t(b.label)}
              </text>
              {b.sub && (
                <text className="arch-sub" x={X[b.col] + W / 2} y={Y[b.row] + 40} textAnchor="middle">
                  {t(b.sub)}
                </text>
              )}
            </g>
          ))}
        </svg>
      </div>
    </figure>
  )
}
