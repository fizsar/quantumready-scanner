import { Link, Route, Routes } from 'react-router'
import type { Lang } from './api'
import { useLang } from './i18n'
import { Home } from './pages/Home'
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

      <main id="main" className="main" tabIndex={-1}>
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/scans/:id" element={<ScanPage />} />
          <Route path="*" element={<NotFound />} />
        </Routes>
      </main>

      <footer className="footer">
        <a href="https://fizsar.com">← fizsar.com</a>
      </footer>
    </div>
  )
}

function LangSwitch() {
  const { lang, setLang, t } = useLang()
  const opt = (l: Lang, label: string) => (
    <button type="button" className="lang-btn" aria-pressed={lang === l} lang={l} onClick={() => setLang(l)}>
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
