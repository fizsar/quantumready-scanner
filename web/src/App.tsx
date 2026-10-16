import { Link, Route, Routes } from 'react-router'
import type { Lang } from './api'
import { useAuth } from './auth'
import { useLang } from './i18n'
import { AccountPage, ForgotPage, LoginPage, RegisterPage, ResetPage, VerifyPage } from './pages/AccountPages'
import { Home } from './pages/Home'
import { InvitePage } from './pages/InvitePage'
import { OrgPage } from './pages/OrgPage'
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
        <div className="topbar-right">
          <AccountLink />
          <LangSwitch />
        </div>
      </header>

      <main id="main" className="main" tabIndex={-1}>
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/scans/:id" element={<ScanPage />} />
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
          <Route path="/verify" element={<VerifyPage />} />
          <Route path="/forgot" element={<ForgotPage />} />
          <Route path="/reset" element={<ResetPage />} />
          <Route path="/account" element={<AccountPage />} />
          <Route path="/org" element={<OrgPage />} />
          <Route path="/org/scans/:id" element={<ScanPage org />} />
          <Route path="/invite" element={<InvitePage />} />
          <Route path="*" element={<NotFound />} />
        </Routes>
      </main>

      <footer className="footer">
        <a href="https://fizsar.com">← fizsar.com</a>
      </footer>
    </div>
  )
}

function AccountLink() {
  const { t } = useLang()
  const { state } = useAuth()
  if (state.status === 'loading') return null
  return state.status === 'user' ? (
    <>
      <Link to="/org" className="nav-link">
        {t('nav.org')}
      </Link>
      <Link to="/account" className="nav-link">
        {t('nav.account')}
      </Link>
    </>
  ) : (
    <Link to="/login" className="nav-link">
      {t('nav.login')}
    </Link>
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
