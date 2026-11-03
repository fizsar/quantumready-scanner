import { lazy, Suspense, useEffect } from 'react'
import { Link, Route, Routes, useLocation } from 'react-router'
import type { Lang } from './api'
import { useAuth } from './auth'
import { useLang } from './i18n'
import { useShowcaseT } from './i18n.showcase'
import { useAccountsEnabled, useDemoEnabled, useSourceURL } from './features'
import { pageView, track } from './metrics'
import { AboutScans } from './pages/AboutScans'
import { AccountPage, ForgotPage, LoginPage, RegisterPage, ResetPage, VerifyPage } from './pages/AccountPages'
import { Architecture } from './pages/Architecture'
import { DemoPage } from './pages/DemoPage'
import { Privacy } from './pages/Privacy'
import { Home } from './pages/Home'
import { InvitePage } from './pages/InvitePage'
import { OrgPage } from './pages/OrgPage'
import { ScanPage } from './pages/ScanPage'
import './showcase.css'
import { usePageMeta } from './usePageMeta'

// The example reports (fixtures) load only on /examples.
const Examples = lazy(() => import('./pages/Examples'))
// Redesign prototype (hidden: noindex, not linked, not in the sitemap); its 3D code loads later still.
const LabOrbital = lazy(() => import('./lab/LabOrbital'))

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

      <PageViews />
      <main id="main" className="main" tabIndex={-1}>
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/scans/:id" element={<ScanPage />} />
          <Route path="/login" element={<AccountsRoute page={<LoginPage />} />} />
          <Route path="/register" element={<AccountsRoute page={<RegisterPage />} />} />
          <Route path="/verify" element={<AccountsRoute page={<VerifyPage />} />} />
          <Route path="/forgot" element={<AccountsRoute page={<ForgotPage />} />} />
          <Route path="/reset" element={<AccountsRoute page={<ResetPage />} />} />
          <Route path="/account" element={<AccountsRoute page={<AccountPage />} />} />
          <Route path="/about-scans" element={<AboutScans />} />
          <Route path="/org" element={<AccountsRoute page={<OrgPage />} />} />
          <Route path="/org/scans/:id" element={<AccountsRoute page={<ScanPage org />} />} />
          <Route path="/invite" element={<AccountsRoute page={<InvitePage />} />} />
          <Route path="/architecture" element={<Architecture />} />
          <Route path="/privacy" element={<Privacy />} />
          <Route path="/demo" element={<DemoRoute />} />
          <Route path="/examples" element={<Suspense><Examples /></Suspense>} />
          <Route path="/lab/orbital" element={<Suspense><LabOrbital /></Suspense>} />
          <Route path="*" element={<NotFound />} />
        </Routes>
      </main>

      <footer className="footer">
        <a href="https://fizsar.com">← fizsar.com</a>
        <Link to="/about-scans">{t('nav.about')}</Link>
        <ShowcaseFooterLinks />
        <SourceLink />
      </footer>
    </div>
  )
}

function AccountLink() {
  const { t } = useLang()
  const { state } = useAuth()
  const accounts = useAccountsEnabled()
  if (accounts !== true || state.status === 'loading') return null
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

/** M1: one page view per route change (not per #anchor). */
function PageViews() {
  const { pathname } = useLocation()
  useEffect(() => pageView(pathname), [pathname])
  return null
}

/** The AGPL's "Source code" link, at the deployed commit; only once QR_SOURCE_URL is set. */
function SourceLink() {
  const { t } = useLang()
  const url = useSourceURL()
  return url ? (
    <a href={url} rel="noopener">
      {t('footer.source')}
    </a>
  ) : null
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

/**
 * The pages of accounts, organizations and invitations exist only while the
 * server has accounts on (ACCOUNTS_ENABLED; off in production): otherwise
 * they are the 404.
 */
function AccountsRoute({ page }: { page: React.ReactElement }) {
  const on = useAccountsEnabled()
  if (on === null) return null
  return on ? page : <NotFound />
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
