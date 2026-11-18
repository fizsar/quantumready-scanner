import { useEffect, useRef, useState } from 'react'
import { chooseMode, type Mode } from '../lab/capability'
import type { Hero } from '../lab/hero/hero'
import { loadHero, storyStillURL } from '../lab/story/load'
import { useLang } from '../i18n'

/** Share of the card that must be on screen for the entry to start. */
export const ENTRY_VISIBLE = 0.4

/**
 * The scene in the home's hero: a framed card with the still image of the 1s
 * orbital from the first frame (plain HTML and CSS) and, where the device
 * allows it, the 3D cloud drawn exactly over it once three.js has loaded
 * (after the first paint, and only when the card is near the screen). The
 * cloud condenses into the orbital and then into the brand's Q̂ (lab/hero).
 *
 * Decorative: hidden from assistive technology, never focusable, and it takes
 * no pointer events, so it cannot get in the way of the form next to it. With
 * reduced motion, without fast WebGL2 or with data saving, only the image.
 * Nothing is drawn while the card is off screen or the tab is hidden.
 */
export function HeroScene() {
  const { t } = useLang()
  // Decided after the first paint (probing WebGL2 takes time on a slow phone): until then, the image.
  const [chosen, setChosen] = useState<Mode | null>(null)
  const [failed, setFailed] = useState(false)
  const [ready, setReady] = useState(false)
  // The canvas has finished fading in: the image under it can go.
  const [covered, setCovered] = useState(false)
  // The label names the orbital: it leaves when the cloud stops being one.
  const [labelled, setLabelled] = useState(true)
  const card = useRef<HTMLDivElement>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const threeD = chosen?.kind === '3d' && !failed

  // Near the screen and the main thread idle: decide whether this device gets the 3D.
  useEffect(() => {
    const el = card.current
    if (!el || typeof IntersectionObserver === 'undefined') return
    let cancelled = false
    const idle = window.requestIdleCallback ?? ((cb: () => void) => window.setTimeout(cb, 200))
    const near = new IntersectionObserver(
      (es) => {
        if (!es.some((e) => e.isIntersecting)) return
        near.disconnect()
        // Two frames on, the page has painted; then whenever the main thread is free.
        requestAnimationFrame(() => requestAnimationFrame(() => idle(() => !cancelled && setChosen(chooseMode()))))
      },
      { rootMargin: '25%' },
    )
    near.observe(el)
    return () => {
      cancelled = true
      near.disconnect()
    }
  }, [])

  useEffect(() => {
    const el = card.current
    if (chosen?.kind !== '3d' || !el) return
    let hero: Hero | null = null
    let cancelled = false
    let on = false
    let enough = false
    // Fetch the 3D code. On screen: draw. Enough on screen: play the entry.
    void loadHero().then(({ createHero }) => {
      if (cancelled || !canvas.current) return
      hero = createHero(canvas.current, {
        points: chosen.points,
        restStill: window.matchMedia?.('(pointer: coarse)').matches ?? false,
        onReady: () => setReady(true),
        onError: () => {
          const h = hero
          hero = null
          setTimeout(() => h?.dispose()) // not from inside its own render
          setFailed(true)
        },
        onPhase: (p) => {
          if (p === 'collapse' || p === 'rest') setLabelled(false)
        },
      })
      hero.setVisible(on, enough)
    })
    const seen = new IntersectionObserver(
      (es) => {
        const e = es[es.length - 1]
        on = e.isIntersecting
        enough = e.intersectionRatio >= ENTRY_VISIBLE
        hero?.setVisible(on, enough)
      },
      { threshold: [0, ENTRY_VISIBLE] },
    )
    seen.observe(el)
    return () => {
      cancelled = true
      seen.disconnect()
      hero?.dispose()
    }
  }, [chosen])

  return (
    <div ref={card} className="hero-scene" data-backdrop data-mode={threeD ? '3d' : 'still'} aria-hidden="true">
      {/* Never an empty card: the still image until the 3D has covered it. */}
      {(!threeD || !covered) && <img className="hero-still" src={storyStillURL('1s')} alt="" width={640} height={640} decoding="async" />}
      {threeD && <canvas ref={canvas} className="hero-canvas" data-ready={ready} onTransitionEnd={() => ready && setCovered(true)} />}
      <span className="hero-mark hero-mark-tl" />
      <span className="hero-mark hero-mark-tr" />
      <span className="hero-mark hero-mark-bl" />
      <span className="hero-mark hero-mark-br" />
      <p className="hero-label" data-on={labelled}>
        {/* MathML, as in the story: the text fonts carry no Greek. */}
        <math>
          <msub>
            <mi>ψ</mi>
            <mrow>
              <mn>1</mn>
              <mo>,</mo>
              <mn>0</mn>
              <mo>,</mo>
              <mn>0</mn>
            </mrow>
          </msub>
        </math>{' '}
        · {t('home.sceneLabel')}
      </p>
    </div>
  )
}
