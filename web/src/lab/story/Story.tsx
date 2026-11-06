import { useEffect, useRef, useState, type ReactNode } from 'react'
import type { Lang } from '../../api'
import { useLang } from '../../i18n'
import { usePageMeta } from '../../usePageMeta'
import { chooseMode, type Mode } from '../capability'
import { energy, r95 } from '../hydrogen'
import { SEED } from '../view'
import type { Story as Engine } from './engine'
import { loadStory, storyStillURL } from './load'
import { covers, orbitalOf, progressAt, stageMask, stageView, STEPS, stepAt, termState, type Step, type Term } from './model'
import { TEXT } from './texts'
import './Story.css'

/** /lab/historia: hidden prototype of the redesign's story (noindex, not linked, not in the sitemap). */
export default function Story() {
  const { lang } = useLang()
  const tx = TEXT[lang]
  usePageMeta(`${tx.title} · QuantumReady`, true)
  const [chosen] = useState<Mode>(() => chooseMode())
  const [failed, setFailed] = useState(false)
  const mode: Mode = failed ? { kind: 'still', reason: 'no-webgl2' } : chosen
  const [index, setIndex] = useState(0)
  const [ready, setReady] = useState(false)
  // The sheet (phones): shown while the track is behind it.
  const [sheetOn, setSheetOn] = useState(false)
  const track = useRef<HTMLDivElement>(null)
  const article = useRef<HTMLElement>(null)
  const sheet = useRef<HTMLDivElement>(null)
  const stage = useRef<HTMLDivElement>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const engine = useRef<Engine | null>(null)
  const visible = useRef(false)
  // The last progress and free area, for an engine that arrives after them
  // (three.js loads after the first paint; at the top of the page no scroll follows).
  const latest = useRef<{
    p: number
    free: { left: number; top: number; right: number; bottom: number } | null
  }>({ p: 0, free: null })

  useEffect(() => {
    let raf = 0
    const update = () => {
      raf = 0
      const el = track.current
      if (!el) return
      // The visible height (iOS toolbars change it while scrolling), not 100vh.
      const vh = window.visualViewport?.height ?? window.innerHeight
      const r = el.getBoundingClientRect()
      const p = progressAt(-r.top / vh)
      latest.current.p = p
      engine.current?.setProgress(p)
      setIndex(stepAt(p))
      // The stage: limited to the track on screen, with soft edges, and fading
      // in with it (model.ts stageView, stageMask).
      const st = stage.current
      if (st) {
        const box = st.getBoundingClientRect()
        const v = stageView(r, box, vh)
        const m = stageMask(v, box.bottom - box.top)
        st.style.setProperty('--vis-top', `${m.top}px`)
        st.style.setProperty('--vis-bottom', `${m.bottom}px`)
        st.style.maskImage = m.image
        st.style.setProperty('-webkit-mask-image', m.image)
        st.style.opacity = String(v.opacity)
        st.style.visibility = v.on ? 'visible' : 'hidden'
        if (v.on !== visible.current) {
          visible.current = v.on
          if (v.on) engine.current?.start()
          else engine.current?.stop()
        }
        // The free part of the stage: right of the text panels on wide screens,
        // all of it on phones (the sheet is below). The shape is framed in it.
        const sh = sheet.current
        const phone = !!sh && getComputedStyle(sh).display !== 'none'
        const panel = el.querySelector('.story-panel')?.getBoundingClientRect()
        const left = phone || !panel ? 0 : Math.min(box.width * 0.6, panel.right - box.left + FREE_GAP)
        const free = {
          left,
          top: 0,
          right: box.width - (phone ? 0 : FREE_GAP),
          bottom: box.height,
        }
        latest.current.free = free
        engine.current?.setFrame(free)
        st.style.setProperty('--free-left', `${free.left}px`)
        st.style.setProperty('--free-right', `${box.width - free.right}px`)
      }
      if (sheet.current) setSheetOn(covers(r, sheet.current.getBoundingClientRect()))
    }
    const onScroll = () => {
      if (!raf) raf = requestAnimationFrame(update)
    }
    update()
    window.addEventListener('scroll', onScroll, { passive: true })
    window.addEventListener('resize', onScroll)
    window.visualViewport?.addEventListener('resize', onScroll)
    window.visualViewport?.addEventListener('scroll', onScroll)
    return () => {
      window.removeEventListener('scroll', onScroll)
      window.removeEventListener('resize', onScroll)
      window.visualViewport?.removeEventListener('resize', onScroll)
      window.visualViewport?.removeEventListener('scroll', onScroll)
      cancelAnimationFrame(raf)
    }
  }, [])

  // Phones: the sheet is as tall as its tallest step (every step is stacked in
  // it), never scrolls inside, and the scene takes the rest of the screen above.
  useEffect(() => {
    const el = sheet.current
    if (!el || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(() => article.current?.style.setProperty('--sheet-h', `${el.offsetHeight}px`))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  // 3D only: three.js loads once the page has painted and the main thread is idle.
  useEffect(() => {
    if (chosen.kind !== '3d') return
    let cancelled = false
    const idle = window.requestIdleCallback ?? ((cb: () => void) => window.setTimeout(cb, 200))
    const handle = idle(() => {
      loadStory().then(({ createStory }) => {
        if (cancelled || !canvas.current) return
        engine.current = createStory(canvas.current, {
          points: chosen.points,
          seed: SEED,
          onReady: () => setReady(true),
          onError: () => {
            const e = engine.current
            engine.current = null
            setTimeout(() => e?.dispose()) // not from inside its own render
            setFailed(true)
          },
        })
        engine.current.setProgress(latest.current.p)
        if (latest.current.free) engine.current.setFrame(latest.current.free)
        if (visible.current) engine.current.start()
      })
    })
    return () => {
      cancelled = true
      if (typeof handle === 'number') window.cancelIdleCallback?.(handle)
      engine.current?.dispose()
      engine.current = null
    }
  }, [chosen])

  const step = STEPS[index]
  const sceneStart = (i: number) => i === 0 || STEPS[i - 1].scene !== STEPS[i].scene

  return (
    <article ref={article} className="story" aria-labelledby="story-title" data-mode={mode.kind}>
      <header className="story-intro">
        <p className="prompt">
          <span className="user">guest@quantumready</span>:<span className="path">~/lab</span>$ ./historia
        </p>
        <h1 id="story-title" className="h-title">
          {tx.title}
        </h1>
        <p className="story-lead">{tx.lead}</p>
        {mode.kind === 'still' && <p className="story-note">{tx.still[mode.reason]}</p>}
      </header>

      {/* Never an empty scene: the step's still image until the 3D has drawn. */}
      <div ref={stage} className="story-stage" data-backdrop aria-hidden="true">
        {(mode.kind === 'still' || !ready) && <img className="story-still" src={storyStillURL(step.id)} alt="" />}
        {mode.kind === '3d' && <canvas ref={canvas} className="story-canvas" data-ready={ready} />}
      </div>

      {/* Phones: text and 3D in separate zones, the current step in a panel below the scene. */}
      <div ref={sheet} className="story-sheet" data-active={sheetOn} aria-hidden="true">
        {STEPS.map((s, i) => (
          <div key={s.id} className="story-layer" data-current={i === index}>
            <StepText step={s} first={sceneStart(i)} lang={lang} />
            <Caption step={s} lang={lang} />
          </div>
        ))}
      </div>

      <div className="story-track" ref={track}>
        {STEPS.map((s, i) => (
          <section key={s.id} className="story-step" aria-labelledby={`story-step-${s.id}`}>
            <div className="story-panel">
              <StepText step={s} first={sceneStart(i)} lang={lang} headingId={`story-step-${s.id}`} />
              {/* The label (quantum numbers, the equation) inside the panel: nothing loose over the scene. */}
              <Caption step={s} lang={lang} />
            </div>
          </section>
        ))}
        <div className="story-tail" />
      </div>

      <section className="story-sources" aria-labelledby="story-sources-title">
        <h2 id="story-sources-title">{tx.sources}</h2>
        <p>{tx.method}</p>
        <p>{tx.units}</p>
        <Sources lang={lang} />
      </section>
    </article>
  )
}

function StepText({ step, first, lang, headingId }: { step: Step; first: boolean; lang: Lang; headingId?: string }) {
  const tx = TEXT[lang]
  const t = tx.steps[step.id]
  const o = orbitalOf(step)
  return (
    <>
      {first && <p className="story-kicker">{tx.scenes[step.scene]}</p>}
      <h2 id={headingId} className="story-h">
        {o ? (
          <>
            {o.name}
            {o.sub && <sub>{o.sub}</sub>} ·{' '}
            <span className="story-nowrap">
              (n, l, m) = ({o.n}, {o.l}, {num(o.m)})
            </span>
          </>
        ) : (
          <Title text={t.title} />
        )}
      </h2>
      <p className="story-body">{t.body}</p>
      <p className="story-legend">
        {(step.scene === 1 ? tx.legendAtom : tx.legendWave).map((l, k) => (
          <span key={l} className={k === 0 ? 'story-pos' : 'story-neg'}>
            ● {l}{' '}
          </span>
        ))}
      </p>
    </>
  )
}

/** Space between the text panels and the scene on wide screens (CSS px). */
const FREE_GAP = 24

/** A step title; an expression before ': ' never breaks inside (e.g. "iℏ ∂ψ/∂t: how it changes"). */
function Title({ text }: { text: string }) {
  const i = text.indexOf(': ')
  if (i < 0) return <>{text}</>
  return (
    <>
      <span className="story-nowrap">{text.slice(0, i + 1)}</span> {text.slice(i + 2)}
    </>
  )
}

/** A typographic minus for negative numbers. */
const num = (v: number) => String(v).replace('-', '−')

function Caption({ step, lang }: { step: Step; lang: Lang }) {
  const tx = TEXT[lang]
  const o = orbitalOf(step)
  if (o) {
    const e = new Intl.NumberFormat(lang, {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    })
      .format(energy(o.n))
      .replace('-', '−')
    const r = new Intl.NumberFormat(lang, { maximumFractionDigits: 1 }).format(r95(o))
    return (
      <div className="story-cap">
        <p className="story-qn">
          <math>
            <msub>
              <mi>ψ</mi>
              <mrow>
                <mn>{o.n}</mn>
                <mo>,</mo>
                <mn>{o.l}</mn>
                <mo>,</mo>
                <mn>{num(o.m)}</mn>
              </mrow>
            </msub>
          </math>{' '}
          {tx.energy} E<sub>{o.n}</sub> = {e} eV
        </p>
        <p className="story-scale">{tx.scale(r)}</p>
      </div>
    )
  }
  return (
    <div className="story-cap story-eq">
      <SchrodingerTerms step={step} />
    </div>
  )
}

/** One term of the equation, with its reveal state (Story.css). */
function T({ term, state, children }: { term: Term; state: string; children: ReactNode }) {
  return (
    <mrow data-term={term} data-state={state}>
      {children}
    </mrow>
  )
}

/** iℏ ∂ψ/∂t = −(ℏ²/2m) ∂²ψ/∂x² + V(x)ψ and ψ = (ψ₁ + ψ₂)/√2, revealed and lit term by term. */
function SchrodingerTerms({ step }: { step: Step }) {
  const st = (t: Term) => termState(step, t)
  const d = (top: ReactNode, bottom: ReactNode) => (
    <mfrac>
      <mrow>{top}</mrow>
      <mrow>{bottom}</mrow>
    </mfrac>
  )
  return (
    <>
      <math display="block" aria-label="ψ(x, t)">
        <T term="psi" state={st('psi')}>
          <mi>ψ</mi>
          <mo>(</mo>
          <mi>x</mi>
          <mo>,</mo>
          <mi>t</mi>
          <mo>)</mo>
        </T>
        <T term="sup" state={st('sup')}>
          <mo>=</mo>
          {d(
            <>
              <msub>
                <mi>ψ</mi>
                <mn>1</mn>
              </msub>
              <mo>+</mo>
              <msub>
                <mi>ψ</mi>
                <mn>2</mn>
              </msub>
            </>,
            <msqrt>
              <mn>2</mn>
            </msqrt>,
          )}
        </T>
      </math>
      <math display="block" aria-label="Schrödinger">
        <T term="dt" state={st('dt')}>
          <mi>i</mi>
          <mi>ℏ</mi>
          {d(
            <>
              <mo>∂</mo>
              <mi>ψ</mi>
            </>,
            <>
              <mo>∂</mo>
              <mi>t</mi>
            </>,
          )}
        </T>
        <T term="kin" state={st('kin')}>
          <mo>=</mo>
          <mo>−</mo>
          {d(
            <msup>
              <mi>ℏ</mi>
              <mn>2</mn>
            </msup>,
            <>
              <mn>2</mn>
              <mi>m</mi>
            </>,
          )}
          {d(
            <>
              <msup>
                <mo>∂</mo>
                <mn>2</mn>
              </msup>
              <mi>ψ</mi>
            </>,
            <>
              <mo>∂</mo>
              <msup>
                <mi>x</mi>
                <mn>2</mn>
              </msup>
            </>,
          )}
        </T>
        <T term="pot" state={st('pot')}>
          <mo>+</mo>
          <mi>V</mi>
          <mo>(</mo>
          <mi>x</mi>
          <mo>)</mo>
          <mi>ψ</mi>
        </T>
      </math>
    </>
  )
}

function Sources({ lang }: { lang: Lang }) {
  const es = lang === 'es'
  return (
    <ul>
      <li>
        D. J. Griffiths, D. F. Schroeter, <cite>Introduction to Quantum Mechanics</cite>, {es ? '3.ª ed.' : '3rd ed.'}, Cambridge University Press, 2018: §2.4 ({es ? 'la partícula libre y los paquetes de ondas' : 'the free particle and wave packets'}) {es ? 'y' : 'and'} §4.2 ({es ? 'el átomo de hidrógeno' : 'the hydrogen atom'}).
      </li>
      <li>
        C. Cohen-Tannoudji, B. Diu, F. Laloë, <cite>Quantum Mechanics</cite>, vol. 1, {es ? 'complemento' : 'complement'} G<sub>I</sub> ({es ? 'el paquete de ondas gaussiano' : 'the Gaussian wave packet'}).
      </li>
      <li>
        NIST <cite>Digital Library of Mathematical Functions</cite>:{' '}
        <a href="https://dlmf.nist.gov/18.9" rel="noopener">
          §18.9
        </a>
        ,{' '}
        <a href="https://dlmf.nist.gov/14.10" rel="noopener">
          §14.10
        </a>
        ,{' '}
        <a href="https://dlmf.nist.gov/14.30" rel="noopener">
          §14.30
        </a>{' '}
        ({es ? 'Laguerre, Legendre y armónicos esféricos' : 'Laguerre, Legendre and spherical harmonics'}
        ).
      </li>
      <li>
        CODATA 2022 (NIST), R∞hc:{' '}
        <a href="https://physics.nist.gov/cgi-bin/cuu/Value?rydhcev" rel="noopener">
          physics.nist.gov
        </a>
        .
      </li>
    </ul>
  )
}
