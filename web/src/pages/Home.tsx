import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router'
import { solveAltcha } from '../altcha'
import { ApiError, createScan, getAltcha, getTargets } from '../api'
import { HostNotAllowed } from '../components/HostNotAllowed'
import { DemoCta } from '../components/DemoButton'
import { HeroScene } from '../components/HeroScene'
import '../Hero.css'
import { HowItWorks } from '../components/HowItWorks'
import { ShowcaseHomeLinks } from '../components/ShowcaseLinks'
import { useLang } from '../i18n'
import { track } from '../metrics'
import { usePageMeta } from '../usePageMeta'
import { errorMessage } from '../verdict'

export function Home() {
  const { lang, t } = useLang()
  const navigate = useNavigate()
  const inputId = useId()
  const errorId = useId()
  usePageMeta(t('meta.home'))

  const [targets, setTargets] = useState<string[] | null>(null)
  const [mode, setMode] = useState({ open: false, proof: false })
  const [proving, setProving] = useState(false)
  const [host, setHost] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<ApiError | null>(null)

  useEffect(() => {
    let cancelled = false
    getTargets()
      .then((r) => !cancelled && (setTargets(r.hosts), setMode({ open: !!r.open, proof: !!r.proof })))
      .catch((e: unknown) => !cancelled && (setTargets([]), setError(e instanceof ApiError ? e : new ApiError(0, 'network', ''))))
    return () => {
      cancelled = true
    }
  }, [])

  const scan = async (h: string) => {
    setBusy(true)
    setError(null)
    try {
      let proof: string | undefined
      if (mode.proof) {
        // Proof of work against abuse: a second or so in the browser.
        setProving(true)
        proof = await solveAltcha(await getAltcha())
        setProving(false)
      }
      const s = await createScan(h, lang, proof)
      track({ e: 'scan_started', d: s.host })
      navigate(`/scans/${s.id}`, { state: s.reused ? { reusedAge: s.age_seconds ?? 0 } : undefined })
    } catch (e) {
      setProving(false)
      setError(e instanceof ApiError ? e : new ApiError(0, 'network', ''))
      setBusy(false)
    }
  }

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (host.trim()) void scan(host)
  }

  return (
    <div className="home">
      {/* The entry: title, form and scene are all in the first paint (the scene as a
          still image); the 3D arrives later, over the image, in its own cell of the grid.
          data-enter: the order of the entrance (Hero.css), context first. */}
      <section className="hero" aria-labelledby="home-title">
        <div className="hero-copy">
          <p className="hero-eyebrow prompt" data-enter="0">
            <span className="hero-dot" aria-hidden="true" />
            ./scan --pqc
          </p>
          <h1 id="home-title" className="hero-title" data-enter="0">
            <Accent text={t('home.title')} accent={t('home.titleAccent')} />
          </h1>
          <p className="lead hero-lead" data-enter="1">
            {t('home.lead')}
          </p>

          <form className="scan-form" onSubmit={onSubmit} noValidate data-enter="2">
            <label htmlFor={inputId} className="label">
              {t('home.formLabel')}
            </label>
            <div className="row">
              <input
                id={inputId}
                className="input"
                type="text"
                inputMode="url"
                autoComplete="url"
                spellCheck={false}
                placeholder={t('home.placeholder')}
                value={host}
                onChange={(e) => setHost(e.target.value)}
                aria-invalid={error ? true : undefined}
                aria-describedby={error ? errorId : undefined}
                maxLength={253}
              />
              <button type="submit" className="btn" disabled={busy || !host.trim()}>
                {busy ? t('home.submitting') : t('home.submit')}
              </button>
            </div>
            {proving && (
              <p className="progress small" role="status">
                <span className="spinner" aria-hidden="true" />
                {t('home.proving')}
              </p>
            )}
            <p id={errorId} className="error" role="alert">
              {error && !(error.code === 'host_not_allowed' && !mode.open) ? errorMessage(error, t) : ''}
            </p>
          </form>
          {/* The closed-beta explanation only while scanning is closed (PUBLIC_SCAN_OPEN=false). */}
          {error?.code === 'host_not_allowed' && !mode.open && <HostNotAllowed targets={targets} busy={busy} onScan={(h) => void scan(h)} />}
        </div>

        {/* Its own cell of the grid: on wide screens it sits under the form, and
            what loads into it later (the example domains) cannot move the scene. */}
        <div className="hero-more" data-enter="3">
          <p className="muted small">{mode.open ? t('home.open') : t('home.demo')}</p>
          {targets === null ? (
            <p className="muted small">{t('home.loadingTargets')}</p>
          ) : (
            <ul className="targets">
              {targets.map((h) => (
                <li key={h}>
                  <button type="button" className="btn btn-target" disabled={busy} onClick={() => void scan(h)} aria-label={t('home.scanThis', { host: h })}>
                    {h}
                  </button>
                </li>
              ))}
            </ul>
          )}
          <p className="muted small">
            <Link to="/about-scans">{t('home.aboutScans')}</Link>
          </p>
        </div>
        <HeroScene />
      </section>
      <DemoCta from="home" />
      <StoryTeaser lang={lang} />
      <HowItWorks />
      <ShowcaseHomeLinks />
    </div>
  )
}

/** Under the scanner: the way into the story (from quantum mechanics to your website). */
function StoryTeaser({ lang }: { lang: 'es' | 'en' }) {
  const es = lang === 'es'
  return (
    <section aria-labelledby="story-teaser-title" className="story-teaser-wrap">
      {/* The whole card is the link to the story. */}
      <Link to="/historia" className="story-teaser">
        <div className="story-teaser-copy">
          <p className="prompt small">./historia</p>
          <h2 id="story-teaser-title" className="story-teaser-title">
            {es ? '¿Por qué tu web necesita criptografía post-cuántica?' : 'Why does your website need post-quantum cryptography?'}
          </h2>
          <p className="muted">
            {es
              ? 'Del átomo al cifrado de tu web, contado con partículas: cúbits, el algoritmo de Shor, cosechar ahora y descifrar después, y los retículos de ML-KEM. Todo calculado con las ecuaciones de verdad.'
              : 'From the atom to your website’s encryption, told with particles: qubits, Shor’s algorithm, harvest now and decrypt later, and the lattices of ML-KEM. All computed from the real equations.'}
          </p>
          <span className="btn story-teaser-btn">
            {es ? 'Ver la historia' : 'Read the story'} <span aria-hidden="true">→</span>
          </span>
        </div>
        <StoryReel />
      </Link>
    </section>
  )
}

/**
 * What is inside the story: an animated image (like a GIF) recorded from the story's own 3D,
 * an 8.5 s seamless loop at 640 px: AVIF (592 KB), else WebP (1.4 MB). The still poster
 * (22 KB, fixed size) shows at once; the animation's sources are added only when the card
 * comes near the screen. With reduced motion no source matches: the poster stays.
 */
function StoryReel() {
  const box = useRef<HTMLDivElement>(null)
  const [near, setNear] = useState(false)
  useEffect(() => {
    const el = box.current
    if (!el || typeof IntersectionObserver === 'undefined') return
    const io = new IntersectionObserver(
      ([e]) => {
        if (!e.isIntersecting) return
        setNear(true)
        io.disconnect()
      },
      { rootMargin: '300px' },
    )
    io.observe(el)
    return () => io.disconnect()
  }, [])
  const motion = '(prefers-reduced-motion: no-preference)'
  return (
    <div ref={box} className="story-preview" aria-hidden="true">
      <picture>
        {near && <source type="image/avif" srcSet="/story-reel.avif" media={motion} />}
        {near && <source type="image/webp" srcSet="/story-reel.webp" media={motion} />}
        <img src="/story-reel-poster.webp" alt="" width={640} height={640} decoding="async" />
      </picture>
    </div>
  )
}

/** The title with its accent words in the brand's gradient: the same text, split around them. */
function Accent({ text, accent }: { text: string; accent: string }) {
  const i = text.indexOf(accent)
  if (i < 0) return <>{text}</>
  return (
    <>
      {text.slice(0, i)}
      <span className="hero-accent">{accent}</span>
      {text.slice(i + accent.length)}
    </>
  )
}

