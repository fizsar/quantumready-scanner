import { useId } from 'react'
import { Link } from 'react-router'
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

// The diagram's boxes: label and sublabel keys, and the kind that colours them.
const H = 52
type Key = 'browser' | 'caddy' | 'web' | 'api' | 'db' | 'worker' | 'scanner' | 'target'
const BOXES: Record<Key, { label: ShowcaseKey; sub?: ShowcaseKey; kind?: string }> = {
  browser: { label: 'arch.flow.browser', kind: 'ext' },
  caddy: { label: 'arch.flow.caddy', sub: 'arch.flow.caddySub' },
  web: { label: 'arch.flow.web', sub: 'arch.flow.webSub' },
  api: { label: 'arch.flow.api', sub: 'arch.flow.apiSub' },
  db: { label: 'arch.flow.db', sub: 'arch.flow.dbSub', kind: 'db' },
  worker: { label: 'arch.flow.worker', sub: 'arch.flow.workerSub' },
  scanner: { label: 'arch.flow.scanner', sub: 'arch.flow.scannerSub' },
  target: { label: 'arch.flow.target', sub: 'arch.flow.targetSub', kind: 'ext' },
}
// The same arrows in both layouts: from, to.
const ARROWS: [Key, Key][] = [
  ['browser', 'caddy'],
  ['caddy', 'web'],
  ['caddy', 'api'],
  ['api', 'db'],
  ['worker', 'db'],
  ['worker', 'scanner'],
  ['scanner', 'target'],
]

type Point = [number, number]
type Side = 'l' | 'r' | 't' | 'b'
/**
 * A layout places every box (top-left corner) and says which sides each
 * arrow joins; an arrow leaving from the top or bottom bends once if needed.
 */
interface Layout {
  name: 'wide' | 'tall'
  width: number
  height: number
  box: number // box width
  at: Record<Key, Point>
  sides: Record<string, [Side, Side]> // "from>to"
}

// Wide screens: left to right, as before.
const X = [8, 148, 288, 428, 568, 708]
const Y = [24, 112, 200]
const WIDE: Layout = {
  name: 'wide',
  width: 844,
  height: 272,
  box: 128,
  at: {
    browser: [X[0], Y[1]], caddy: [X[1], Y[1]], web: [X[2], Y[0]], api: [X[2], Y[2]],
    db: [X[3], Y[2]], worker: [X[4], Y[2]], scanner: [X[4], Y[1]], target: [X[5], Y[1]],
  },
  sides: {
    'browser>caddy': ['r', 'l'], 'caddy>web': ['t', 'l'], 'caddy>api': ['b', 'l'], 'api>db': ['r', 'l'],
    'worker>db': ['l', 'r'], 'worker>scanner': ['t', 'b'], 'scanner>target': ['r', 'l'],
  },
}

// Narrow screens: top to bottom in two columns, the same boxes and arrows.
const L = 12
const R = 178
const TALL: Layout = {
  name: 'tall',
  width: 340,
  height: 512,
  box: 150,
  at: {
    browser: [95, 16], caddy: [95, 100], web: [L, 184], api: [R, 184],
    db: [R, 268], worker: [L, 268], scanner: [L, 352], target: [L, 436],
  },
  sides: {
    'browser>caddy': ['b', 't'], 'caddy>web': ['b', 't'], 'caddy>api': ['b', 't'], 'api>db': ['b', 't'],
    'worker>db': ['r', 'l'], 'worker>scanner': ['b', 't'], 'scanner>target': ['b', 't'],
  },
}

function anchor([x, y]: Point, W: number, side: Side): Point {
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

// JetBrains Mono advances 0.6 em per character. A text that would not fit its
// box (some labels, in some languages) is squeezed to fit with textLength, an
// SVG attribute, not a style (CSP).
function fit(text: string, fontSize: number, room: number) {
  return text.length * fontSize * 0.6 > room ? { textLength: room, lengthAdjust: 'spacingAndGlyphs' as const } : {}
}

function edgePath(l: Layout, from: Key, to: Key): string {
  const [fs, ts] = l.sides[`${from}>${to}`]
  const [x1, y1] = anchor(l.at[from], l.box, fs)
  const [x2, y2] = anchor(l.at[to], l.box, ts)
  if (x1 === x2 || y1 === y2) return `M${x1} ${y1} L${x2} ${y2}`
  if (ts === 't') {
    const mid = (y1 + y2) / 2 // down, across, down
    return `M${x1} ${y1} L${x1} ${mid} L${x2} ${mid} L${x2} ${y2}`
  }
  return `M${x1} ${y1} L${x1} ${y2} L${x2} ${y2}` // up or down, then across
}

/**
 * The flow of a scan as inline SVG, classes only (the CSP forbids inline
 * styles; colours in Architecture.css). Two layouts of the same diagram: CSS
 * shows the wide one on wide screens and the tall one on narrow ones, so it
 * never scrolls sideways; the hidden one is display: none, also for screen
 * readers.
 */
function FlowDiagram() {
  return (
    <figure className="arch-figure">
      <Diagram layout={WIDE} />
      <Diagram layout={TALL} />
    </figure>
  )
}

function Diagram({ layout: l }: { layout: Layout }) {
  const { t } = useShowcaseT()
  const id = useId()
  return (
    <svg
      className={`arch-diagram arch-diagram-${l.name}`}
      viewBox={`0 0 ${l.width} ${l.height}`}
      role="img"
      aria-labelledby={`${id}-t`}
      aria-describedby={`${id}-d`}
    >
      <title id={`${id}-t`}>{t('arch.flow.svgTitle')}</title>
      <desc id={`${id}-d`}>{t('arch.flow.svgDesc')}</desc>
      <defs>
        <marker id={`${id}-arrow`} viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
          <path className="arch-arrowhead" d="M0 0 L10 5 L0 10 z" />
        </marker>
      </defs>
      <rect className="arch-bg" x="0" y="0" width={l.width} height={l.height} rx="12" />
      {ARROWS.map(([from, to]) => (
        <path key={from + to} className="arch-edge" d={edgePath(l, from, to)} markerEnd={`url(#${id}-arrow)`} />
      ))}
      {(Object.keys(BOXES) as Key[]).map((key) => {
        const b = BOXES[key]
        const [x, y] = l.at[key]
        const W = l.box
        return (
          <g key={key} className={`arch-node ${b.kind ? `arch-node-${b.kind}` : ''}`}>
            <rect x={x} y={y} width={W} height={H} rx="8" />
            <text className="arch-label" x={x + W / 2} y={y + (b.sub ? 22 : 31)} textAnchor="middle" {...fit(t(b.label), 14, W - 12)}>
              {t(b.label)}
            </text>
            {b.sub && (
              <text className="arch-sub" x={x + W / 2} y={y + 40} textAnchor="middle" {...fit(t(b.sub), 11, W - 12)}>
                {t(b.sub)}
              </text>
            )}
          </g>
        )
      })}
    </svg>
  )
}
