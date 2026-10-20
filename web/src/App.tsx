import { lazy, Suspense, useEffect } from 'react'
import { Link, Route, Routes, useLocation } from 'react-router'
import type { Lang } from './api'
import { useLang } from './i18n'
import { useShowcaseT } from './i18n.showcase'
import { useDemoEnabled } from './features'
import { pageView, track } from './metrics'
import { Architecture } from './pages/Architecture'
import { DemoPage } from './pages/DemoPage'
import { Privacy } from './pages/Privacy'
import { Home } from './pages/Home'
import './showcase.css'

// The example reports (fixtures) load only on /examples.
const Examples = lazy(() => import('./pages/Examples'))
import { ScanPage } from './pages/ScanPage'
import { usePageMeta } from './usePageMeta'

export default function App() {
  const { t } = useLang()
  return (
    <div className="page">
      <a className="skip" href="#main">
        {t('nav.skip')}
      </a>
      <header className="topbar">
        <nav aria-label="breadcrumb" className="crumbs">
          <a href="https://fizsar.com">[~/fizsar]</a>
          <span className="muted"> / </span>
          <Link to="/" className="brand-link">
            <span className="logo-sm" aria-hidden="true">
              Q&#770;
            </span>{' '}
            quantumready
          </Link>
        </nav>
        <LangSwitch />
      </header>

      <PageViews />
      <main id="main" className="main" tabIndex={-1}>
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/scans/:id" element={<ScanPage />} />
          <Route path="/architecture" element={<Architecture />} />
          <Route path="/privacy" element={<Privacy />} />
          <Route path="/demo" element={<DemoRoute />} />
          <Route path="/examples" element={<Suspense><Examples /></Suspense>} />
          <Route path="*" element={<NotFound />} />
        </Routes>
      </main>

      <footer className="footer">
        <a href="https://fizsar.com">← fizsar.com</a>
        <ShowcaseFooterLinks />
      </footer>
    </div>
  )
}

/** M1: one page view per route change (not per #anchor). */
function PageViews() {
  const { pathname } = useLocation()
  useEffect(() => pageView(pathname), [pathname])
  return null
}

function ShowcaseFooterLinks() {
  const { t } = useShowcaseT()
  return (
    <>
      <Link to="/examples">{t('links.examples')}</Link>
      <Link to="/architecture">{t('footer.architecture')}</Link>
      <Link to="/privacy">{t('footer.privacy')}</Link>
    </>
  )
}

function LangSwitch() {
  const { lang, setLang, t } = useLang()
  const opt = (l: Lang, label: string) => (
    <button
      type="button"
      className="lang-btn"
      aria-pressed={lang === l}
      lang={l}
      onClick={() => {
        if (l !== lang) track({ e: 'lang_switch', d: l })
        setLang(l)
      }}
    >
      {label}
    </button>
  )
  return (
    <div role="group" aria-label={t('nav.lang')} className="lang-switch">
      {opt('es', 'ES')}
      {opt('en', 'EN')}
    </div>
  )
}

/** /demo exists only while the server has the form on (DEMO_ENABLED). */
function DemoRoute() {
  const on = useDemoEnabled()
  if (on === null) return null
  return on ? <DemoPage /> : <NotFound />
}

function NotFound() {
  const { t } = useLang()
  usePageMeta(t('meta.notFound'), true)
  return (
    <section className="terminal">
      <h1 className="h-title">{t('notFound.title')}</h1>
      <Link to="/">{t('footer.newScan')}</Link>
    </section>
  )
}
